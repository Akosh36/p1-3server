package capd

import (
	"bytes"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

const stopGracePeriod = 5 * time.Second

type runningCapture struct {
	captureID int64
	mac       string
	filePath  string
	startedAt time.Time
	cmd       *exec.Cmd
	stderr    *bytes.Buffer
	// done is closed exactly once, by watch()'s single call to cmd.Wait().
	// Stop() must never call cmd.Wait() itself — os/exec explicitly forbids
	// calling Wait concurrently from two goroutines, and doing so anyway
	// was a real bug found here: under light traffic the racing watch()
	// goroutine usually reaped the process first and Stop()'s own Wait()
	// call harmlessly lost the race, but under heavy traffic (tcpdump
	// taking longer to flush and exit) the two calls could each end up
	// blocked waiting on the other's result, hanging /captures/{id}/stop
	// forever. Waiting on this channel instead of calling Wait again fixes
	// it: there is only ever one Wait() call, in one place.
	done chan struct{}
}

// Manager owns every tcpdump process capd currently has running, plus disk
// quota enforcement across captureDir. Like lb.Manager and
// firewall.Manager, it never touches Postgres.
type Manager struct {
	iface         string
	captureDir    string
	rotateBytes   int64
	rotateAfter   time.Duration
	maxTotalBytes int64

	mu      sync.Mutex
	running map[int64]*runningCapture // key: CaptureID
	evicted []string                  // file paths deleted for quota since the last Status() drain
}

func NewManager(iface, captureDir string, rotateBytes int64, rotateAfter time.Duration, maxTotalBytes int64) *Manager {
	return &Manager{
		iface:         iface,
		captureDir:    captureDir,
		rotateBytes:   rotateBytes,
		rotateAfter:   rotateAfter,
		maxTotalBytes: maxTotalBytes,
		running:       make(map[int64]*runningCapture),
	}
}

// Start begins capturing req.MAC's traffic on this daemon's configured
// interface into req.FilePath. Filtering by MAC (not IP) survives a DHCP
// lease renewal, and capturing on the LAN-facing interface sees both
// directions of the device's routed traffic — every packet either arrives
// from the device addressed to this gateway's own MAC, or leaves this
// gateway addressed to the device's MAC, since this platform is the
// device's default gateway (architecture decision #14).
func (m *Manager) Start(req StartRequest) error {
	if !validMAC(req.MAC) {
		return fmt.Errorf("invalid mac address: %q", req.MAC)
	}
	cleanPath, err := validatePath(m.captureDir, req.FilePath)
	if err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.running[req.CaptureID]; exists {
		return fmt.Errorf("capture %d already running", req.CaptureID)
	}

	if err := os.MkdirAll(filepath.Dir(cleanPath), 0o750); err != nil {
		return fmt.Errorf("create capture dir: %w", err)
	}

	var stderr bytes.Buffer
	// -U flushes each packet to disk immediately, so SIGTERM always leaves a
	// valid, fully-flushed pcap file behind even for a low-traffic device —
	// without it a rotation/download could read back a truncated tail.
	cmd := exec.Command("tcpdump", "-i", m.iface, "-U", "-w", cleanPath, "ether", "host", req.MAC)
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start tcpdump: %w: %s", err, stderr.String())
	}

	rc := &runningCapture{
		captureID: req.CaptureID,
		mac:       req.MAC,
		filePath:  cleanPath,
		startedAt: time.Now(),
		cmd:       cmd,
		stderr:    &stderr,
		done:      make(chan struct{}),
	}
	m.running[req.CaptureID] = rc

	go m.watch(rc)

	slog.Info("capd: capture started", "capture_id", req.CaptureID, "mac", req.MAC, "file", cleanPath)
	return nil
}

// watch reaps tcpdump's exit. A clean Stop() removes the entry from
// m.running before signaling the process, so by the time Wait returns here
// the map either no longer has this entry (expected stop) or still points
// at this exact *runningCapture (unexpected exit — permission dropped,
// interface disappeared, disk full, etc. — which is worth logging loudly
// since nothing else will notice until the next status poll shows the
// capture simply isn't running anymore).
func (m *Manager) watch(rc *runningCapture) {
	waitErr := rc.cmd.Wait()
	close(rc.done)

	m.mu.Lock()
	defer m.mu.Unlock()
	if current, ok := m.running[rc.captureID]; ok && current == rc {
		if waitErr != nil {
			slog.Error("capd: tcpdump exited unexpectedly", "capture_id", rc.captureID, "error", waitErr, "stderr", rc.stderr.String())
		}
		delete(m.running, rc.captureID)
	}
}

// Stop signals a running capture's tcpdump process to end (SIGTERM, giving
// it stopGracePeriod to flush and exit cleanly before SIGKILL) and returns
// its final file size.
func (m *Manager) Stop(captureID int64) (StopResult, error) {
	m.mu.Lock()
	rc, exists := m.running[captureID]
	if exists {
		delete(m.running, captureID) // remove first: watch()'s "unexpected exit" branch is only for exits we didn't ask for
	}
	m.mu.Unlock()

	if !exists {
		return StopResult{}, fmt.Errorf("capture %d not running", captureID)
	}

	if err := rc.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		slog.Warn("capd: SIGTERM failed, killing", "capture_id", captureID, "error", err)
		_ = rc.cmd.Process.Kill()
	}

	select {
	case <-rc.done:
	case <-time.After(stopGracePeriod):
		slog.Warn("capd: tcpdump didn't exit after SIGTERM, killing", "capture_id", captureID)
		_ = rc.cmd.Process.Kill()
		<-rc.done
	}

	size := fileSize(rc.filePath)
	slog.Info("capd: capture stopped", "capture_id", captureID, "file", rc.filePath, "size_bytes", size)
	return StopResult{CaptureID: captureID, FilePath: rc.filePath, SizeBytes: size}, nil
}

// StopAll ends every currently-running capture. Called on daemon shutdown
// so a manual stop/restart (outside systemd's cgroup-wide SIGTERM, which
// would reach tcpdump directly anyway) never leaves an orphaned tcpdump
// process still writing to a file nothing is tracking anymore.
func (m *Manager) StopAll() {
	m.mu.Lock()
	ids := make([]int64, 0, len(m.running))
	for id := range m.running {
		ids = append(ids, id)
	}
	m.mu.Unlock()

	for _, id := range ids {
		if _, err := m.Stop(id); err != nil {
			slog.Warn("capd: failed to stop capture during shutdown", "capture_id", id, "error", err)
		}
	}
}

func fileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}

// Status reports every currently-running capture's live size/elapsed time
// and whether it has crossed this daemon's configured rotation threshold,
// plus any files evicted by quota enforcement since the last call.
func (m *Manager) Status() StatusResponse {
	m.mu.Lock()
	defer m.mu.Unlock()

	resp := StatusResponse{}
	now := time.Now()
	for _, rc := range m.running {
		size := fileSize(rc.filePath)
		elapsed := now.Sub(rc.startedAt)
		needsRotation := false
		reason := ""
		switch {
		case m.rotateBytes > 0 && size >= m.rotateBytes:
			needsRotation, reason = true, "size_limit"
		case m.rotateAfter > 0 && elapsed >= m.rotateAfter:
			needsRotation, reason = true, "time_limit"
		}
		resp.Captures = append(resp.Captures, CaptureStatus{
			CaptureID:      rc.captureID,
			MAC:            rc.mac,
			FilePath:       rc.filePath,
			SizeBytes:      size,
			StartedAt:      rc.startedAt.UTC().Format(time.RFC3339),
			ElapsedSeconds: elapsed.Seconds(),
			NeedsRotation:  needsRotation,
			RotationReason: reason,
		})
	}

	resp.Evicted = m.evicted
	m.evicted = nil // drained: the caller is expected to act on this exactly once
	return resp
}

// ReadFile returns a completed capture's bytes for the control-plane to
// proxy as a download. Refuses to read a file currently open for writing —
// tcpdump's own file isn't guaranteed consistent mid-capture, and the
// control-plane is expected to Stop() an active capture before offering it
// for download (see docs/deploy.md's Phase 6 section).
func (m *Manager) ReadFile(path string) ([]byte, error) {
	clean, err := validatePath(m.captureDir, path)
	if err != nil {
		return nil, err
	}

	m.mu.Lock()
	for _, rc := range m.running {
		if rc.filePath == clean {
			m.mu.Unlock()
			return nil, fmt.Errorf("capture is still recording, stop it first")
		}
	}
	m.mu.Unlock()

	return os.ReadFile(clean)
}
