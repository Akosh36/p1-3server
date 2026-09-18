package firewall

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// Manager owns the currently-applied ruleset state and serializes every
// apply so two overlapping syncs can never race each other into the kernel.
type Manager struct {
	cfg RulesetConfig

	mu            sync.Mutex
	lastState     DesiredState
	lastAppliedAt time.Time
	lastError     string
}

func NewManager(cfg RulesetConfig) *Manager {
	return &Manager{cfg: cfg}
}

// ApplyBaseline loads a deny-everything ruleset (empty allow-lists). It
// must run before anything else on startup: until the control-plane's
// first sync arrives, the fail-safe state is "nobody is allowed", not
// "nftables hasn't been configured yet and everything passes through".
func (m *Manager) ApplyBaseline(ctx context.Context) error {
	return m.Sync(ctx, DesiredState{})
}

// Sync builds and atomically applies the ruleset for the given desired
// state. Safe to call concurrently; calls are serialized.
func (m *Manager) Sync(ctx context.Context, state DesiredState) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	ruleset := BuildRuleset(m.cfg, state)
	if err := Apply(ctx, ruleset); err != nil {
		m.lastError = err.Error()
		return fmt.Errorf("apply ruleset: %w", err)
	}

	m.lastState = state
	m.lastAppliedAt = time.Now()
	m.lastError = ""
	slog.Info("fwctl: ruleset applied", "user_macs", len(state.UserMACs), "admin_macs", len(state.AdminMACs))
	return nil
}

type Status struct {
	UserMACCount  int       `json:"user_mac_count"`
	AdminMACCount int       `json:"admin_mac_count"`
	LastAppliedAt time.Time `json:"last_applied_at"`
	LastError     string    `json:"last_error,omitempty"`
}

func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	return Status{
		UserMACCount:  len(m.lastState.UserMACs),
		AdminMACCount: len(m.lastState.AdminMACs),
		LastAppliedAt: m.lastAppliedAt,
		LastError:     m.lastError,
	}
}
