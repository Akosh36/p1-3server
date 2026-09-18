package httpapi

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Akosh36/p1-3server/internal/auth"
	"github.com/Akosh36/p1-3server/internal/models"
)

var errInvalidAllowedIP = errors.New("must be a valid IP address or CIDR range")

// isLastActiveSuperAdmin reports whether id is the only remaining active
// super_admin — deleting, deactivating, or demoting that account would
// leave nobody able to reach any requireSuperAdmin endpoint (including
// this one), permanently locking the whole panel out of admin management.
func isLastActiveSuperAdmin(ctx context.Context, pool *pgxpool.Pool, id int64) (bool, error) {
	var isTargetActiveSuperAdmin bool
	err := pool.QueryRow(ctx, `
		SELECT role = 'super_admin' AND is_active FROM admins WHERE id = $1
	`, id).Scan(&isTargetActiveSuperAdmin)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil // nonexistent admin — the caller's own 404 check handles this
	}
	if err != nil {
		return false, err
	}
	if !isTargetActiveSuperAdmin {
		return false, nil
	}

	var othersRemain bool
	if err := pool.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM admins WHERE role = 'super_admin' AND is_active AND id != $1)
	`, id).Scan(&othersRemain); err != nil {
		return false, err
	}
	return !othersRemain, nil
}

func validateAllowedIP(s string) error {
	if _, _, err := net.ParseCIDR(s); err == nil {
		return nil
	}
	if ip := net.ParseIP(s); ip != nil {
		return nil
	}
	return errInvalidAllowedIP
}

func validateAllowedMAC(s string) error {
	_, err := net.ParseMAC(s)
	return err
}

func (s *Server) handleListAdmins(w http.ResponseWriter, r *http.Request) {
	rows, err := s.pool.Query(r.Context(), `
		SELECT id, username, role, host(allowed_ip), allowed_mac::text, is_active, created_by, created_at, last_login_at
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
		if err := rows.Scan(&a.ID, &a.Username, &a.Role, &a.AllowedIP, &a.AllowedMAC, &a.IsActive, &a.CreatedBy, &a.CreatedAt, &a.LastLoginAt); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to read admin row")
			return
		}
		admins = append(admins, a)
	}
	writeJSON(w, http.StatusOK, admins)
}

type createAdminRequest struct {
	Username   string           `json:"username"`
	Password   string           `json:"password"`
	Role       models.AdminRole `json:"role"`
	AllowedIP  *string          `json:"allowed_ip,omitempty"`
	AllowedMAC *string          `json:"allowed_mac,omitempty"`
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
	if len(req.Password) < auth.MinPasswordLength {
		writeError(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}
	if req.Role != models.RoleAdmin && req.Role != models.RoleSuperAdmin {
		req.Role = models.RoleAdmin
	}
	if req.AllowedIP != nil && *req.AllowedIP != "" {
		if err := validateAllowedIP(*req.AllowedIP); err != nil {
			writeError(w, http.StatusBadRequest, "allowed_ip: "+err.Error())
			return
		}
	} else {
		req.AllowedIP = nil
	}
	if req.AllowedMAC != nil && *req.AllowedMAC != "" {
		if err := validateAllowedMAC(*req.AllowedMAC); err != nil {
			writeError(w, http.StatusBadRequest, "allowed_mac: "+err.Error())
			return
		}
	} else {
		req.AllowedMAC = nil
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
		INSERT INTO admins (username, password_hash, role, allowed_ip, allowed_mac, created_by)
		VALUES ($1, $2, $3, $4, $5, $6) RETURNING id
	`, req.Username, passwordHash, req.Role, req.AllowedIP, req.AllowedMAC, claims.AdminID).Scan(&newID)
	if err != nil {
		writeError(w, http.StatusConflict, "username already exists")
		return
	}

	_ = s.recordAudit(ctx, claims, "admin.create", "admin", &newID, map[string]interface{}{
		"username": req.Username, "role": req.Role,
	})

	writeJSON(w, http.StatusCreated, map[string]interface{}{"id": newID})
}

type updateAdminRequest struct {
	Role       *models.AdminRole `json:"role,omitempty"`
	IsActive   *bool             `json:"is_active,omitempty"`
	Password   *string           `json:"password,omitempty"`
	AllowedIP  *string           `json:"allowed_ip,omitempty"` // "" clears the restriction
	AllowedMAC *string           `json:"allowed_mac,omitempty"`
}

// handleUpdateAdmin is the Phase 10 addition that makes an admin account
// actually manageable after creation: deactivating instead of deleting
// (preserving audit_logs.actor_admin_id's link to their past actions,
// which a DELETE would null out), rotating a forgotten/leaked password,
// changing role, and setting the decision-#7 IP/MAC login pin that had no
// way to be set before this phase. Every change here takes effect on the
// account's very next request, not just its next login — see requireAuth,
// which re-reads is_active/role from Postgres on every call instead of
// trusting a JWT until it expires.
func (s *Server) handleUpdateAdmin(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid admin id")
		return
	}

	var req updateAdminRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Role != nil && *req.Role != models.RoleAdmin && *req.Role != models.RoleSuperAdmin {
		writeError(w, http.StatusBadRequest, "role must be 'admin' or 'super_admin'")
		return
	}

	claims := claimsFromContext(r)
	ctx := r.Context()

	demotesOrDeactivates := (req.IsActive != nil && !*req.IsActive) || (req.Role != nil && *req.Role != models.RoleSuperAdmin)
	if demotesOrDeactivates {
		if id == claims.AdminID {
			writeError(w, http.StatusBadRequest, "cannot deactivate or demote your own account")
			return
		}
		last, err := isLastActiveSuperAdmin(ctx, s.pool, id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to check admin role")
			return
		}
		if last {
			writeError(w, http.StatusConflict, "cannot deactivate or demote the last active super_admin")
			return
		}
	}

	var passwordHash *string
	if req.Password != nil {
		if len(*req.Password) < auth.MinPasswordLength {
			writeError(w, http.StatusBadRequest, "password must be at least 8 characters")
			return
		}
		h, err := auth.HashPassword(*req.Password)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to hash password")
			return
		}
		passwordHash = &h
	}

	var allowedIP *string
	if req.AllowedIP != nil {
		if *req.AllowedIP == "" {
			allowedIP = nil
		} else {
			if err := validateAllowedIP(*req.AllowedIP); err != nil {
				writeError(w, http.StatusBadRequest, "allowed_ip: "+err.Error())
				return
			}
			allowedIP = req.AllowedIP
		}
	}
	var allowedMAC *string
	if req.AllowedMAC != nil {
		if *req.AllowedMAC == "" {
			allowedMAC = nil
		} else {
			if err := validateAllowedMAC(*req.AllowedMAC); err != nil {
				writeError(w, http.StatusBadRequest, "allowed_mac: "+err.Error())
				return
			}
			allowedMAC = req.AllowedMAC
		}
	}

	tag, err := s.pool.Exec(ctx, `
		UPDATE admins SET
			role          = COALESCE($2::admin_role, role),
			is_active     = COALESCE($3::boolean, is_active),
			password_hash = COALESCE($4::text, password_hash),
			allowed_ip    = CASE WHEN $5::boolean THEN $6::inet ELSE allowed_ip END,
			allowed_mac   = CASE WHEN $7::boolean THEN $8::macaddr ELSE allowed_mac END
		WHERE id = $1
	`, id, req.Role, req.IsActive, passwordHash,
		req.AllowedIP != nil, allowedIP,
		req.AllowedMAC != nil, allowedMAC)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update admin")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "admin not found")
		return
	}

	_ = s.recordAudit(ctx, claims, "admin.update", "admin", &id, map[string]interface{}{
		"role": req.Role, "is_active": req.IsActive,
		"password_changed": req.Password != nil,
		"allowed_ip_set":   req.AllowedIP != nil, "allowed_mac_set": req.AllowedMAC != nil,
	})

	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
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

	if last, err := isLastActiveSuperAdmin(r.Context(), s.pool, id); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to check admin role")
		return
	} else if last {
		writeError(w, http.StatusConflict, "cannot delete the last active super_admin")
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
