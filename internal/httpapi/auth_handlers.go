package httpapi

import (
	"net/http"

	"github.com/Akosh36/p1-3server/internal/auth"
	"github.com/Akosh36/p1-3server/internal/models"
)

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	TOTPCode string `json:"totp_code,omitempty"`
}

type loginResponse struct {
	Token string       `json:"token"`
	Admin models.Admin `json:"admin"`
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Username == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "username and password are required")
		return
	}

	ctx := r.Context()
	var admin models.Admin
	err := s.pool.QueryRow(ctx, `
		SELECT id, username, password_hash, totp_secret, role, is_active
		FROM admins WHERE username = $1
	`, req.Username).Scan(&admin.ID, &admin.Username, &admin.PasswordHash, &admin.TOTPSecret, &admin.Role, &admin.IsActive)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid username or password")
		return
	}

	if !admin.IsActive {
		writeError(w, http.StatusForbidden, "account disabled")
		return
	}
	if !auth.CheckPassword(admin.PasswordHash, req.Password) {
		writeError(w, http.StatusUnauthorized, "invalid username or password")
		return
	}
	if admin.TOTPSecret != nil && *admin.TOTPSecret != "" {
		if req.TOTPCode == "" || !auth.ValidateTOTP(*admin.TOTPSecret, req.TOTPCode) {
			writeError(w, http.StatusUnauthorized, "valid 2FA code required")
			return
		}
	}

	token, err := auth.IssueToken(s.cfg.JWTSecret, admin.ID, admin.Role, s.cfg.JWTTokenTTL)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to issue token")
		return
	}

	if _, err := s.pool.Exec(ctx, `UPDATE admins SET last_login_at = now() WHERE id = $1`, admin.ID); err != nil {
		// Non-fatal: login still succeeds even if we fail to record the timestamp.
		_ = err
	}

	admin.PasswordHash = ""
	writeJSON(w, http.StatusOK, loginResponse{Token: token, Admin: admin})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	claims := claimsFromContext(r)
	ctx := r.Context()

	var admin models.Admin
	err := s.pool.QueryRow(ctx, `
		SELECT id, username, role, is_active, created_at, last_login_at
		FROM admins WHERE id = $1
	`, claims.AdminID).Scan(&admin.ID, &admin.Username, &admin.Role, &admin.IsActive, &admin.CreatedAt, &admin.LastLoginAt)
	if err != nil {
		writeError(w, http.StatusNotFound, "admin not found")
		return
	}
	writeJSON(w, http.StatusOK, admin)
}
