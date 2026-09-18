// Package discovery is the control-plane half of Phase 1: it pulls the
// point-in-time Snapshot netdiscd publishes over its local Unix socket and
// reconciles it into Postgres (devices, switch_ports). netdiscd itself never
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

	for _, d := range snapshot.Devices {
		var switchPortID interface{}
		if d.SwitchName != "" {
			if id, ok := portIDs[switchPortKey(d.SwitchName, d.PortNumber)]; ok {
				switchPortID = id
			}
		}

		_, err := tx.Exec(ctx, `
			INSERT INTO devices (mac_address, ip_address, hostname, conn_type, switch_port_id, ssid, last_seen_at, is_online, first_seen_at)
			VALUES ($1, NULLIF($2, '')::inet, NULLIF($3, ''), $4, $5, NULLIF($6, ''), $7, $8, now())
			ON CONFLICT (mac_address) DO UPDATE SET
				ip_address     = COALESCE(EXCLUDED.ip_address, devices.ip_address),
				hostname       = COALESCE(EXCLUDED.hostname, devices.hostname),
				conn_type      = EXCLUDED.conn_type,
				switch_port_id = COALESCE(EXCLUDED.switch_port_id, devices.switch_port_id),
				ssid           = COALESCE(EXCLUDED.ssid, devices.ssid),
				last_seen_at   = GREATEST(devices.last_seen_at, EXCLUDED.last_seen_at),
				is_online      = EXCLUDED.is_online
		`, d.MAC, d.IP, d.Hostname, d.ConnType, switchPortID, d.SSID, d.LastSeenAt, d.IsOnline)
		if err != nil {
			return fmt.Errorf("upsert device %s: %w", d.MAC, err)
		}
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
