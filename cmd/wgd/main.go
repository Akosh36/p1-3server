// Command wgd is the site-to-site WireGuard daemon (Phase 7). Like
// netdiscd/fwctl/lbd/capd, it runs directly on the gateway host (root or
// CAP_NET_ADMIN — creating and configuring a WireGuard interface needs it)
// and never touches Postgres: the control-plane API's internal/wgsync
// package tells it the desired remote-peer list over a local Unix socket,
// and pulls live handshake status back the same way.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/Akosh36/p1-3server/internal/config"
	"github.com/Akosh36/p1-3server/internal/wg"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	iface := envString("WGD_INTERFACE", "wg0")
	address := os.Getenv("WGD_ADDRESS")
	if address == "" {
		slog.Error("WGD_ADDRESS must be set to this gateway's own CIDR address on the WireGuard tunnel subnet (e.g. 10.99.0.1/24)")
		os.Exit(1)
	}
	listenPort := envInt("WGD_LISTEN_PORT", 51820)
	privateKeyPath := envString("WGD_PRIVATE_KEY_PATH", "/var/lib/p13server/wireguard/private.key")
	reachableAfter := time.Duration(envInt("WGD_REACHABLE_AFTER_SECONDS", 150)) * time.Second

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	manager := wg.NewManager(iface, address, listenPort, privateKeyPath, reachableAfter)
	if err := manager.Start(); err != nil {
		slog.Error("failed to start WireGuard interface", "error", err)
		os.Exit(1)
	}

	server, err := wg.ServeControl(manager, cfg.WgdSocket)
	if err != nil {
		slog.Error("failed to start control socket", "error", err)
		os.Exit(1)
	}
	slog.Info("wgd listening", "socket", cfg.WgdSocket, "interface", iface, "address", address, "listen_port", listenPort)

	<-ctx.Done()
	slog.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	server.Shutdown(shutdownCtx)
	manager.Stop()
}

func envString(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}
