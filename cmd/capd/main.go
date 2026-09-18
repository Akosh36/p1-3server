// Command capd is the on-demand packet-capture daemon (Phase 6). Like
// netdiscd/fwctl/lbd, it runs directly on the gateway host (root or
// CAP_NET_RAW — tcpdump needs to read raw frames off CAPD_INTERFACE) and
// never touches Postgres: the control-plane API's internal/capdsync package
// tells it which device (by MAC) to record into which file over a local
// Unix socket, and pulls live status/quota-eviction results back the same
// way.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/Akosh36/p1-3server/internal/capd"
	"github.com/Akosh36/p1-3server/internal/config"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	iface := os.Getenv("CAPD_INTERFACE")
	if iface == "" {
		slog.Error("CAPD_INTERFACE must be set to the LAN-facing interface to capture on")
		os.Exit(1)
	}

	rotateMB := envInt64("CAPD_ROTATE_MB", 50)
	rotateSeconds := envInt64("CAPD_ROTATE_SECONDS", 300)
	maxTotalMB := envInt64("CAPD_MAX_TOTAL_MB", 2048)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	manager := capd.NewManager(iface, cfg.CaptureDir, rotateMB*1024*1024, time.Duration(rotateSeconds)*time.Second, maxTotalMB*1024*1024)
	go manager.RunQuotaEnforcement(ctx)

	server, err := capd.ServeControl(manager, cfg.CapdSocket)
	if err != nil {
		slog.Error("failed to start control socket", "error", err)
		os.Exit(1)
	}
	slog.Info("capd listening", "socket", cfg.CapdSocket, "interface", iface, "capture_dir", cfg.CaptureDir,
		"rotate_mb", rotateMB, "rotate_seconds", rotateSeconds, "max_total_mb", maxTotalMB)

	<-ctx.Done()
	slog.Info("shutting down")
	manager.StopAll()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	server.Shutdown(shutdownCtx)
}

func envInt64(key string, fallback int64) int64 {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
	}
	return fallback
}
