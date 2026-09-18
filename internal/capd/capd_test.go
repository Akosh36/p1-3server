package capd

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestValidMAC(t *testing.T) {
	valid := []string{"aa:bb:cc:dd:ee:ff", "00:11:22:33:44:55", "AA:BB:CC:DD:EE:FF"}
	for _, mac := range valid {
		if !validMAC(mac) {
			t.Errorf("expected %q to be a valid MAC", mac)
		}
	}

	invalid := []string{"", "aa:bb:cc:dd:ee", "aa-bb-cc-dd-ee-ff", "not a mac", "aa:bb:cc:dd:ee:ff:gg", "gg:bb:cc:dd:ee:ff"}
	for _, mac := range invalid {
		if validMAC(mac) {
			t.Errorf("expected %q to be an invalid MAC", mac)
		}
	}
}

func TestValidatePath(t *testing.T) {
	captureDir := "/var/lib/p13server/captures"

	inside := []string{
		"/var/lib/p13server/captures/device_1/5.pcap",
		"/var/lib/p13server/captures/5.pcap",
	}
	for _, p := range inside {
		if _, err := validatePath(captureDir, p); err != nil {
			t.Errorf("expected %q to be accepted, got error: %v", p, err)
		}
	}

	outside := []string{
		"/var/lib/p13server/captures/../../etc/passwd",
		"/etc/passwd",
		"/var/lib/p13server/captures-other/5.pcap",
	}
	for _, p := range outside {
		if _, err := validatePath(captureDir, p); err == nil {
			t.Errorf("expected %q to be rejected as outside %q", p, captureDir)
		}
	}
}

func TestManager_StartRejectsInvalidMAC(t *testing.T) {
	m := NewManager("lo", t.TempDir(), 0, 0, 0)
	err := m.Start(StartRequest{CaptureID: 1, MAC: "not-a-mac", FilePath: filepath.Join(m.captureDir, "1.pcap")})
	if err == nil {
		t.Fatal("expected an error for an invalid MAC, got nil")
	}
}

func TestManager_StartRejectsPathOutsideCaptureDir(t *testing.T) {
	m := NewManager("lo", t.TempDir(), 0, 0, 0)
	err := m.Start(StartRequest{CaptureID: 1, MAC: "aa:bb:cc:dd:ee:ff", FilePath: "/etc/passwd"})
	if err == nil {
		t.Fatal("expected an error for a path outside the capture directory, got nil")
	}
}

func writeDummyFile(t *testing.T, path string, size int, modTime time.Time) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, make([]byte, size), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, modTime, modTime); err != nil {
		t.Fatal(err)
	}
}

func TestEnforceQuota_DeletesOldestFilesUntilUnderLimit(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	oldest := filepath.Join(dir, "oldest.pcap")
	middle := filepath.Join(dir, "middle.pcap")
	newest := filepath.Join(dir, "newest.pcap")
	writeDummyFile(t, oldest, 100, now.Add(-3*time.Hour))
	writeDummyFile(t, middle, 100, now.Add(-2*time.Hour))
	writeDummyFile(t, newest, 100, now.Add(-1*time.Hour))

	// Total is 300 bytes; cap at 150 should evict the single oldest file
	// (down to 200) and then the next-oldest too (down to 100), leaving
	// only the newest.
	m := NewManager("lo", dir, 0, 0, 150)
	m.enforceQuota()

	if _, err := os.Stat(oldest); !os.IsNotExist(err) {
		t.Errorf("expected oldest.pcap to be evicted, stat error: %v", err)
	}
	if _, err := os.Stat(middle); !os.IsNotExist(err) {
		t.Errorf("expected middle.pcap to be evicted, stat error: %v", err)
	}
	if _, err := os.Stat(newest); err != nil {
		t.Errorf("expected newest.pcap to survive, stat error: %v", err)
	}

	status := m.Status()
	if len(status.Evicted) != 2 {
		t.Fatalf("expected 2 evicted paths reported, got %d: %v", len(status.Evicted), status.Evicted)
	}
}

func TestEnforceQuota_NeverDeletesActiveFile(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	active := filepath.Join(dir, "active.pcap")
	writeDummyFile(t, active, 100, now.Add(-5*time.Hour)) // oldest, but currently "recording"

	m := NewManager("lo", dir, 0, 0, 10) // quota far smaller than the file itself
	m.mu.Lock()
	m.running[42] = &runningCapture{captureID: 42, filePath: active, startedAt: now}
	m.mu.Unlock()

	m.enforceQuota()

	if _, err := os.Stat(active); err != nil {
		t.Errorf("expected the actively-recording file to survive quota enforcement, stat error: %v", err)
	}
}
