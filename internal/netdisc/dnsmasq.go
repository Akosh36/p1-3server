package netdisc

import (
	"bufio"
	"context"
	"log/slog"
	"os"
	"strings"
	"time"
)

// RunDNSMasqCollector re-reads dnsmasq's lease file on every interval and
// feeds each lease's MAC/IP/hostname into store. leasePath is normally
// /var/lib/misc/dnsmasq.leases (dnsmasq's compiled-in default) — see
// deploy/dnsmasq once Phase 3 wires up the actual DHCP config. The whole
// file is re-parsed each tick rather than tailed: lease files stay small
// (one line per currently-leased device) even at the "50-300 devices"
// scale this platform targets, so re-parsing is simpler and can't drift out
// of sync with lease expiry/release rewrites.
func RunDNSMasqCollector(ctx context.Context, store *Store, leasePath string, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		if leases, err := readDNSMasqLeases(leasePath); err != nil {
			if !os.IsNotExist(err) {
				slog.Warn("netdiscd: failed to read dnsmasq leases", "path", leasePath, "error", err)
			}
		} else {
			for _, l := range leases {
				store.ObserveDHCPLease(l.mac, l.ip, l.hostname)
			}
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

type dnsmasqLease struct {
	mac, ip, hostname string
}

// readDNSMasqLeases parses dnsmasq's lease file format:
//
//	<expiry-epoch> <mac-address> <ip-address> <hostname-or-*> <client-id-or-*>
func readDNSMasqLeases(path string) ([]dnsmasqLease, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var leases []dnsmasqLease
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 4 {
			continue
		}
		mac, ip, hostname := strings.ToLower(fields[1]), fields[2], fields[3]
		if hostname == "*" {
			hostname = ""
		}
		leases = append(leases, dnsmasqLease{mac: mac, ip: ip, hostname: hostname})
	}
	return leases, scanner.Err()
}
