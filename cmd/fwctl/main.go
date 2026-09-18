// Command fwctl is the nftables ACL enforcement daemon (Phase 2). It runs
// directly on the gateway host — never in Docker — because it needs
// CAP_NET_ADMIN to load firewall rules. Like netdiscd, it never touches
// Postgres: it applies a deny-everything baseline the instant it starts,
// then waits for the control-plane API's internal/aclsync package to push
// the real allow-lists over a local Unix socket.
//
// On shutdown it deliberately leaves the last-applied ruleset in the
// kernel rather than tearing it down — a crashed or restarting fwctl must
// never mean "default-deny is now off".
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Akosh36/p1-3server/internal/config"
	"github.com/Akosh36/p1-3server/internal/firewall"
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

	rulesetCfg := firewall.RulesetConfig{
		TableName:       envString("FWCTL_TABLE_NAME", "p13server"),
		ManagementPorts: envIntList("FWCTL_MANAGEMENT_PORTS", []int{22, 8080}),
		WANInterface:    envString("FWCTL_WAN_INTERFACE", ""),
	}

	if rulesetCfg.WANInterface != "" {
		// Gateway mode (Phase 3): without this, nftables' forward chain
		// never even sees LAN->internet packets — the kernel drops them
		// before the netfilter forward hook runs at all.
		if err := firewall.EnableIPForwarding(); err != nil {
			slog.Error("failed to enable IP forwarding — refusing to start in gateway mode without it", "error", err)
			os.Exit(1)
		}
		slog.Info("fwctl: gateway mode enabled", "wan_interface", rulesetCfg.WANInterface)
	}

	manager := firewall.NewManager(rulesetCfg)

	baselineCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	if err := manager.ApplyBaseline(baselineCtx); err != nil {
		cancel()
		slog.Error("failed to apply deny-all baseline ruleset — refusing to start without it", "error", err)
		os.Exit(1)
	}
	cancel()
	slog.Info("fwctl: deny-all baseline applied, table", "table", rulesetCfg.TableName)

	server, err := firewall.ServeControl(manager, cfg.FwctlSocket)
	if err != nil {
		slog.Error("failed to start control socket", "error", err)
		os.Exit(1)
	}
	slog.Info("fwctl listening", "socket", cfg.FwctlSocket)

	<-ctx.Done()
	slog.Info("shutting down (firewall ruleset stays applied)")
	shutdownCtx, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel2()
	server.Shutdown(shutdownCtx)
}

func envString(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// envIntList parses a comma-separated env var like "22,8080" into []int.
func envIntList(key string, fallback []int) []int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	var out []int
	for _, part := range strings.Split(v, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil {
			continue
		}
		out = append(out, n)
	}
	if len(out) == 0 {
		return fallback
	}
	return out
}
