// Package wgsync is the control-plane half of Phase 7: it reads
// vpn_peers/lan_networks from Postgres, decides which peers should
// currently be active (both the peer itself and any lan_networks row
// toggling it must be active), and pushes that desired list to wgd over
// its local Unix socket. wgd itself never touches Postgres — see
// internal/wg.
package wgsync

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type peer struct {
	PublicKey     string `json:"public_key"`
	AllowedSubnet string `json:"allowed_subnet"`
	Endpoint      string `json:"endpoint,omitempty"`
}

type syncRequest struct {
	Peers []peer `json:"peers"`
}

type peerStatus struct {
	PublicKey       string `json:"public_key"`
	LastHandshakeAt string `json:"last_handshake_at,omitempty"`
	IsReachable     bool   `json:"is_reachable"`
}

type statusResponse struct {
	LocalPublicKey string       `json:"local_public_key"`
	ListenPort     int          `json:"listen_port"`
	Peers          []peerStatus `json:"peers"`
}

// Run pushes the desired peer list to wgd and pulls its live handshake
// status back every interval, until ctx is cancelled. Same resilience
// pattern as aclsync/discovery/lbsync/capdsync: if wgd isn't running, this
// logs a warning once and keeps retrying rather than crashing the API.
func Run(ctx context.Context, pool *pgxpool.Pool, socketPath string, interval time.Duration) {
	client := newUnixSocketClient(socketPath)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	warnedUnreachable := false
	for {
		if err := tick(ctx, pool, client); err != nil {
			if !warnedUnreachable {
				slog.Warn("wgsync: wgd unreachable, VPN tunnel state will not update until it is running", "socket", socketPath, "error", err)
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

type peerRow struct {
	id            int64
	publicKey     string
	allowedSubnet string
	endpoint      *string
	desired       bool
}

func tick(ctx context.Context, pool *pgxpool.Pool, client *http.Client) error {
	allPeers, err := loadAllPeers(ctx, pool)
	if err != nil {
		return fmt.Errorf("load peers: %w", err)
	}

	var desired []peer
	for _, p := range allPeers {
		if !p.desired {
			continue
		}
		endpoint := ""
		if p.endpoint != nil {
			endpoint = *p.endpoint
		}
		desired = append(desired, peer{PublicKey: p.publicKey, AllowedSubnet: p.allowedSubnet, Endpoint: endpoint})
	}

	if err := push(ctx, client, desired); err != nil {
		return fmt.Errorf("push: %w", err)
	}

	status, err := pull(ctx, client)
	if err != nil {
		return fmt.Errorf("pull status: %w", err)
	}

	reachableByKey := make(map[string]bool, len(status.Peers))
	handshakeByKey := make(map[string]string, len(status.Peers))
	for _, ps := range status.Peers {
		reachableByKey[ps.PublicKey] = ps.IsReachable
		handshakeByKey[ps.PublicKey] = ps.LastHandshakeAt
	}

	for _, p := range allPeers {
		// A deactivated peer (p.desired == false) is never pushed to wgd,
		// so it can never be "reachable" — recomputing this for every peer
		// on every tick (rather than only writing when wgd reports one)
		// makes sure a LAN toggled off stops showing a stale "reachable"
		// from before deactivation, instead of just going silent.
		reachable := p.desired && reachableByKey[p.publicKey]
		if err := updatePeerAndLAN(ctx, pool, p.id, handshakeByKey[p.publicKey], reachable); err != nil {
			slog.Error("wgsync: failed to update peer/LAN status", "peer_id", p.id, "error", err)
		}
	}

	return nil
}

// loadAllPeers returns every vpn_peers row, each flagged with whether it
// should currently be active on wgd's interface: the peer definition
// itself must be active, AND — if it's linked to a lan_networks row (the
// admin's Active/Deactive toggle on the LAN page) — that row must be
// active too. A peer with no linked lan_networks row yet is desired on
// vpn_peers.is_active alone.
func loadAllPeers(ctx context.Context, pool *pgxpool.Pool) ([]peerRow, error) {
	rows, err := pool.Query(ctx, `
		SELECT vp.id, vp.public_key, vp.allowed_subnet::text, vp.endpoint, vp.is_active,
		       NOT EXISTS (
		           SELECT 1 FROM lan_networks ln
		           WHERE ln.vpn_peer_id = vp.id AND ln.is_active = false
		       ) AS lan_ok
		FROM vpn_peers vp
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []peerRow
	for rows.Next() {
		var pr peerRow
		var isActive, lanOK bool
		if err := rows.Scan(&pr.id, &pr.publicKey, &pr.allowedSubnet, &pr.endpoint, &isActive, &lanOK); err != nil {
			return nil, err
		}
		pr.desired = isActive && lanOK
		result = append(result, pr)
	}
	return result, rows.Err()
}

func updatePeerAndLAN(ctx context.Context, pool *pgxpool.Pool, peerID int64, handshakeAt string, reachable bool) error {
	if handshakeAt != "" {
		if _, err := pool.Exec(ctx, `UPDATE vpn_peers SET last_handshake_at = $1 WHERE id = $2`, handshakeAt, peerID); err != nil {
			return err
		}
	}
	_, err := pool.Exec(ctx, `
		UPDATE lan_networks SET is_reachable = $1, last_status_check_at = now()
		WHERE vpn_peer_id = $2
	`, reachable, peerID)
	return err
}

func push(ctx context.Context, client *http.Client, peers []peer) error {
	body, err := json.Marshal(syncRequest{Peers: peers})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://wgd/sync", bytes.NewReader(body))
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
		msg, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("wgd returned status %d: %s", resp.StatusCode, msg)
	}
	return nil
}

func pull(ctx context.Context, client *http.Client) (statusResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://wgd/status", nil)
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

// LocalInfo returns this gateway's own WireGuard public key and listen
// port, for an admin action (e.g. GET /api/wireguard/local-info) that
// needs it right now rather than waiting on the next tick — the value
// doesn't change once wgd has started, so there's nothing to reconcile.
func LocalInfo(ctx context.Context, socketPath string) (publicKey string, listenPort int, err error) {
	status, err := pull(ctx, newUnixSocketClient(socketPath))
	if err != nil {
		return "", 0, err
	}
	return status.LocalPublicKey, status.ListenPort, nil
}

func newUnixSocketClient(socketPath string) *http.Client {
	return &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, "unix", socketPath)
			},
		},
	}
}
