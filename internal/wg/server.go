package wg

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
)

// ServeControl listens on a Unix socket for the control-plane API
// (internal/wgsync) to push the desired peer list (POST /sync) and pull
// live handshake status (GET /status) — localhost-only by construction,
// like netdiscd's/fwctl's/lbd's/capd's sockets.
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

	mux.HandleFunc("POST /sync", func(w http.ResponseWriter, r *http.Request) {
		var req SyncRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := m.Sync(req.Peers); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("GET /status", func(w http.ResponseWriter, r *http.Request) {
		status, err := m.Status()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(status)
	})

	server := &http.Server{Handler: mux}
	go server.Serve(listener)
	return server, nil
}
