// Command api runs the control-plane REST API: authentication, RBAC, and
// CRUD over devices/admins/server groups/LAN networks. It holds no special
// OS privileges — see docs/deploy for how it relates to the privileged
// data-plane daemons (netdiscd, fwctl, lbd, capd, wgd).
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Akosh36/p1-3server/internal/aclsync"
	"github.com/Akosh36/p1-3server/internal/auth"
	"github.com/Akosh36/p1-3server/internal/capdsync"
	"github.com/Akosh36/p1-3server/internal/config"
	"github.com/Akosh36/p1-3server/internal/db"
	"github.com/Akosh36/p1-3server/internal/discovery"
	"github.com/Akosh36/p1-3server/internal/httpapi"
	"github.com/Akosh36/p1-3server/internal/lbsync"
	"github.com/Akosh36/p1-3server/internal/metrics"
	"github.com/Akosh36/p1-3server/internal/models"
	"github.com/Akosh36/p1-3server/internal/wgsync"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := db.Migrate(ctx, pool); err != nil {
		slog.Error("failed to run migrations", "error", err)
		os.Exit(1)
	}
	slog.Info("migrations applied")

	if err := bootstrapSuperAdmin(ctx, pool); err != nil {
		slog.Error("failed to bootstrap super admin", "error", err)
		os.Exit(1)
	}

	go metrics.Run(ctx, pool, 10*time.Second)
	go discovery.Run(ctx, pool, cfg.NetdiscSocket, 5*time.Second)
	go aclsync.Run(ctx, pool, cfg.FwctlSocket, 2*time.Second)
	go lbsync.Run(ctx, pool, cfg.LbdSocket, 3*time.Second)
	go capdsync.Run(ctx, pool, cfg.CapdSocket, cfg.CaptureDir, 2*time.Second)
	go wgsync.Run(ctx, pool, cfg.WgdSocket, 3*time.Second)

	srv := httpapi.New(pool, cfg)
	httpServer := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           srv.Router(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		slog.Info("api server listening", "addr", cfg.HTTPAddr)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("http server error", "error", err)
			stop()
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		slog.Error("graceful shutdown failed", "error", err)
	}
}

// bootstrapSuperAdmin creates the first super_admin account from
// BOOTSTRAP_ADMIN_USERNAME / BOOTSTRAP_ADMIN_PASSWORD when the admins table
// is empty, so a fresh deployment has a way to log in at all. It is a no-op
// once at least one admin exists.
func bootstrapSuperAdmin(ctx context.Context, pool *pgxpool.Pool) error {
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM admins`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	username := os.Getenv("BOOTSTRAP_ADMIN_USERNAME")
	password := os.Getenv("BOOTSTRAP_ADMIN_PASSWORD")
	if username == "" || password == "" {
		slog.Warn("no admins exist yet and BOOTSTRAP_ADMIN_USERNAME/BOOTSTRAP_ADMIN_PASSWORD are unset; set them and restart to create the first super_admin")
		return nil
	}

	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}

	_, err = pool.Exec(ctx, `
		INSERT INTO admins (username, password_hash, role) VALUES ($1, $2, $3)
	`, username, hash, models.RoleSuperAdmin)
	if err != nil {
		return err
	}

	slog.Info("bootstrapped first super_admin", "username", username)
	return nil
}
