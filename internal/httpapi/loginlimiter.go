package httpapi

import (
	"sync"
	"time"
)

// loginFailureLimit/loginFailureWindow are decision #16's "aqlli standart
// qiymatlar" (smart defaults) applied to the login endpoint, not just
// nftables' DDoS thresholds: 5 failed attempts from one source IP within
// 5 minutes blocks further attempts from that IP until the window clears.
// A handful of admins on one LAN never legitimately need more than that to
// log in; a credential-stuffing or TOTP-brute-force script does.
const (
	loginFailureLimit   = 5
	loginFailureWindow  = 5 * time.Minute
	loginLimiterSweepAt = 1000 // map size that triggers a stale-entry sweep
)

// loginLimiter tracks recent failed login attempts per source IP, in
// memory only — this runs on one gateway host, not a fleet, so there is
// no shared-state store to keep in sync. Successes never count against an
// IP; only recordFailure does.
type loginLimiter struct {
	mu       sync.Mutex
	failures map[string][]time.Time
}

func newLoginLimiter() *loginLimiter {
	return &loginLimiter{failures: make(map[string][]time.Time)}
}

// allow reports whether ip may attempt another login right now.
func (l *loginLimiter) allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.pruneLocked(ip)
	if len(l.failures) > loginLimiterSweepAt {
		l.sweepLocked()
	}
	return len(l.failures[ip]) < loginFailureLimit
}

func (l *loginLimiter) recordFailure(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.pruneLocked(ip)
	l.failures[ip] = append(l.failures[ip], time.Now())
}

// pruneLocked drops ip's failures older than the window; callers hold mu.
func (l *loginLimiter) pruneLocked(ip string) {
	cutoff := time.Now().Add(-loginFailureWindow)
	existing := l.failures[ip]
	kept := existing[:0]
	for _, t := range existing {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) == 0 {
		delete(l.failures, ip)
	} else {
		l.failures[ip] = kept
	}
}

// sweepLocked bounds memory use: an IP that fails a few times and never
// returns would otherwise sit in the map forever. Only runs once the map
// has grown large enough to matter; callers hold mu.
func (l *loginLimiter) sweepLocked() {
	cutoff := time.Now().Add(-loginFailureWindow)
	for ip, times := range l.failures {
		kept := times[:0]
		for _, t := range times {
			if t.After(cutoff) {
				kept = append(kept, t)
			}
		}
		if len(kept) == 0 {
			delete(l.failures, ip)
		} else {
			l.failures[ip] = kept
		}
	}
}
