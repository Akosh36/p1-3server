package httpapi

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/Akosh36/p1-3server/internal/auth"
	"github.com/Akosh36/p1-3server/internal/models"
)

func (s *Server) handleListAdmins(w http.ResponseWriter, r *http.Request) {
	rows, err := s.pool.Query(r.Context(), `
		SELECT id, username, role, is_active, created_by, created_at, last_login_at
		FROM admins ORDER BY created_at
	`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list admins")
		return
	}
	defer rows.Close()

	admins := []models.Admin{}
	for rows.Next() {
		var a models.Admin
		if err := rows.Scan(&a.ID, &a.Username, &a.Role, &a.IsActive, &a.CreatedBy, &a.CreatedAt, &a.LastLoginAt); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to read admin row")
			return
		}
		admins = append(admins, a)
	}
	writeJSON(w, http.StatusOK, admins)
}

type createAdminRequest struct {
	Username string           `json:"username"`
	Password string           `json:"password"`
	Role     models.AdminRole `json:"role"`
}

func (s *Server) handleCreateAdmin(w http.ResponseWriter, r *http.Request) {
	var req createAdminRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Username == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "username and password are required")
		return
	}
	if req.Role != models.RoleAdmin && req.Role != models.RoleSuperAdmin {
		req.Role = models.RoleAdmin
	}

	passwordHash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to hash password")
		return
	}

	claims := claimsFromContext(r)
	ctx := r.Context()

	var newID int64
	err = s.pool.QueryRow(ctx, `
		INSERT INTO admins (username, password_hash, role, created_by)
		VALUES ($1, $2, $3, $4) RETURNING id
	`, req.Username, passwordHash, req.Role, claims.AdminID).Scan(&newID)
	if err != nil {
		writeError(w, http.StatusConflict, "username already exists")
		return
	}

	_ = s.recordAudit(ctx, claims, "admin.create", "admin", &newID, map[string]interface{}{
		"username": req.Username, "role": req.Role,
	})

	writeJSON(w, http.StatusCreated, map[string]interface{}{"id": newID})
}

func (s *Server) handleDeleteAdmin(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid admin id")
		return
	}

	claims := claimsFromContext(r)
	if id == claims.AdminID {
		writeError(w, http.StatusBadRequest, "cannot delete your own account")
		return
	}

	ctx := r.Context()
	tag, err := s.pool.Exec(ctx, `DELETE FROM admins WHERE id = $1`, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete admin")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "admin not found")
		return
	}

	_ = s.recordAudit(ctx, claims, "admin.delete", "admin", &id, nil)
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
