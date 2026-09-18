package lb

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

const healthCheckInterval = 3 * time.Second

// runningGroup is one live VIP: its listener, its swappable backend pool,
// and the backendState objects currently reachable through that pool
// (kept keyed by address so a later Sync can reuse them instead of
// resetting health/connection history for a backend that's still there).
type runningGroup struct {
	group        Group
	listener     net.Listener
	poolPtr      *atomicPoolPtr
	cancelHealth context.CancelFunc
	states       map[string]*backendState // key: "ip:port"
}

// Manager owns every currently-running VIP listener on this host. Like
// firewall.Manager (fwctl) and netdisc.Store (netdiscd), it never touches
// Postgres — internal/lbsync is what feeds it, over lbd's Unix socket.
type Manager struct {
	iface   string          // interface VIPs are added to (see vip.go)
	baseCtx context.Context // long-lived: outlives any single Sync call

	mu      sync.Mutex
	running map[string]*runningGroup // key: Group.key() ("vip:port")

	trafficMu sync.Mutex
	traffic   map[trafficKey]*trafficCounter
}

type trafficKey struct {
	vipAddress string
	clientIP   string
}

type trafficCounter struct {
	bytesUp   atomic.Int64
	bytesDown atomic.Int64
}

// NewManager takes baseCtx separately from the ctx each Sync call receives:
// a listener/health-check goroutine started during a sync must keep running
// after that specific call returns, so it must NOT be derived from a
// request-scoped context (ServeControl's /sync handler passes r.Context(),
// which net/http cancels the instant the HTTP response is written — using
// that as the listener's lifetime context was a real bug caught by the
// ip-netns test: the VIP listener closed itself right after every sync).
func NewManager(iface string, baseCtx context.Context) *Manager {
	return &Manager{
		iface:   iface,
		baseCtx: baseCtx,
		running: make(map[string]*runningGroup),
		traffic: make(map[trafficKey]*trafficCounter),
	}
}

// recordTraffic accumulates one finished connection's byte counts, keyed
// by the VIP it came through and the client's IP — passed into every
// listener started by startLocked as their trafficRecorder.
func (m *Manager) recordTraffic(vipAddress, clientIP string, bytesUp, bytesDown int64) {
	if bytesUp == 0 && bytesDown == 0 {
		return
	}
	m.trafficMu.Lock()
	defer m.trafficMu.Unlock()
	key := trafficKey{vipAddress: vipAddress, clientIP: clientIP}
	c, ok := m.traffic[key]
	if !ok {
		c = &trafficCounter{}
		m.traffic[key] = c
	}
	c.bytesUp.Add(bytesUp)
	c.bytesDown.Add(bytesDown)
}

// DrainTraffic returns every client's accumulated traffic since the last
// call and resets the counters — internal/lbsync polls this on its normal
// tick and is the only thing that ever sees a given byte count, so
// draining (not just reading) here is what keeps the two in sync without
// needing lbd to track "already reported" state itself.
func (m *Manager) DrainTraffic() []ClientTraffic {
	m.trafficMu.Lock()
	defer m.trafficMu.Unlock()

	entries := make([]ClientTraffic, 0, len(m.traffic))
	for k, c := range m.traffic {
		entries = append(entries, ClientTraffic{
			VIPAddress: k.vipAddress,
			ClientIP:   k.clientIP,
			BytesUp:    c.bytesUp.Load(),
			BytesDown:  c.bytesDown.Load(),
		})
	}
	m.traffic = make(map[trafficKey]*trafficCounter)
	return entries
}

// Sync reconciles the running listeners against the desired groups:
// unchanged VIPs keep their listener and reuse backendState for backends
// that are still present (preserving health/connection history); new
// groups get a VIP address + listener; removed groups are torn down and
// their VIP address released. Safe to call concurrently with itself
// (serialized) and with Status().
func (m *Manager) Sync(ctx context.Context, groups []Group) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	desired := make(map[string]Group, len(groups))
	for _, g := range groups {
		desired[g.key()] = g
	}

	// Tear down anything no longer desired.
	for key, rg := range m.running {
		if _, keep := desired[key]; !keep {
			m.stopLocked(ctx, key, rg)
		}
	}

	var errs []error
	for key, g := range desired {
		if rg, exists := m.running[key]; exists {
			m.updatePoolLocked(rg, g)
			continue
		}
		if err := m.startLocked(ctx, key, g); err != nil {
			errs = append(errs, fmt.Errorf("group %s (%s): %w", g.Nickname, key, err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("%d group(s) failed to start: %v", len(errs), errs)
	}
	return nil
}

// opCtx bounds only the synchronous setup calls below (adding the VIP);
// the listener and health-check goroutines started here must outlive
// opCtx (which, called from the /sync HTTP handler, is request-scoped and
// would otherwise cancel — and tear the listener down — the instant that
// HTTP response is written), so they're derived from m.baseCtx instead.
func (m *Manager) startLocked(opCtx context.Context, key string, g Group) error {
	if err := addVIP(opCtx, m.iface, g.VIPAddress); err != nil {
		return fmt.Errorf("add VIP: %w", err)
	}

	listener, err := net.Listen("tcp", g.vipAddr())
	if err != nil {
		delVIP(opCtx, m.iface, g.VIPAddress)
		return fmt.Errorf("listen: %w", err)
	}

	states := make(map[string]*backendState, len(g.Backends))
	for _, b := range g.Backends {
		states[b.addr()] = newBackendState(b)
	}

	poolPtr := &atomicPoolPtr{}
	poolPtr.Store(buildPool(g, states))

	healthCtx, cancel := context.WithCancel(m.baseCtx)
	go runHealthChecks(healthCtx, poolPtr, healthCheckInterval)

	listenerCtx, listenerCancel := context.WithCancel(m.baseCtx)
	go runListener(listenerCtx, listener, g.vipAddr(), g.VIPAddress, poolPtr, m.recordTraffic)
	// One cancel tears down both the health-check loop and the listener;
	// wrap so stopLocked only needs to call it once.
	combinedCancel := func() {
		cancel()
		listenerCancel()
	}

	m.running[key] = &runningGroup{
		group:        g,
		listener:     listener,
		poolPtr:      poolPtr,
		cancelHealth: combinedCancel,
		states:       states,
	}
	slog.Info("lbd: VIP started", "group", g.Nickname, "vip", g.vipAddr(), "backends", len(g.Backends))
	return nil
}

func (m *Manager) updatePoolLocked(rg *runningGroup, g Group) {
	// Reuse backendState for any backend address that's still present, so
	// its health-check history and active-connection count survive a sync
	// that only reorders the list, changes weights, or adds/removes a
	// sibling backend.
	newStates := make(map[string]*backendState, len(g.Backends))
	for _, b := range g.Backends {
		addr := b.addr()
		if existing, ok := rg.states[addr]; ok {
			newStates[addr] = existing
		} else {
			newStates[addr] = newBackendState(b)
		}
	}
	rg.states = newStates
	rg.group = g
	rg.poolPtr.Store(buildPool(g, newStates))
}

func (m *Manager) stopLocked(ctx context.Context, key string, rg *runningGroup) {
	rg.cancelHealth()
	rg.listener.Close()
	if err := delVIP(ctx, m.iface, rg.group.VIPAddress); err != nil {
		slog.Warn("lbd: failed to release VIP on teardown", "vip", rg.group.VIPAddress, "error", err)
	}
	delete(m.running, key)
	slog.Info("lbd: VIP stopped", "group", rg.group.Nickname, "vip", key)
}

func buildPool(g Group, states map[string]*backendState) *backendPool {
	pool := &backendPool{algorithm: g.Algorithm}
	for _, b := range g.Backends {
		pool.backends = append(pool.backends, states[b.addr()])
	}
	return pool
}

// Status reports live health for every running group's backends.
func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()

	status := Status{}
	for _, rg := range m.running {
		gs := GroupStatus{VIPAddress: rg.group.VIPAddress, VIPPort: rg.group.VIPPort}
		for _, b := range rg.group.Backends {
			if state, ok := rg.states[b.addr()]; ok {
				gs.Backends = append(gs.Backends, state.health())
			}
		}
		status.Groups = append(status.Groups, gs)
	}
	return status
}
