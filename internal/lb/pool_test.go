package lb

import "testing"

func mkStates(backends ...Backend) map[string]*backendState {
	states := make(map[string]*backendState, len(backends))
	for _, b := range backends {
		states[b.addr()] = newBackendState(b)
	}
	return states
}

func TestBackendPool_RoundRobinCyclesThroughHealthyBackends(t *testing.T) {
	b1 := Backend{IP: "10.0.0.1", Port: 80}
	b2 := Backend{IP: "10.0.0.2", Port: 80}
	states := mkStates(b1, b2)
	pool := &backendPool{algorithm: "round_robin", backends: []*backendState{states[b1.addr()], states[b2.addr()]}}

	var picks []string
	for i := 0; i < 4; i++ {
		b, ok := pool.pick()
		if !ok {
			t.Fatalf("pick() returned no backend on iteration %d", i)
		}
		picks = append(picks, b.backend.IP)
	}

	want := []string{"10.0.0.1", "10.0.0.2", "10.0.0.1", "10.0.0.2"}
	for i := range want {
		if picks[i] != want[i] {
			t.Errorf("pick %d = %s, want %s (full sequence: %v)", i, picks[i], want[i], picks)
		}
	}
}

func TestBackendPool_SkipsUnhealthyBackends(t *testing.T) {
	b1 := Backend{IP: "10.0.0.1", Port: 80}
	b2 := Backend{IP: "10.0.0.2", Port: 80}
	states := mkStates(b1, b2)
	states[b1.addr()].recordCheck(false, 0)
	states[b1.addr()].recordCheck(false, 0) // 2 consecutive failures -> unhealthy

	pool := &backendPool{algorithm: "round_robin", backends: []*backendState{states[b1.addr()], states[b2.addr()]}}

	for i := 0; i < 5; i++ {
		b, ok := pool.pick()
		if !ok {
			t.Fatalf("pick() returned no backend on iteration %d", i)
		}
		if b.backend.IP != "10.0.0.2" {
			t.Errorf("pick %d = %s, want only the healthy backend 10.0.0.2", i, b.backend.IP)
		}
	}
}

func TestBackendPool_NoHealthyBackendsReturnsFalse(t *testing.T) {
	b1 := Backend{IP: "10.0.0.1", Port: 80}
	states := mkStates(b1)
	states[b1.addr()].recordCheck(false, 0)
	states[b1.addr()].recordCheck(false, 0)

	pool := &backendPool{algorithm: "round_robin", backends: []*backendState{states[b1.addr()]}}
	if _, ok := pool.pick(); ok {
		t.Fatal("expected pick() to fail when no backend is healthy")
	}
}

func TestBackendPool_LeastConnPicksFewestActiveConnections(t *testing.T) {
	b1 := Backend{IP: "10.0.0.1", Port: 80}
	b2 := Backend{IP: "10.0.0.2", Port: 80}
	states := mkStates(b1, b2)
	states[b1.addr()].activeConns.Store(5)
	states[b2.addr()].activeConns.Store(1)

	pool := &backendPool{algorithm: "least_conn", backends: []*backendState{states[b1.addr()], states[b2.addr()]}}
	b, ok := pool.pick()
	if !ok || b.backend.IP != "10.0.0.2" {
		t.Fatalf("expected least_conn to pick 10.0.0.2 (1 conn), got %+v ok=%v", b, ok)
	}
}

func TestBackendState_FlapAvoidance(t *testing.T) {
	s := newBackendState(Backend{IP: "10.0.0.1", Port: 80})
	if !s.isCurrentlyHealthy() {
		t.Fatal("a fresh backendState should start healthy (optimistic default)")
	}

	s.recordCheck(false, 0)
	if !s.isCurrentlyHealthy() {
		t.Fatal("a single failed check must not flip status yet (unhealthyAfterFailures=2)")
	}
	s.recordCheck(false, 0)
	if s.isCurrentlyHealthy() {
		t.Fatal("expected unhealthy after 2 consecutive failures")
	}

	s.recordCheck(true, 1.5)
	if !s.isCurrentlyHealthy() {
		t.Fatal("expected healthy again after a successful check (healthyAfterSuccesses=1)")
	}
}
