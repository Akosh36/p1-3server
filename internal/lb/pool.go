package lb

import (
	"sync"
	"sync/atomic"
	"time"
)

// unhealthyAfter/healthyAfter are the flap-avoidance thresholds: a backend
// needs this many consecutive failed/successful health checks before its
// status actually flips, so one transient blip doesn't yank it out of
// rotation and one lucky probe doesn't put a still-flaky backend back in.
const (
	unhealthyAfterFailures = 2
	healthyAfterSuccesses  = 1
)

// backendState is one backend's live, mutable state: health-check results
// and the active-connection count least_conn selects on. It outlives any
// single backendPool swap that still references the same IP:port, so a
// backend's health/connection history survives a sync that only reorders
// or re-weights the pool.
type backendState struct {
	backend Backend

	mu               sync.Mutex
	isHealthy        bool
	consecutiveFails int
	consecutiveOK    int
	lastCheckAt      time.Time
	responseTimeMs   float64

	activeConns atomic.Int64
}

func newBackendState(b Backend) *backendState {
	// Start optimistic (healthy) so a freshly (re)synced backend gets a
	// chance to take traffic immediately; the first failed check demotes
	// it just as fast as any other.
	return &backendState{backend: b, isHealthy: true}
}

func (s *backendState) recordCheck(ok bool, latencyMs float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastCheckAt = time.Now()
	if ok {
		s.responseTimeMs = latencyMs
		s.consecutiveOK++
		s.consecutiveFails = 0
		if s.consecutiveOK >= healthyAfterSuccesses {
			s.isHealthy = true
		}
	} else {
		s.consecutiveFails++
		s.consecutiveOK = 0
		if s.consecutiveFails >= unhealthyAfterFailures {
			s.isHealthy = false
		}
	}
}

func (s *backendState) health() BackendHealth {
	s.mu.Lock()
	defer s.mu.Unlock()
	var lastCheck string
	if !s.lastCheckAt.IsZero() {
		lastCheck = s.lastCheckAt.Format(time.RFC3339)
	}
	return BackendHealth{
		IP:             s.backend.IP,
		Port:           s.backend.Port,
		IsHealthy:      s.isHealthy,
		LastCheckAt:    lastCheck,
		ResponseTimeMs: s.responseTimeMs,
		ActiveConns:    s.activeConns.Load(),
	}
}

func (s *backendState) isCurrentlyHealthy() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.isHealthy
}

// backendPool is the swappable unit: a sync sets a fresh pool on the
// running listener without disturbing already-proxied connections
// (existing io.Copy goroutines hold their own backendState pointer, not
// the pool), and without needing the health-check history to reset for a
// backend that was already present in the previous pool — see Manager.Sync.
type backendPool struct {
	algorithm string
	backends  []*backendState
	rrCounter atomic.Uint64
}

// pick chooses a backend for a new connection, or (nil, false) if none of
// the pool's backends are currently healthy.
func (p *backendPool) pick() (*backendState, bool) {
	healthy := make([]*backendState, 0, len(p.backends))
	for _, b := range p.backends {
		if b.isCurrentlyHealthy() {
			healthy = append(healthy, b)
		}
	}
	if len(healthy) == 0 {
		return nil, false
	}

	if p.algorithm == "least_conn" {
		best := healthy[0]
		for _, b := range healthy[1:] {
			if b.activeConns.Load() < best.activeConns.Load() {
				best = b
			}
		}
		return best, true
	}

	// round_robin (default)
	idx := p.rrCounter.Add(1) - 1
	return healthy[idx%uint64(len(healthy))], true
}
