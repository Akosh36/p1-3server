// Package httpapi implements the control-plane REST API served by cmd/api.
// It holds no special OS privileges: all privileged network operations are
// delegated to the data-plane daemons (netdiscd, fwctl, lbd, capd) over
// their local control sockets.
package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Akosh36/p1-3server/internal/config"
)

type Server struct {
	pool *pgxpool.Pool
	cfg  *config.Config
}

func New(pool *pgxpool.Pool, cfg *config.Config) *Server {
	return &Server{pool: pool, cfg: cfg}
}

func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{s.cfg.AllowedOrigins},
		AllowedMethods:   []string{"GET", "POST", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Authorization", "Content-Type"},
		AllowCredentials: true,
	}))

	r.Get("/api/health", s.handleHealth)
	r.Post("/api/auth/login", s.handleLogin)

	// Phase 5: cmd/backendagentd runs on a backend server, not as a logged-in
	// admin, so it authenticates with a per-backend agent_token (checked
	// inside the handler) instead of the JWT requireAuth group below.
	r.Post("/api/agent/metrics", s.handleAgentMetrics)

	r.Group(func(r chi.Router) {
		r.Use(s.requireAuth)

		r.Get("/api/auth/me", s.handleMe)

		r.Get("/api/devices", s.handleListDevices)
		r.Patch("/api/devices/{id}", s.handleUpdateDevice)

		r.Get("/api/switch-ports", s.handleListSwitchPorts)

		r.Get("/api/lan-networks", s.handleListLANNetworks)
		r.Patch("/api/lan-networks/{id}", s.handleUpdateLANNetwork)

		r.Get("/api/server-groups", s.handleListServerGroups)
		r.Post("/api/server-groups", s.handleCreateServerGroup)
		r.Patch("/api/server-groups/{id}", s.handleUpdateServerGroup)
		r.Delete("/api/server-groups/{id}", s.handleDeleteServerGroup)
		r.Post("/api/server-groups/{id}/backends", s.handleAddBackend)
		r.Delete("/api/backends/{id}", s.handleDeleteBackend)
		r.Post("/api/backends/{id}/regenerate-token", s.handleRegenerateBackendToken)
		r.Get("/api/backends/{id}/metrics", s.handleBackendMetricsHistory)

		r.Get("/api/metrics/self", s.handleSelfMetrics)
		r.Get("/api/metrics/history", s.handleMetricsHistory)

		r.Get("/api/audit-logs", s.handleListAuditLogs)

		r.Get("/api/firewall/status", s.handleFirewallStatus)

		r.Group(func(r chi.Router) {
			r.Use(s.requireSuperAdmin)
			r.Get("/api/admins", s.handleListAdmins)
			r.Post("/api/admins", s.handleCreateAdmin)
			r.Delete("/api/admins/{id}", s.handleDeleteAdmin)
		})
	})

	return r
}
