// Package aclsync is the control-plane half of Phase 2: it reads the
// current devices/access_grants state from Postgres — the single source of
// truth for who is "user" vs "admin" (CLAUDE.md §11) — and pushes the
// resulting MAC allow-lists to fwctl over its local Unix socket every few
// seconds. fwctl itself never queries Postgres; see internal/firewall.
package aclsync

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

type syncRequest struct {
	UserMACs  []string `json:"user_macs"`
	AdminMACs []string `json:"admin_macs"`
}

// Run polls Postgres every interval and pushes the current allow-lists to
// fwctl's socket, until ctx is cancelled. If fwctl isn't running (no local
// dev setup has it — the rest of the platform works fine without it), each
// failed push is logged once and retried on the next tick; it never
// crashes the API. This is the same resilience pattern as
// internal/discovery.Run for netdiscd.
func Run(ctx context.Context, pool *pgxpool.Pool, socketPath string, interval time.Duration) {
	client := newUnixSocketClient(socketPath)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	warnedUnreachable := false
	for {
		userMACs, adminMACs, err := loadGrantedMACs(ctx, pool)
		if err != nil {
			slog.Error("aclsync: failed to load access grants", "error", err)
		} else if err := push(ctx, client, userMACs, adminMACs); err != nil {
			if !warnedUnreachable {
				slog.Warn("aclsync: fwctl unreachable, firewall will not enforce access changes until it is running", "socket", socketPath, "error", err)
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

func loadGrantedMACs(ctx context.Context, pool *pgxpool.Pool) (userMACs, adminMACs []string, err error) {
	rows, err := pool.Query(ctx, `
		SELECT d.mac_address::text, ag.role
		FROM devices d
		JOIN access_grants ag ON ag.device_id = d.id
	`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var mac, role string
		if err := rows.Scan(&mac, &role); err != nil {
			return nil, nil, err
		}
		switch role {
		case "admin":
			adminMACs = append(adminMACs, mac)
		case "user":
			userMACs = append(userMACs, mac)
		}
	}
	return userMACs, adminMACs, rows.Err()
}

func push(ctx context.Context, client *http.Client, userMACs, adminMACs []string) error {
	body, err := json.Marshal(syncRequest{UserMACs: userMACs, AdminMACs: adminMACs})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://fwctl/sync", bytes.NewReader(body))
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
		return fmt.Errorf("fwctl returned status %d", resp.StatusCode)
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
