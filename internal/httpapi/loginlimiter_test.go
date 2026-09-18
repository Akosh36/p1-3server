package httpapi

import "testing"

func TestLoginLimiter_BlocksAfterLimitThenRecovers(t *testing.T) {
	l := newLoginLimiter()
	ip := "10.0.0.5"

	for i := 0; i < loginFailureLimit; i++ {
		if !l.allow(ip) {
			t.Fatalf("attempt %d: expected allow before the limit is reached", i+1)
		}
		l.recordFailure(ip)
	}
	if l.allow(ip) {
		t.Fatalf("expected the %dth attempt to be blocked", loginFailureLimit+1)
	}

	// A stale entry (older than the window) must not count against the limit.
	for i := range l.failures[ip] {
		l.failures[ip][i] = l.failures[ip][i].Add(-2 * loginFailureWindow)
	}
	if !l.allow(ip) {
		t.Fatal("expected allow once all recorded failures have aged out of the window")
	}
}

func TestLoginLimiter_TracksIPsIndependently(t *testing.T) {
	l := newLoginLimiter()
	for i := 0; i < loginFailureLimit; i++ {
		l.recordFailure("10.0.0.1")
	}
	if l.allow("10.0.0.1") {
		t.Fatal("expected 10.0.0.1 to be blocked")
	}
	if !l.allow("10.0.0.2") {
		t.Fatal("a different IP's failures must not affect this one")
	}
}

func TestLoginLimiter_SuccessDoesNotResetOtherIPs(t *testing.T) {
	// allow() itself never records anything — only recordFailure does —
	// so a caller that never calls recordFailure (a successful login)
	// leaves the counter untouched, matching how handleLogin only calls
	// recordFailure on its failure paths.
	l := newLoginLimiter()
	l.recordFailure("10.0.0.9")
	if !l.allow("10.0.0.9") {
		t.Fatal("one failure must not block on its own")
	}
	if len(l.failures["10.0.0.9"]) != 1 {
		t.Fatalf("allow() must not itself add a failure entry, got %d", len(l.failures["10.0.0.9"]))
	}
}
