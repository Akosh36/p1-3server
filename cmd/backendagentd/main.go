// Command backendagentd is the Phase 5 metrics push-agent. Unlike
// netdiscd/fwctl/lbd, it does not run on this platform's gateway host at
// all — it's installed on each BACKEND server sitting behind a load-balancing
// VIP (Phase 4), wherever that server actually lives. It samples that
// host's own CPU/RAM/disk/network (internal/hostmetrics, the same sampler
// cmd/api uses for the gateway's own "Server" card) and pushes a sample to
// the control-plane API's /api/agent/metrics every interval, authenticated
// with the per-backend token shown in the Servers page when the backend was
// added (or regenerated).
//
// It never touches Postgres and knows nothing about the gateway's data-plane
// daemons — from its point of view the control-plane API is just an HTTP
// endpoint reachable over the network, which is why it takes a URL
// (AGENT_API_URL) rather than a local Unix socket path like the other
// daemons in this repo.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/Akosh36/p1-3server/internal/hostmetrics"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	apiURL := os.Getenv("AGENT_API_URL")
	token := os.Getenv("AGENT_TOKEN")
	if apiURL == "" || token == "" {
		slog.Error("AGENT_API_URL and AGENT_TOKEN must both be set")
		os.Exit(1)
	}
	interval := envDuration("AGENT_INTERVAL", 10*time.Second)
	endpoint := strings.TrimRight(apiURL, "/") + "/api/agent/metrics"

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	client := &http.Client{Timeout: 5 * time.Second}
	sampler := hostmetrics.NewSampler()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	slog.Info("backendagentd started", "endpoint", endpoint, "interval", interval)

	warnedUnreachable := false
	for {
		select {
		case <-ctx.Done():
			slog.Info("shutting down")
			return
		case now := <-ticker.C:
			sample, ok := sampler.Sample(now)
			if !ok {
				continue // first tick: no rate baseline yet
			}
			if err := push(ctx, client, endpoint, token, sample); err != nil {
				if !warnedUnreachable {
					slog.Warn("backendagentd: push failed, will keep retrying", "endpoint", endpoint, "error", err)
					warnedUnreachable = true
				}
			} else {
				warnedUnreachable = false
			}
		}
	}
}

func push(ctx context.Context, client *http.Client, endpoint, token string, s hostmetrics.Sample) error {
	body, err := json.Marshal(s)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("push returned status %d", resp.StatusCode)
	}
	return nil
}

func envDuration(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}
