package netdisc

import (
	"bufio"
	"context"
	"log/slog"
	"os"
	"strings"
	"time"
)

const arpTablePath = "/proc/net/arp"

// invalidMAC is what the kernel prints for an incomplete ARP entry (a
// request was sent but never answered) — not a real device, so it must not
// show up as a discovered host.
const invalidMAC = "00:00:00:00:00:00"

// RunARPCollector polls the kernel's ARP table (/proc/net/arp) on Linux
// every interval and feeds resolved entries into store. This is purely
// passive — it reports whatever the kernel has already resolved via normal
// traffic, so a device that has been silent since boot won't appear until
// something (a lease renewal, a ping) makes the kernel ARP for it. Active
// probing (arping a subnet) needs CAP_NET_RAW and is intentionally left out
// of Phase 1 — see CLAUDE.md's Phase 1 scope note.
func RunARPCollector(ctx context.Context, store *Store, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		if entries, err := readARPTable(arpTablePath); err != nil {
			slog.Warn("netdiscd: failed to read ARP table", "path", arpTablePath, "error", err)
		} else {
			now := time.Now()
			for _, e := range entries {
				store.ObserveARP(e.mac, e.ip, now)
			}
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

type arpEntry struct {
	ip, mac string
}

// readARPTable parses /proc/net/arp's fixed-width column format:
//
//	IP address       HW type     Flags       HW address            Mask     Device
//	192.168.1.50     0x1         0x2         aa:bb:cc:dd:ee:01     *        eth0
func readARPTable(path string) ([]arpEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var entries []arpEntry
	scanner := bufio.NewScanner(f)
	scanner.Scan() // header line
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 4 {
			continue
		}
		ip, mac := fields[0], strings.ToLower(fields[3])
		if mac == "" || mac == invalidMAC {
			continue
		}
		entries = append(entries, arpEntry{ip: ip, mac: mac})
	}
	return entries, scanner.Err()
}
