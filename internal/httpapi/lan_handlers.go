package httpapi

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/Akosh36/p1-3server/internal/models"
)

func (s *Server) handleListLANNetworks(w http.ResponseWriter, r *http.Request) {
	rows, err := s.pool.Query(r.Context(), `
		SELECT id, name, type, vpn_peer_id, is_active, is_reachable, last_status_check_at
		FROM lan_networks ORDER BY name
	`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list LAN networks")
		return
	}
	defer rows.Close()

	networks := []models.LANNetwork{}
	for rows.Next() {
		var n models.LANNetwork
		if err := rows.Scan(&n.ID, &n.Name, &n.Type, &n.VPNPeerID, &n.IsActive, &n.IsReachable, &n.LastStatusCheckAt); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to read LAN network row")
			return
		}
		networks = append(networks, n)
	}
	writeJSON(w, http.StatusOK, networks)
}

type updateLANNetworkRequest struct {
	Name     *string `json:"name,omitempty"`
	IsActive *bool   `json:"is_active,omitempty"`
}

func (s *Server) handleUpdateLANNetwork(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid LAN network id")
		return
	}

	var req updateLANNetworkRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	ctx := r.Context()
	claims := claimsFromContext(r)

	if req.Name != nil {
		if _, err := s.pool.Exec(ctx, `UPDATE lan_networks SET name = $1 WHERE id = $2`, *req.Name, id); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to rename LAN network")
			return
		}
	}
	if req.IsActive != nil {
		if _, err := s.pool.Exec(ctx, `UPDATE lan_networks SET is_active = $1 WHERE id = $2`, *req.IsActive, id); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to toggle LAN network")
			return
		}
	}

	_ = s.recordAudit(ctx, claims, "lan_network.update", "lan_network", &id, map[string]interface{}{
		"name": req.Name, "is_active": req.IsActive,
	})

	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}
