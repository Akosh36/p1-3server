// Command netdiscd is the LAN device-discovery daemon (Phase 1). It runs
// directly on the gateway host — never in Docker — because it needs to read
// the kernel's ARP table, dnsmasq's lease file, and (optionally) hostapd's
// control socket. It never touches Postgres itself: it only publishes a
// point-in-time snapshot over a local Unix socket, which cmd/api's
// internal/discovery package pulls and reconciles into the devices and
// switch_ports tables. See CLAUDE.md section 4 for why the split exists.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Akosh36/p1-3server/internal/config"
	"github.com/Akosh36/p1-3server/internal/netdisc"
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

	store := netdisc.NewStore()

	server, err := netdisc.ServeSnapshot(store, cfg.NetdiscSocket)
	if err != nil {
		slog.Error("failed to start snapshot server", "error", err)
		os.Exit(1)
	}
	slog.Info("netdiscd listening", "socket", cfg.NetdiscSocket)

	go netdisc.RunARPCollector(ctx, store, envDuration("NETDISC_ARP_INTERVAL", 5*time.Second))

	go netdisc.RunDNSMasqCollector(ctx, store,
		envString("NETDISC_DNSMASQ_LEASE_FILE", "/var/lib/misc/dnsmasq.leases"),
		envDuration("NETDISC_DNSMASQ_INTERVAL", 10*time.Second))

	go netdisc.RunSNMPCollector(ctx, store, netdisc.SNMPConfig{
		SwitchName: envString("NETDISC_SNMP_SWITCH_NAME", "switch1"),
		Target:     envString("NETDISC_SNMP_TARGET", ""),
		Community:  envString("NETDISC_SNMP_COMMUNITY", "public"),
	}, envDuration("NETDISC_SNMP_INTERVAL", 15*time.Second))

	go netdisc.RunHostapdCollector(ctx, store,
		envString("NETDISC_HOSTAPD_SOCKET_DIR", ""),
		envString("NETDISC_HOSTAPD_SSID", ""),
		envDuration("NETDISC_HOSTAPD_INTERVAL", 5*time.Second))

	staleAfter := envDuration("NETDISC_STALE_AFTER", 90*time.Second)
	pruneInterval := envDuration("NETDISC_PRUNE_INTERVAL", 10*time.Second)
	go runPruner(ctx, store, staleAfter, pruneInterval)

	<-ctx.Done()
	slog.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	server.Shutdown(shutdownCtx)
}

func runPruner(ctx context.Context, store *netdisc.Store, staleAfter, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			store.Prune(staleAfter, now)
		}
	}
}

func envString(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envDuration(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}
