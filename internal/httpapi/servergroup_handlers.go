package httpapi

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/Akosh36/p1-3server/internal/models"
)

func (s *Server) handleListServerGroups(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rows, err := s.pool.Query(ctx, `
		SELECT id, nickname, color_hex, host(vip_address), vip_port, protocol, algorithm, is_active, created_at
		FROM server_groups ORDER BY nickname
	`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list server groups")
		return
	}
	defer rows.Close()

	groups := []models.ServerGroup{}
	groupIndex := map[int64]int{}
	for rows.Next() {
		var g models.ServerGroup
		if err := rows.Scan(&g.ID, &g.Nickname, &g.ColorHex, &g.VIPAddress, &g.VIPPort, &g.Protocol, &g.Algorithm, &g.IsActive, &g.CreatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to read server group row")
			return
		}
		g.Backends = []models.BackendServer{}
		groupIndex[g.ID] = len(groups)
		groups = append(groups, g)
	}
	rows.Close()

	if len(groups) > 0 {
		backendRows, err := s.pool.Query(ctx, `
			SELECT id, group_id, host(ip), port, weight, is_healthy, last_check_at, response_time_ms
			FROM backend_servers ORDER BY group_id, ip, port
		`)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to list backends")
			return
		}
		defer backendRows.Close()
		for backendRows.Next() {
			var b models.BackendServer
			if err := backendRows.Scan(&b.ID, &b.GroupID, &b.IP, &b.Port, &b.Weight, &b.IsHealthy, &b.LastCheckAt, &b.ResponseTimeMs); err != nil {
				writeError(w, http.StatusInternalServerError, "failed to read backend row")
				return
			}
			if idx, ok := groupIndex[b.GroupID]; ok {
				groups[idx].Backends = append(groups[idx].Backends, b)
			}
		}
	}

	writeJSON(w, http.StatusOK, groups)
}

type createServerGroupRequest struct {
	Nickname   string             `json:"nickname"`
	ColorHex   string             `json:"color_hex"`
	VIPAddress string             `json:"vip_address"`
	VIPPort    int                `json:"vip_port"`
	Algorithm  models.LBAlgorithm `json:"algorithm"`
}

func (s *Server) handleCreateServerGroup(w http.ResponseWriter, r *http.Request) {
	var req createServerGroupRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Nickname == "" || req.VIPAddress == "" || req.VIPPort == 0 {
		writeError(w, http.StatusBadRequest, "nickname, vip_address and vip_port are required")
		return
	}
	if req.ColorHex == "" {
		req.ColorHex = "#4f46e5"
	}
	if req.Algorithm != models.AlgoLeastConn {
		req.Algorithm = models.AlgoRoundRobin
	}

	ctx := r.Context()
	claims := claimsFromContext(r)

	var newID int64
	err := s.pool.QueryRow(ctx, `
		INSERT INTO server_groups (nickname, color_hex, vip_address, vip_port, algorithm)
		VALUES ($1, $2, $3, $4, $5) RETURNING id
	`, req.Nickname, req.ColorHex, req.VIPAddress, req.VIPPort, req.Algorithm).Scan(&newID)
	if err != nil {
		writeError(w, http.StatusConflict, "nickname or VIP address already in use")
		return
	}

	_ = s.recordAudit(ctx, claims, "server_group.create", "server_group", &newID, map[string]interface{}{
		"nickname": req.Nickname, "vip": req.VIPAddress,
	})

	writeJSON(w, http.StatusCreated, map[string]interface{}{"id": newID})
}

type updateServerGroupRequest struct {
	Nickname  *string             `json:"nickname,omitempty"`
	ColorHex  *string             `json:"color_hex,omitempty"`
	Algorithm *models.LBAlgorithm `json:"algorithm,omitempty"`
	IsActive  *bool               `json:"is_active,omitempty"`
}

func (s *Server) handleUpdateServerGroup(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid server group id")
		return
	}

	var req updateServerGroupRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	ctx := r.Context()
	claims := claimsFromContext(r)

	_, err = s.pool.Exec(ctx, `
		UPDATE server_groups SET
			nickname  = COALESCE($1, nickname),
			color_hex = COALESCE($2, color_hex),
			algorithm = COALESCE($3, algorithm),
			is_active = COALESCE($4, is_active)
		WHERE id = $5
	`, req.Nickname, req.ColorHex, req.Algorithm, req.IsActive, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update server group")
		return
	}

	_ = s.recordAudit(ctx, claims, "server_group.update", "server_group", &id, nil)
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (s *Server) handleDeleteServerGroup(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid server group id")
		return
	}

	ctx := r.Context()
	claims := claimsFromContext(r)

	tag, err := s.pool.Exec(ctx, `DELETE FROM server_groups WHERE id = $1`, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete server group")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "server group not found")
		return
	}

	_ = s.recordAudit(ctx, claims, "server_group.delete", "server_group", &id, nil)
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

type addBackendRequest struct {
	IP     string `json:"ip"`
	Port   int    `json:"port"`
	Weight int    `json:"weight"`
}

func (s *Server) handleAddBackend(w http.ResponseWriter, r *http.Request) {
	groupID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid server group id")
		return
	}

	var req addBackendRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.IP == "" || req.Port == 0 {
		writeError(w, http.StatusBadRequest, "ip and port are required")
		return
	}
	if req.Weight <= 0 {
		req.Weight = 1
	}

	ctx := r.Context()
	claims := claimsFromContext(r)

	var newID int64
	err = s.pool.QueryRow(ctx, `
		INSERT INTO backend_servers (group_id, ip, port, weight)
		VALUES ($1, $2, $3, $4) RETURNING id
	`, groupID, req.IP, req.Port, req.Weight).Scan(&newID)
	if err != nil {
		writeError(w, http.StatusConflict, "backend already exists in this group")
		return
	}

	_ = s.recordAudit(ctx, claims, "backend.add", "backend_server", &newID, map[string]interface{}{
		"group_id": groupID, "ip": req.IP, "port": req.Port,
	})

	writeJSON(w, http.StatusCreated, map[string]interface{}{"id": newID})
}

func (s *Server) handleDeleteBackend(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid backend id")
		return
	}

	ctx := r.Context()
	claims := claimsFromContext(r)

	tag, err := s.pool.Exec(ctx, `DELETE FROM backend_servers WHERE id = $1`, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete backend")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "backend not found")
		return
	}

	_ = s.recordAudit(ctx, claims, "backend.delete", "backend_server", &id, nil)
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
