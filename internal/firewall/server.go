package firewall

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
)

type syncRequest struct {
	UserMACs  []string `json:"user_macs"`
	AdminMACs []string `json:"admin_macs"`
}

// ServeControl listens on a Unix socket at socketPath for the control-plane
// API to push desired ACL state (POST /sync) and read diagnostics
// (GET /status). Like netdiscd's socket, this is localhost-only by
// construction — see CLAUDE.md §4.
func ServeControl(m *Manager, socketPath string) (*http.Server, error) {
	if err := os.MkdirAll(filepath.Dir(socketPath), 0755); err != nil {
		return nil, fmt.Errorf("create socket dir: %w", err)
	}
	os.Remove(socketPath)

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", socketPath, err)
	}
	if err := os.Chmod(socketPath, 0660); err != nil {
		return nil, fmt.Errorf("chmod socket: %w", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/sync", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		var req syncRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		state := DesiredState{UserMACs: req.UserMACs, AdminMACs: req.AdminMACs}
		if err := m.Sync(r.Context(), state); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(m.Status())
	})

	server := &http.Server{Handler: mux}
	go server.Serve(listener)
	return server, nil
}
