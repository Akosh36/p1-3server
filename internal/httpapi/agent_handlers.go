package httpapi

import (
	"net/http"
	"strings"

	"github.com/Akosh36/p1-3server/internal/hostmetrics"
)

// handleAgentMetrics receives one host-metrics sample pushed by
// cmd/backendagentd running on a backend server. It is deliberately outside
// the requireAuth (admin JWT) group: the caller is a machine on a backend
// server, not a logged-in admin, so it authenticates with the per-backend
// agent_token instead (Bearer, same header shape as JWT so the agent can
// reuse a normal HTTP client, but checked against backend_servers directly).
func (s *Server) handleAgentMetrics(w http.ResponseWriter, r *http.Request) {
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, "Bearer ") {
		writeError(w, http.StatusUnauthorized, "missing bearer token")
		return
	}
	token := strings.TrimPrefix(header, "Bearer ")

	var sample hostmetrics.Sample
	if err := decodeJSON(r, &sample); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	ctx := r.Context()
	var backendID int64
	err := s.pool.QueryRow(ctx, `SELECT id FROM backend_servers WHERE agent_token = $1`, token).Scan(&backendID)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid agent token")
		return
	}

	_, err = s.pool.Exec(ctx, `
		INSERT INTO backend_metrics (backend_server_id, cpu_percent, mem_percent, disk_percent, disk_read_bps, disk_write_bps, net_in_bps, net_out_bps)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, backendID, sample.CPUPercent, sample.MemPercent, sample.DiskPercent, sample.DiskReadBps, sample.DiskWriteBps, sample.NetInBps, sample.NetOutBps)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to record metric")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
