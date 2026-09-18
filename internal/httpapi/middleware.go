package httpapi

import (
	"context"
	"net/http"
	"strings"

	"github.com/Akosh36/p1-3server/internal/auth"
	"github.com/Akosh36/p1-3server/internal/models"
)

type ctxKey string

const ctxKeyClaims ctxKey = "claims"

// requireAuth validates the Bearer JWT on every protected route. This is the
// first authentication layer (decision #7 in the architecture log); the
// second layer — restricting the admin panel to devices already holding an
// "admin" access_grant — is enforced upstream by nftables on the real
// deployment (see docs/deploy) once netdiscd/fwctl are wired in, and can
// additionally be checked here via requireAdminNetworkLocation once device
// data is populated.
//
// Phase 10 finding: a JWT's role/validity is a snapshot taken at login time.
// Trusting it for an admin's whole TTL (up to JWT_TOKEN_TTL) meant a
// just-deleted or just-deactivated admin's existing token kept working
// until it naturally expired — so every request re-checks is_active/role
// against Postgres instead of relying solely on the signed claims. This
// costs one indexed lookup per request, an acceptable tradeoff at this
// platform's scale (a handful of admins, not a high-QPS public API).
func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			writeError(w, http.StatusUnauthorized, "missing bearer token")
			return
		}
		tokenString := strings.TrimPrefix(header, "Bearer ")

		claims, err := auth.ParseToken(s.cfg.JWTSecret, tokenString)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "invalid or expired token")
			return
		}

		var isActive bool
		var role models.AdminRole
		err = s.pool.QueryRow(r.Context(), `SELECT is_active, role FROM admins WHERE id = $1`, claims.AdminID).Scan(&isActive, &role)
		if err != nil || !isActive {
			writeError(w, http.StatusUnauthorized, "invalid or expired token")
			return
		}
		claims.Role = role // authoritative now, not whatever the token was issued with

		ctx := context.WithValue(r.Context(), ctxKeyClaims, claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) requireSuperAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := r.Context().Value(ctxKeyClaims).(*auth.Claims)
		if !ok || claims.Role != models.RoleSuperAdmin {
			writeError(w, http.StatusForbidden, "super_admin role required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func claimsFromContext(r *http.Request) *auth.Claims {
	claims, _ := r.Context().Value(ctxKeyClaims).(*auth.Claims)
	return claims
}
