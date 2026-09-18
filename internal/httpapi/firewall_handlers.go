package httpapi

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"time"
)

type firewallStatus struct {
	Connected     bool      `json:"connected"`
	UserMACCount  int       `json:"user_mac_count"`
	AdminMACCount int       `json:"admin_mac_count"`
	LastAppliedAt time.Time `json:"last_applied_at,omitempty"`
	LastError     string    `json:"last_error,omitempty"`
}

// handleFirewallStatus reports what fwctl has ACTUALLY loaded into the
// kernel right now (via its /status endpoint), not just what Postgres says
// it should be — the two can differ for a few seconds around a change, or
// indefinitely if fwctl isn't running at all.
func (s *Server) handleFirewallStatus(w http.ResponseWriter, r *http.Request) {
	client := &http.Client{
		Timeout: 2 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, "unix", s.cfg.FwctlSocket)
			},
		},
	}

	resp, err := client.Get("http://fwctl/status")
	if err != nil {
		writeJSON(w, http.StatusOK, firewallStatus{Connected: false})
		return
	}
	defer resp.Body.Close()

	var upstream struct {
		UserMACCount  int       `json:"user_mac_count"`
		AdminMACCount int       `json:"admin_mac_count"`
		LastAppliedAt time.Time `json:"last_applied_at"`
		LastError     string    `json:"last_error,omitempty"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&upstream); err != nil {
		writeJSON(w, http.StatusOK, firewallStatus{Connected: false})
		return
	}

	writeJSON(w, http.StatusOK, firewallStatus{
		Connected:     true,
		UserMACCount:  upstream.UserMACCount,
		AdminMACCount: upstream.AdminMACCount,
		LastAppliedAt: upstream.LastAppliedAt,
		LastError:     upstream.LastError,
	})
}
