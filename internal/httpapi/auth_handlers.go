package httpapi

import (
	"context"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

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

// handleLogin is the only endpoint on the whole API an unauthenticated
// caller can push credentials into, so it carries three Phase 10 additions
// no other handler needs: a per-source-IP failure limiter (nothing
// previously stopped unlimited password/TOTP guessing), an audit_logs
// entry for every attempt — success or failure — so a brute-force run
// leaves a real trail (previously only last_login_at existed, which just
// gets silently overwritten by whichever attempt succeeds), and enforcing
// decision #7's IP/MAC login pin, which existed as admins.allowed_ip/
// allowed_mac columns from Phase 0 but was never actually checked anywhere.
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if !s.loginLimiter.allow(ip) {
		writeError(w, http.StatusTooManyRequests, "too many failed login attempts — try again in a few minutes")
		return
	}

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

	// fail logs the real reason to audit_logs (targetID is nil when the
	// username itself never matched a row) but always counts against the
	// rate limiter; the caller still chooses what the client sees.
	fail := func(reason string, targetID *int64) {
		s.loginLimiter.recordFailure(ip)
		_ = s.recordAudit(ctx, nil, "auth.login_failed", "admin", targetID, map[string]interface{}{
			"username": req.Username, "reason": reason, "ip": ip,
		})
	}

	var admin models.Admin
	var ipAllowed bool
	err := s.pool.QueryRow(ctx, `
		SELECT id, username, password_hash, totp_secret, role, is_active,
		       host(allowed_ip), allowed_mac::text,
		       (allowed_ip IS NULL OR $2::inet <<= allowed_ip) AS ip_allowed
		FROM admins WHERE username = $1
	`, req.Username, ip).Scan(&admin.ID, &admin.Username, &admin.PasswordHash, &admin.TOTPSecret, &admin.Role, &admin.IsActive,
		&admin.AllowedIP, &admin.AllowedMAC, &ipAllowed)
	if err != nil {
		fail("unknown username", nil)
		writeError(w, http.StatusUnauthorized, "invalid username or password")
		return
	}

	// Deliberately the same generic message as "unknown username" (and
	// below, "wrong password") rather than the old distinct "account
	// disabled" — that used to let an unauthenticated caller learn a
	// username exists and is disabled just by trying it.
	if !admin.IsActive {
		fail("account disabled", &admin.ID)
		writeError(w, http.StatusUnauthorized, "invalid username or password")
		return
	}
	if !auth.CheckPassword(admin.PasswordHash, req.Password) {
		fail("wrong password", &admin.ID)
		writeError(w, http.StatusUnauthorized, "invalid username or password")
		return
	}
	// From here on the caller has already proven the password — telling
	// them specifically why they're still blocked (2FA, network/device
	// pin) isn't an enumeration risk the way it would be before this
	// point, and it's the only way a legitimate admin can tell what to
	// fix. This matches the pre-existing TOTP message's behavior.
	if admin.TOTPSecret != nil && *admin.TOTPSecret != "" {
		if req.TOTPCode == "" || !auth.ValidateTOTP(*admin.TOTPSecret, req.TOTPCode) {
			fail("invalid or missing 2FA code", &admin.ID)
			writeError(w, http.StatusUnauthorized, "valid 2FA code required")
			return
		}
	}
	if !ipAllowed {
		fail("source IP outside allowed_ip", &admin.ID)
		writeError(w, http.StatusForbidden, "this account cannot log in from this network")
		return
	}
	if admin.AllowedMAC != nil {
		if !macAllowed(ctx, s.pool, ip, *admin.AllowedMAC) {
			fail("source device MAC not allowed", &admin.ID)
			writeError(w, http.StatusForbidden, "this account cannot log in from this device")
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

	_ = s.recordAudit(ctx, &auth.Claims{AdminID: admin.ID, Role: admin.Role}, "auth.login_success", "admin", &admin.ID, map[string]interface{}{"ip": ip})

	admin.PasswordHash = ""
	writeJSON(w, http.StatusOK, loginResponse{Token: token, Admin: admin})
}

// macAllowed resolves the MAC address netdiscd's own ARP discovery
// (Phase 1) most recently associated with ip via the devices table — the
// control-plane API has no host network privileges of its own to read
// /proc/net/arp directly (see CLAUDE.md §4's privilege split), so it reads
// the same real ARP-derived data discovery already wrote to Postgres.
// An IP netdiscd has never seen fails closed: with allowed_mac configured,
// an unresolvable device can't be verified, so it isn't trusted.
func macAllowed(ctx context.Context, pool *pgxpool.Pool, ip, allowedMAC string) bool {
	var deviceMAC string
	err := pool.QueryRow(ctx, `SELECT mac_address::text FROM devices WHERE host(ip_address) = $1`, ip).Scan(&deviceMAC)
	if err != nil {
		return false
	}
	return strings.EqualFold(deviceMAC, allowedMAC)
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
