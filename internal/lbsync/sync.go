// Package lbsync is the control-plane half of Phase 4: it reads
// server_groups/backend_servers from Postgres and pushes the desired VIP
// configuration to lbd over its local Unix socket, then pulls lbd's live
// health results back into backend_servers.is_healthy/last_check_at/
// response_time_ms. lbd itself never touches Postgres — see internal/lb.
package lbsync

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type backend struct {
	IP     string `json:"ip"`
	Port   int    `json:"port"`
	Weight int    `json:"weight"`
}

type group struct {
	Nickname   string    `json:"nickname"`
	VIPAddress string    `json:"vip_address"`
	VIPPort    int       `json:"vip_port"`
	Algorithm  string    `json:"algorithm"`
	Backends   []backend `json:"backends"`
}

type syncRequest struct {
	Groups []group `json:"groups"`
}

type backendHealth struct {
	IP             string  `json:"ip"`
	Port           int     `json:"port"`
	IsHealthy      bool    `json:"is_healthy"`
	LastCheckAt    string  `json:"last_check_at"`
	ResponseTimeMs float64 `json:"response_time_ms"`
}

type groupStatus struct {
	VIPAddress string          `json:"vip_address"`
	VIPPort    int             `json:"vip_port"`
	Backends   []backendHealth `json:"backends"`
}

type statusResponse struct {
	Groups []groupStatus `json:"groups"`
}

// clientTraffic mirrors internal/lb.ClientTraffic: one client IP's
// accumulated byte counts through one group's VIP since the last drain.
type clientTraffic struct {
	VIPAddress string `json:"vip_address"`
	ClientIP   string `json:"client_ip"`
	BytesUp    int64  `json:"bytes_up"`
	BytesDown  int64  `json:"bytes_down"`
}

type trafficResponse struct {
	Entries []clientTraffic `json:"entries"`
}

// Run pushes the current server_groups/backend_servers to lbd and pulls its
// health results back every interval, until ctx is cancelled. Same
// resilience pattern as internal/discovery and internal/aclsync: if lbd
// isn't running, this logs a warning once and keeps retrying rather than
// crashing the API.
func Run(ctx context.Context, pool *pgxpool.Pool, socketPath string, interval time.Duration) {
	client := newUnixSocketClient(socketPath)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	warnedUnreachable := false
	for {
		if err := tick(ctx, pool, client); err != nil {
			if !warnedUnreachable {
				slog.Warn("lbsync: lbd unreachable, backend health will not update until it is running", "socket", socketPath, "error", err)
				warnedUnreachable = true
			}
		} else {
			warnedUnreachable = false
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func tick(ctx context.Context, pool *pgxpool.Pool, client *http.Client) error {
	groups, err := loadGroups(ctx, pool)
	if err != nil {
		return fmt.Errorf("load groups: %w", err)
	}

	if err := push(ctx, client, groups); err != nil {
		return fmt.Errorf("push: %w", err)
	}

	status, err := pull(ctx, client)
	if err != nil {
		return fmt.Errorf("pull status: %w", err)
	}

	if err := writeHealth(ctx, pool, status); err != nil {
		return fmt.Errorf("write health: %w", err)
	}

	// Traffic accounting is best-effort and kept separate from the error
	// path above: push+pull(status) succeeding already proves lbd is
	// reachable, so a failure here (a transient write error, say) logs and
	// moves on instead of re-triggering Run's "lbd unreachable" warning for
	// an unrelated problem.
	traffic, err := pullTraffic(ctx, client)
	if err != nil {
		slog.Warn("lbsync: failed to pull traffic from lbd", "error", err)
	} else if err := writeTraffic(ctx, pool, traffic); err != nil {
		slog.Warn("lbsync: failed to write device traffic", "error", err)
	}

	return nil
}

func loadGroups(ctx context.Context, pool *pgxpool.Pool) ([]group, error) {
	rows, err := pool.Query(ctx, `
		SELECT id, nickname, host(vip_address), vip_port, algorithm
		FROM server_groups WHERE is_active = true
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type groupRow struct {
		id int64
		g  group
	}
	var groupRows []groupRow
	for rows.Next() {
		var gr groupRow
		if err := rows.Scan(&gr.id, &gr.g.Nickname, &gr.g.VIPAddress, &gr.g.VIPPort, &gr.g.Algorithm); err != nil {
			return nil, err
		}
		groupRows = append(groupRows, gr)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	groups := make([]group, 0, len(groupRows))
	for _, gr := range groupRows {
		backendRows, err := pool.Query(ctx, `
			SELECT host(ip), port, weight FROM backend_servers WHERE group_id = $1
		`, gr.id)
		if err != nil {
			return nil, err
		}
		for backendRows.Next() {
			var b backend
			if err := backendRows.Scan(&b.IP, &b.Port, &b.Weight); err != nil {
				backendRows.Close()
				return nil, err
			}
			gr.g.Backends = append(gr.g.Backends, b)
		}
		backendRows.Close()
		groups = append(groups, gr.g)
	}
	return groups, nil
}

func writeHealth(ctx context.Context, pool *pgxpool.Pool, status statusResponse) error {
	for _, gs := range status.Groups {
		for _, b := range gs.Backends {
			_, err := pool.Exec(ctx, `
				UPDATE backend_servers SET is_healthy = $1, last_check_at = $2, response_time_ms = $3
				WHERE ip = $4 AND port = $5
				  AND group_id = (SELECT id FROM server_groups WHERE host(vip_address) = $6 AND vip_port = $7)
			`, b.IsHealthy, nullIfEmpty(b.LastCheckAt), b.ResponseTimeMs, b.IP, b.Port, gs.VIPAddress, gs.VIPPort)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func nullIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

func push(ctx context.Context, client *http.Client, groups []group) error {
	body, err := json.Marshal(syncRequest{Groups: groups})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://lbd/sync", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("lbd returned status %d", resp.StatusCode)
	}
	return nil
}

func pull(ctx context.Context, client *http.Client) (statusResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://lbd/status", nil)
	if err != nil {
		return statusResponse{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return statusResponse{}, err
	}
	defer resp.Body.Close()

	var status statusResponse
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return statusResponse{}, err
	}
	return status, nil
}

func pullTraffic(ctx context.Context, client *http.Client) (trafficResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://lbd/traffic", nil)
	if err != nil {
		return trafficResponse{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return trafficResponse{}, err
	}
	defer resp.Body.Close()

	var traffic trafficResponse
	if err := json.NewDecoder(resp.Body).Decode(&traffic); err != nil {
		return trafficResponse{}, err
	}
	return traffic, nil
}

// writeTraffic attributes each drained client-traffic entry to the device
// and server group it came from, if they're both known to Postgres. lbd
// only ever sees raw IP addresses — it has no idea which devices.id a
// client IP maps to or which server_groups.id a VIP belongs to — so the
// INSERT...SELECT below resolves both here and naturally drops any entry
// that doesn't match a known device or group, rather than guessing or
// fabricating an association.
func writeTraffic(ctx context.Context, pool *pgxpool.Pool, t trafficResponse) error {
	for _, entry := range t.Entries {
		if entry.BytesUp == 0 && entry.BytesDown == 0 {
			continue
		}
		// bytes_in/bytes_out are from the device's own perspective, same
		// convention as system_metrics' net_in_bps/net_out_bps: what the
		// backend sent back to the client (BytesDown) is what the device
		// received (bytes_in); what the client sent upstream (BytesUp) is
		// what the device sent out (bytes_out).
		_, err := pool.Exec(ctx, `
			INSERT INTO device_traffic_stats (device_id, backend_group_id, bytes_in, bytes_out)
			SELECT d.id, sg.id, $3, $4
			FROM devices d, server_groups sg
			WHERE d.ip_address = $1::inet AND sg.vip_address = $2::inet
		`, entry.ClientIP, entry.VIPAddress, entry.BytesDown, entry.BytesUp)
		if err != nil {
			return fmt.Errorf("client %s via vip %s: %w", entry.ClientIP, entry.VIPAddress, err)
		}
	}
	return nil
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
