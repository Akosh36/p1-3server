// Package config loads runtime configuration for every binary in this
// repository (cmd/api, cmd/netdiscd, cmd/fwctl, cmd/lbd, cmd/capd) from
// environment variables, keeping a single source of truth for defaults.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	// PostgreSQL
	DatabaseURL string

	// HTTP API
	HTTPAddr        string
	JWTSecret       string
	JWTTokenTTL     time.Duration
	AllowedOrigins  string

	// Data-plane daemon control sockets (Unix sockets, localhost-only)
	NetdiscSocket  string
	FwctlSocket    string
	LbdSocket      string
	CapdSocket     string

	// Paths
	CaptureDir string
}

func env(key, fallback string) string {
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

func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

// Load reads configuration from the environment, applying sane defaults for
// local development so `go run ./cmd/api` works out of the box against the
// dev Postgres instance created by `make dev-db`.
func Load() (*Config, error) {
	cfg := &Config{
		DatabaseURL:    env("DATABASE_URL", "postgres://p13server:devpassword@127.0.0.1:5432/p13server?sslmode=disable"),
		HTTPAddr:       env("HTTP_ADDR", ":8080"),
		JWTSecret:      env("JWT_SECRET", ""),
		JWTTokenTTL:    envDuration("JWT_TOKEN_TTL", 12*time.Hour),
		AllowedOrigins: env("ALLOWED_ORIGINS", "*"),
		NetdiscSocket:  env("NETDISC_SOCKET", "/run/p13server/netdiscd.sock"),
		FwctlSocket:    env("FWCTL_SOCKET", "/run/p13server/fwctl.sock"),
		LbdSocket:      env("LBD_SOCKET", "/run/p13server/lbd.sock"),
		CapdSocket:     env("CAPD_SOCKET", "/run/p13server/capd.sock"),
		CaptureDir:     env("CAPTURE_DIR", "/var/lib/p13server/captures"),
	}

	if cfg.JWTSecret == "" {
		return nil, fmt.Errorf("JWT_SECRET must be set (generate one with: openssl rand -hex 32)")
	}
	_ = envInt // reserved for future numeric env vars

	return cfg, nil
}
