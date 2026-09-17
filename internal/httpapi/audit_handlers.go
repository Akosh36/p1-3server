package httpapi

import (
	"net/http"
	"strconv"

	"github.com/Akosh36/p1-3server/internal/models"
)

func (s *Server) handleListAuditLogs(w http.ResponseWriter, r *http.Request) {
	limit := 200
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 1000 {
			limit = n
		}
	}

	rows, err := s.pool.Query(r.Context(), `
		SELECT id, actor_admin_id, action, target_type, target_id, details, created_at
		FROM audit_logs ORDER BY created_at DESC LIMIT $1
	`, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list audit logs")
		return
	}
	defer rows.Close()

	logs := []models.AuditLog{}
	for rows.Next() {
		var l models.AuditLog
		if err := rows.Scan(&l.ID, &l.ActorAdminID, &l.Action, &l.TargetType, &l.TargetID, &l.Details, &l.CreatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to read audit log row")
			return
		}
		logs = append(logs, l)
	}
	writeJSON(w, http.StatusOK, logs)
}
