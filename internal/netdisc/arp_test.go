package netdisc

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadARPTable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "arp")
	content := "IP address       HW type     Flags       HW address            Mask     Device\n" +
		"192.168.1.50     0x1         0x2         aa:bb:cc:dd:ee:01     *        eth0\n" +
		"192.168.1.51     0x1         0x0         00:00:00:00:00:00     *        eth0\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	entries, err := readARPTable(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1 (incomplete entry must be filtered out): %+v", len(entries), entries)
	}
	if entries[0].ip != "192.168.1.50" || entries[0].mac != "aa:bb:cc:dd:ee:01" {
		t.Errorf("entries[0] = %+v", entries[0])
	}
}

// TestReadARPTableReal reads the real /proc/net/arp on this Linux host —
// this only checks the parser doesn't choke on real kernel output, not any
// specific content.
func TestReadARPTableReal(t *testing.T) {
	if _, err := os.Stat(arpTablePath); err != nil {
		t.Skipf("no %s on this system: %v", arpTablePath, err)
	}
	entries, err := readARPTable(arpTablePath)
	if err != nil {
		t.Fatalf("unexpected error reading real ARP table: %v", err)
	}
	t.Logf("real ARP table has %d resolved entries", len(entries))
}
