package wg

import (
	"strconv"
	"testing"
	"time"
)

func TestParseDump(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	reachableAfter := 150 * time.Second

	// Real `wg show <iface> dump` shape: first line is the interface's own
	// private-key/public-key/listen-port/fwmark, then one tab-separated
	// line per peer (public-key, preshared-key, endpoint, allowed-ips,
	// latest-handshake unix seconds, rx, tx, persistent-keepalive).
	recentHandshake := now.Add(-30 * time.Second).Unix()
	staleHandshake := now.Add(-10 * time.Minute).Unix()
	dump := "cHJpdmF0ZWtleQ==\tcHVibGlja2V5\t51820\toff\n" +
		"peerA\t(none)\t1.2.3.4:51820\t192.168.50.0/24\t" + strconv.FormatInt(recentHandshake, 10) + "\t100\t200\t25\n" +
		"peerB\t(none)\t5.6.7.8:51820\t192.168.60.0/24\t" + strconv.FormatInt(staleHandshake, 10) + "\t50\t60\t25\n" +
		"peerC\t(none)\t(none)\t192.168.70.0/24\t0\t0\t0\t25\n"

	peers := parseDump(dump, reachableAfter, now)
	if len(peers) != 3 {
		t.Fatalf("expected 3 peers, got %d: %+v", len(peers), peers)
	}

	byKey := map[string]PeerStatus{}
	for _, p := range peers {
		byKey[p.PublicKey] = p
	}

	if !byKey["peerA"].IsReachable {
		t.Errorf("peerA (30s ago) should be reachable")
	}
	if byKey["peerA"].LastHandshakeAt == "" {
		t.Errorf("peerA should have a non-empty LastHandshakeAt")
	}

	if byKey["peerB"].IsReachable {
		t.Errorf("peerB (10m ago) should NOT be reachable")
	}

	if byKey["peerC"].IsReachable {
		t.Errorf("peerC (never handshaked, timestamp 0) should NOT be reachable")
	}
	if byKey["peerC"].LastHandshakeAt != "" {
		t.Errorf("peerC should have an empty LastHandshakeAt, got %q", byKey["peerC"].LastHandshakeAt)
	}
}

func TestParseDump_NoPeers(t *testing.T) {
	dump := "cHJpdmF0ZWtleQ==\tcHVibGlja2V5\t51820\toff\n"
	peers := parseDump(dump, 150*time.Second, time.Now())
	if len(peers) != 0 {
		t.Fatalf("expected 0 peers, got %d", len(peers))
	}
}
