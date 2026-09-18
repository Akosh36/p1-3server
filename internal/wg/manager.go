package wg

import (
	"fmt"
	"log/slog"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	defaultReachableAfter = 150 * time.Second
	persistentKeepalive   = "25"
	stopGracePeriod       = 5 * time.Second
)

// Manager owns this gateway's single WireGuard interface and every peer
// (remote site) currently configured on it. Like lb.Manager and
// firewall.Manager, it never touches Postgres.
type Manager struct {
	iface          string
	address        string
	listenPort     int
	privateKeyPath string
	reachableAfter time.Duration

	publicKey string

	userspaceCmd  *exec.Cmd
	userspaceDone chan struct{}

	mu sync.Mutex
}

func NewManager(iface, address string, listenPort int, privateKeyPath string, reachableAfter time.Duration) *Manager {
	if reachableAfter <= 0 {
		reachableAfter = defaultReachableAfter
	}
	return &Manager{
		iface:          iface,
		address:        address,
		listenPort:     listenPort,
		privateKeyPath: privateKeyPath,
		reachableAfter: reachableAfter,
	}
}

// Start brings up the local WireGuard interface: generates (or loads) this
// gateway's private key, creates the interface (native kernel, or the
// wireguard-go fallback), and configures its own address/listen port.
// Called once at daemon startup, before ServeControl.
func (m *Manager) Start() error {
	privateKey, err := ensurePrivateKey(m.privateKeyPath)
	if err != nil {
		return fmt.Errorf("private key: %w", err)
	}
	publicKey, err := publicKeyFor(privateKey)
	if err != nil {
		return fmt.Errorf("derive public key: %w", err)
	}
	m.publicKey = publicKey

	cmd, done, err := bringUpInterface(m.iface)
	if err != nil {
		return fmt.Errorf("bring up interface: %w", err)
	}
	m.userspaceCmd = cmd
	m.userspaceDone = done

	if err := runWG("set", m.iface, "private-key", m.privateKeyPath, "listen-port", strconv.Itoa(m.listenPort)); err != nil {
		return fmt.Errorf("configure interface: %w", err)
	}
	if err := runIP("address", "add", m.address, "dev", m.iface); err != nil {
		return fmt.Errorf("assign address: %w", err)
	}
	if err := runIP("link", "set", m.iface, "up"); err != nil {
		return fmt.Errorf("bring interface up: %w", err)
	}

	usingFallback := cmd != nil
	slog.Info("wgd: interface ready", "interface", m.iface, "address", m.address,
		"listen_port", m.listenPort, "public_key", m.publicKey, "userspace_fallback", usingFallback)
	return nil
}

// Stop tears down the interface. For the userspace fallback this means
// signaling wireguard-go and waiting for it to exit (mirroring capd's
// Stop: one goroutine — the one started in bringUpInterface — owns
// cmd.Wait(), this just waits on the channel it closes); for a native
// kernel interface, `ip link del` alone is the entire teardown.
func (m *Manager) Stop() {
	if m.userspaceCmd == nil {
		_ = runIP("link", "del", m.iface)
		return
	}

	if err := m.userspaceCmd.Process.Signal(syscall.SIGTERM); err != nil {
		_ = m.userspaceCmd.Process.Kill()
	}
	select {
	case <-m.userspaceDone:
	case <-time.After(stopGracePeriod):
		_ = m.userspaceCmd.Process.Kill()
		<-m.userspaceDone
	}
}

// Sync reconciles the interface's configured peers against the desired
// list: peers no longer desired are removed, desired peers are
// added/updated (idempotent — re-applying the same config is harmless).
// PersistentKeepalive is always set so a peer behind NAT still re-handshakes
// regularly, which is what makes the reachability check in Status() close
// to real-time rather than only updating when user traffic happens to flow.
//
// `wg set` only configures the peer's crypto/networking parameters — unlike
// wg-quick, it does NOT touch the routing table, so a peer's AllowedIPs
// alone would never actually route traffic anywhere. Sync explicitly adds
// a route for each peer's remote subnet via this interface (and removes it
// when the peer is removed), which is the piece wg-quick normally does for
// you and easy to miss when driving `wg` directly.
func (m *Manager) Sync(peers []Peer) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	current, err := m.currentPeers()
	if err != nil {
		return fmt.Errorf("list current peers: %w", err)
	}

	desired := make(map[string]Peer, len(peers))
	for _, p := range peers {
		desired[p.PublicKey] = p
	}

	var errs []error
	for pubkey, allowedIPs := range current {
		if _, keep := desired[pubkey]; keep {
			continue
		}
		if err := runWG("set", m.iface, "peer", pubkey, "remove"); err != nil {
			errs = append(errs, fmt.Errorf("remove peer %s: %w", pubkey, err))
		}
		for _, cidr := range strings.Split(allowedIPs, ",") {
			cidr = strings.TrimSpace(cidr)
			if cidr != "" && cidr != "(none)" {
				_ = runIP("route", "del", cidr, "dev", m.iface) // best-effort: fine if already gone
			}
		}
	}

	for pubkey, p := range desired {
		args := []string{"set", m.iface, "peer", pubkey, "allowed-ips", p.AllowedSubnet, "persistent-keepalive", persistentKeepalive}
		if p.Endpoint != "" {
			args = append(args, "endpoint", p.Endpoint)
		}
		if err := runWG(args...); err != nil {
			errs = append(errs, fmt.Errorf("configure peer %s: %w", pubkey, err))
			continue
		}
		if err := runIP("route", "replace", p.AllowedSubnet, "dev", m.iface); err != nil {
			errs = append(errs, fmt.Errorf("add route for peer %s: %w", pubkey, err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("%d peer(s) failed to sync: %v", len(errs), errs)
	}
	return nil
}

// currentPeers returns every peer currently configured on the interface,
// keyed by public key, with its AllowedIPs (comma-separated CIDRs) so a
// removal can also clean up the matching route.
func (m *Manager) currentPeers() (map[string]string, error) {
	out, err := runWGCapture("show", m.iface, "allowed-ips")
	if err != nil {
		return nil, err
	}
	current := make(map[string]string)
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.SplitN(line, "\t", 2)
		if len(fields) == 2 {
			current[fields[0]] = fields[1]
		}
	}
	return current, nil
}

// Status reports this gateway's tunnel identity and every configured
// peer's live handshake state.
func (m *Manager) Status() (Status, error) {
	dump, err := runWGCapture("show", m.iface, "dump")
	if err != nil {
		return Status{}, err
	}
	return Status{
		LocalPublicKey: m.publicKey,
		ListenPort:     m.listenPort,
		Peers:          parseDump(dump, m.reachableAfter, time.Now()),
	}, nil
}

// parseDump turns `wg show <iface> dump`'s output into peer statuses. Pure
// function of its inputs (no shelling out), so it's covered by
// manager_test.go against real captured output shapes rather than a live
// interface.
//
// Format: the first line is the interface's own
// private-key/public-key/listen-port/fwmark — not a peer, always skipped.
// Each following line is one peer: public-key, preshared-key, endpoint,
// allowed-ips, latest-handshake (unix seconds, 0 = never), rx, tx,
// persistent-keepalive, tab-separated.
func parseDump(dump string, reachableAfter time.Duration, now time.Time) []PeerStatus {
	lines := strings.Split(strings.TrimSpace(dump), "\n")
	var peers []PeerStatus
	for i, line := range lines {
		if i == 0 || line == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 5 {
			continue
		}

		ps := PeerStatus{PublicKey: fields[0]}
		if sec, err := strconv.ParseInt(fields[4], 10, 64); err == nil && sec > 0 {
			handshakeAt := time.Unix(sec, 0).UTC()
			ps.LastHandshakeAt = handshakeAt.Format(time.RFC3339)
			ps.IsReachable = now.Sub(handshakeAt) < reachableAfter
		}
		peers = append(peers, ps)
	}
	return peers
}
