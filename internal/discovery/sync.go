// Package discovery is the control-plane half of Phase 1: it pulls the
// point-in-time Snapshot netdiscd publishes over its local Unix socket and
// reconciles it into Postgres (devices, switch_ports, and — since Phase 9 —
// lan_networks for wired/wireless_local networks). netdiscd itself never
// touches the database — see CLAUDE.md section 4 for why the split exists.
package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Akosh36/p1-3server/internal/netdisc"
)

// Run polls netdiscd's snapshot over socketPath every interval and upserts
// it into Postgres, until ctx is cancelled. If netdiscd isn't running yet
// (no local dev setup has it — Phase 0's control-plane works fine without
// it), each failed poll is logged once and retried on the next tick; it
// never crashes the API.
func Run(ctx context.Context, pool *pgxpool.Pool, socketPath string, interval time.Duration) {
	client := newUnixSocketClient(socketPath)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	warnedUnreachable := false
	for {
		snapshot, err := fetchSnapshot(ctx, client)
		if err != nil {
			if !warnedUnreachable {
				slog.Warn("discovery: netdiscd unreachable, device inventory will not update until it is running", "socket", socketPath, "error", err)
				warnedUnreachable = true
			}
		} else {
			warnedUnreachable = false
			if err := reconcile(ctx, pool, snapshot); err != nil {
				slog.Error("discovery: failed to reconcile snapshot", "error", err)
			}
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func newUnixSocketClient(socketPath string) *http.Client {
	return &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, "unix", socketPath)
			},
		},
	}
}

func fetchSnapshot(ctx context.Context, client *http.Client) (*netdisc.Snapshot, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://netdiscd/snapshot", nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var snapshot netdisc.Snapshot
	if err := json.NewDecoder(resp.Body).Decode(&snapshot); err != nil {
		return nil, fmt.Errorf("decode snapshot: %w", err)
	}
	return &snapshot, nil
}

func reconcile(ctx context.Context, pool *pgxpool.Pool, snapshot *netdisc.Snapshot) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	portIDs, err := upsertSwitchPorts(ctx, tx, snapshot.SwitchPorts)
	if err != nil {
		return fmt.Errorf("upsert switch ports: %w", err)
	}

	wiredLANIDs, err := upsertWiredLANNetworks(ctx, tx, snapshot)
	if err != nil {
		return fmt.Errorf("upsert wired LAN networks: %w", err)
	}
	wirelessLANIDs, err := upsertWirelessLocalLANNetworks(ctx, tx, snapshot.Devices)
	if err != nil {
		return fmt.Errorf("upsert wireless LAN networks: %w", err)
	}

	for _, d := range snapshot.Devices {
		var switchPortID interface{}
		if d.SwitchName != "" {
			if id, ok := portIDs[switchPortKey(d.SwitchName, d.PortNumber)]; ok {
				switchPortID = id
			}
		}

		var lanNetworkID interface{}
		switch d.ConnType {
		case "wired":
			if id, ok := wiredLANIDs[d.SwitchName]; ok {
				lanNetworkID = id
			}
		case "wireless_local":
			if id, ok := wirelessLANIDs[d.SSID]; ok {
				lanNetworkID = id
			}
		}

		_, err := tx.Exec(ctx, `
			INSERT INTO devices (mac_address, ip_address, hostname, conn_type, switch_port_id, ssid, lan_network_id, last_seen_at, is_online, first_seen_at)
			VALUES ($1, NULLIF($2, '')::inet, NULLIF($3, ''), $4, $5, NULLIF($6, ''), $7, $8, $9, now())
			ON CONFLICT (mac_address) DO UPDATE SET
				ip_address     = COALESCE(EXCLUDED.ip_address, devices.ip_address),
				hostname       = COALESCE(EXCLUDED.hostname, devices.hostname),
				conn_type      = EXCLUDED.conn_type,
				switch_port_id = COALESCE(EXCLUDED.switch_port_id, devices.switch_port_id),
				ssid           = COALESCE(EXCLUDED.ssid, devices.ssid),
				lan_network_id = COALESCE(EXCLUDED.lan_network_id, devices.lan_network_id),
				last_seen_at   = GREATEST(devices.last_seen_at, EXCLUDED.last_seen_at),
				is_online      = EXCLUDED.is_online
		`, d.MAC, d.IP, d.Hostname, d.ConnType, switchPortID, d.SSID, lanNetworkID, d.LastSeenAt, d.IsOnline)
		if err != nil {
			return fmt.Errorf("upsert device %s: %w", d.MAC, err)
		}
	}

	if err := updateLocalLANReachability(ctx, tx); err != nil {
		return fmt.Errorf("update LAN reachability: %w", err)
	}

	return tx.Commit(ctx)
}

// upsertSwitchPorts writes every observed port and returns a
// "switchName/portNumber" -> id map so device rows can resolve their
// switch_port_id foreign key.
func upsertSwitchPorts(ctx context.Context, tx pgx.Tx, ports []netdisc.ObservedSwitchPort) (map[string]int64, error) {
	ids := make(map[string]int64, len(ports))
	for _, p := range ports {
		var id int64
		err := tx.QueryRow(ctx, `
			INSERT INTO switch_ports (switch_name, port_number, label, link_status, last_change_at)
			VALUES ($1, $2, NULLIF($3, ''), $4, now())
			ON CONFLICT (switch_name, port_number) DO UPDATE SET
				label          = COALESCE(EXCLUDED.label, switch_ports.label),
				last_change_at = CASE WHEN switch_ports.link_status <> EXCLUDED.link_status
				                      THEN now() ELSE switch_ports.last_change_at END,
				link_status    = EXCLUDED.link_status
			RETURNING id
		`, p.SwitchName, p.PortNumber, p.Label, p.LinkStatus).Scan(&id)
		if err != nil {
			return nil, fmt.Errorf("upsert switch port %s/%d: %w", p.SwitchName, p.PortNumber, err)
		}
		ids[switchPortKey(p.SwitchName, p.PortNumber)] = id
	}
	return ids, nil
}

func switchPortKey(switchName string, port int) string {
	return switchName + "/" + strconv.Itoa(port)
}

// unassignedWiredLANName buckets wired devices netdiscd can't tie to a
// specific switch — e.g. no SNMP MAC-to-port mapping (BRIDGE-MIB
// dot1dTpFdbTable, Phase 1's documented "best-effort, not all switches
// support it" limitation) — into one catch-all network instead of leaving
// them with no LAN network at all.
const unassignedWiredLANName = "Simli tarmoq (port aniqlanmagan)"

// upsertWiredLANNetworks ensures one lan_networks row (type='wired') per
// distinct switch reported this tick, plus the catch-all row above if any
// wired device has no resolved switch, and returns a "switch_name" -> id
// map (using "" as the catch-all's key, matching ObservedDevice.SwitchName
// being empty in that case) so the device loop in reconcile can resolve
// each wired device's lan_network_id.
func upsertWiredLANNetworks(ctx context.Context, tx pgx.Tx, snapshot *netdisc.Snapshot) (map[string]int64, error) {
	ids := make(map[string]int64)
	for _, p := range snapshot.SwitchPorts {
		if _, ok := ids[p.SwitchName]; ok {
			continue
		}
		id, err := upsertLocalLANNetwork(ctx, tx, "wired", p.SwitchName)
		if err != nil {
			return nil, fmt.Errorf("switch %s: %w", p.SwitchName, err)
		}
		ids[p.SwitchName] = id
	}

	for _, d := range snapshot.Devices {
		if d.ConnType != "wired" || d.SwitchName != "" {
			continue
		}
		if _, ok := ids[""]; ok {
			break
		}
		id, err := upsertLocalLANNetwork(ctx, tx, "wired", unassignedWiredLANName)
		if err != nil {
			return nil, fmt.Errorf("catch-all wired network: %w", err)
		}
		ids[""] = id
		break
	}
	return ids, nil
}

// upsertWirelessLocalLANNetworks ensures one lan_networks row
// (type='wireless_local') per distinct SSID observed this tick, and
// returns an "ssid" -> id map. A wireless_local device with no SSID
// (shouldn't normally happen — hostapd always reports one) is simply left
// without a lan_network_id, same as a wired device is until its switch is
// known.
func upsertWirelessLocalLANNetworks(ctx context.Context, tx pgx.Tx, devices []netdisc.ObservedDevice) (map[string]int64, error) {
	ids := make(map[string]int64)
	for _, d := range devices {
		if d.ConnType != "wireless_local" || d.SSID == "" {
			continue
		}
		if _, ok := ids[d.SSID]; ok {
			continue
		}
		id, err := upsertLocalLANNetwork(ctx, tx, "wireless_local", d.SSID)
		if err != nil {
			return nil, fmt.Errorf("ssid %s: %w", d.SSID, err)
		}
		ids[d.SSID] = id
	}
	return ids, nil
}

// upsertLocalLANNetwork ensures a lan_networks row exists for a
// netdiscd-observed wired switch or wireless_local SSID. Unlike a remote
// VPN entry (created explicitly by an admin, with wgd able to really
// activate/deactivate it), there is no lever here to turn a physical
// switch or WiFi AP off — is_active is left at its schema default (true)
// and never touched by discovery.
func upsertLocalLANNetwork(ctx context.Context, tx pgx.Tx, lanType, name string) (int64, error) {
	var id int64
	err := tx.QueryRow(ctx, `
		INSERT INTO lan_networks (name, type)
		VALUES ($1, $2)
		ON CONFLICT (type, name) DO UPDATE SET name = EXCLUDED.name
		RETURNING id
	`, name, lanType).Scan(&id)
	return id, err
}

// updateLocalLANReachability computes is_reachable for every auto-discovered
// local network as "does it currently have at least one online device" —
// the same real, no-daemon-needed signal Phase 1 already uses per-device,
// just aggregated. Remote VPN networks are untouched here; wgsync (Phase 7)
// owns their is_reachable based on WireGuard handshake state.
func updateLocalLANReachability(ctx context.Context, tx pgx.Tx) error {
	_, err := tx.Exec(ctx, `
		UPDATE lan_networks ln
		SET is_reachable = EXISTS (
			SELECT 1 FROM devices d WHERE d.lan_network_id = ln.id AND d.is_online = true
		),
		last_status_check_at = now()
		WHERE ln.type IN ('wired', 'wireless_local')
	`)
	return err
}
