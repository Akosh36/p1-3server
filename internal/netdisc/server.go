package netdisc

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
)

// ServeSnapshot listens on a Unix socket at socketPath and serves the
// store's current Snapshot as JSON on GET /snapshot. The socket is
// localhost-only by construction (it's a filesystem path, not a TCP port),
// which is how the control-plane API is meant to reach it — see the
// data-plane/control-plane split in CLAUDE.md and docs/deploy.md.
func ServeSnapshot(store *Store, socketPath string) (*http.Server, error) {
	if err := os.MkdirAll(filepath.Dir(socketPath), 0755); err != nil {
		return nil, fmt.Errorf("create socket dir: %w", err)
	}
	os.Remove(socketPath) // clear a stale socket from a previous run

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", socketPath, err)
	}
	if err := os.Chmod(socketPath, 0660); err != nil {
		return nil, fmt.Errorf("chmod socket: %w", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/snapshot", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(store.Snapshot())
	})

	server := &http.Server{Handler: mux}
	go server.Serve(listener)
	return server, nil
}
