package netdisc

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadDNSMasqLeases(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dnsmasq.leases")
	content := "1234567890 aa:bb:cc:dd:ee:01 192.168.1.50 laptop-akobir *\n" +
		"1234567891 aa:bb:cc:dd:ee:02 192.168.1.51 * *\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	leases, err := readDNSMasqLeases(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(leases) != 2 {
		t.Fatalf("got %d leases, want 2", len(leases))
	}
	if leases[0].mac != "aa:bb:cc:dd:ee:01" || leases[0].ip != "192.168.1.50" || leases[0].hostname != "laptop-akobir" {
		t.Errorf("lease[0] = %+v", leases[0])
	}
	if leases[1].hostname != "" {
		t.Errorf("lease[1].hostname = %q, want empty for '*'", leases[1].hostname)
	}
}

func TestReadDNSMasqLeasesMissingFile(t *testing.T) {
	_, err := readDNSMasqLeases("/nonexistent/path/dnsmasq.leases")
	if !os.IsNotExist(err) {
		t.Fatalf("expected os.IsNotExist error, got %v", err)
	}
}
