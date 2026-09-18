package lb

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
)

type syncRequest struct {
	Groups []Group `json:"groups"`
}

// ServeControl listens on a Unix socket for the control-plane API to push
// desired groups (POST /sync) and pull live backend health (GET /status).
// Localhost-only by construction, like netdiscd's and fwctl's sockets.
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
		if err := m.Sync(r.Context(), req.Groups); err != nil {
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
