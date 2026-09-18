package lb

import (
	"context"
	"net"
	"time"
)

const healthCheckTimeout = 2 * time.Second

// runHealthChecks probes every backend in the *current* pool every
// interval, via a plain TCP connect (protocol-agnostic — lbd proxies raw
// TCP for "istalgan turdagi server", so it can't assume HTTP to check with
// a real request). It re-reads poolPtr every tick rather than closing over
// one pool snapshot, so a Sync that swaps in a new backend list (see
// Manager.Sync) takes effect on the very next tick without needing to
// restart this loop.
func runHealthChecks(ctx context.Context, poolPtr *atomicPoolPtr, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		for _, b := range poolPtr.Load().backends {
			go probeOnce(b)
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func probeOnce(b *backendState) {
	start := time.Now()
	conn, err := net.DialTimeout("tcp", b.backend.addr(), healthCheckTimeout)
	latencyMs := float64(time.Since(start).Microseconds()) / 1000.0
	if err != nil {
		b.recordCheck(false, 0)
		return
	}
	conn.Close()
	b.recordCheck(true, latencyMs)
}
