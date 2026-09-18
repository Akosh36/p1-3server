// Command lbd is the L4 TCP load balancer daemon (Phase 4). It runs
// directly on the gateway host — never in Docker — because a VIP only
// works if it's a real address on a real interface (see internal/lb/vip.go)
// and binding a non-loopback listener there needs that address to already
// exist locally. Like netdiscd and fwctl, it never touches Postgres: the
// control-plane API's internal/lbsync package pushes the desired
// groups/backends over a local Unix socket and pulls health results back
// the same way.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Akosh36/p1-3server/internal/config"
	"github.com/Akosh36/p1-3server/internal/lb"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	iface := os.Getenv("LBD_INTERFACE")
	if iface == "" {
		slog.Error("LBD_INTERFACE must be set to the LAN-facing interface VIPs attach to")
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	manager := lb.NewManager(iface, ctx)

	server, err := lb.ServeControl(manager, cfg.LbdSocket)
	if err != nil {
		slog.Error("failed to start control socket", "error", err)
		os.Exit(1)
	}
	slog.Info("lbd listening", "socket", cfg.LbdSocket, "interface", iface)

	<-ctx.Done()
	slog.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	server.Shutdown(shutdownCtx)
}
