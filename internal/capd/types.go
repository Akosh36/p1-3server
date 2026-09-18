// Package capd is the data-plane half of Phase 6: on-demand per-device
// packet capture. Like netdiscd/fwctl/lbd it never touches Postgres — the
// control-plane's internal/capdsync tells it what to record over a local
// Unix socket, and reads live status/quota-eviction results back the same
// way.
//
// capd is deliberately "dumb": it doesn't decide capture IDs, file names,
// or what happens after a rotation — it just runs (or stops) a tcpdump
// process for a given MAC into a given path, and reports when a running
// capture has crossed its rotation threshold so the control-plane (which
// owns all of that state in traffic_captures) can act on it. This mirrors
// lbd's Manager/Sync split: capd executes, Postgres decides.
package capd

// StartRequest asks capd to begin capturing one device's traffic. CaptureID
// and FilePath are chosen by the control-plane (the traffic_captures row
// already exists with this path before capd is asked to start) — capd only
// validates that FilePath resolves inside its configured capture directory.
type StartRequest struct {
	CaptureID int64  `json:"capture_id"`
	MAC       string `json:"mac"`
	FilePath  string `json:"file_path"`
}

// StopResult is what capd reports after cleanly ending a capture.
type StopResult struct {
	CaptureID int64  `json:"capture_id"`
	FilePath  string `json:"file_path"`
	SizeBytes int64  `json:"size_bytes"`
}

// CaptureStatus is one currently-running capture's live state. NeedsRotation
// is capd's read of its own configured size/time thresholds — it does not
// act on it itself; internal/capdsync polls this and performs the actual
// stop-and-restart-a-new-segment, so the decision (and the resulting new
// traffic_captures row) is made in exactly one place.
type CaptureStatus struct {
	CaptureID      int64   `json:"capture_id"`
	MAC            string  `json:"mac"`
	FilePath       string  `json:"file_path"`
	SizeBytes      int64   `json:"size_bytes"`
	StartedAt      string  `json:"started_at"`
	ElapsedSeconds float64 `json:"elapsed_seconds"`
	NeedsRotation  bool    `json:"needs_rotation"`
	RotationReason string  `json:"rotation_reason,omitempty"`
}

// StatusResponse is GET /captures/status's body. Evicted lists capture
// files capd deleted since the last poll to stay under its disk quota —
// draining this list is how internal/capdsync learns which
// traffic_captures rows need to be marked status='error' even though
// nothing asked capd to stop them.
type StatusResponse struct {
	Captures []CaptureStatus `json:"captures"`
	Evicted  []string        `json:"evicted,omitempty"`
}
