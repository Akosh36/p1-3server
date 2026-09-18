// Package capdsync is the control-plane half of Phase 6: it owns every
// decision about traffic_captures (when a capture starts, when it rotates
// into a new file, when it's ended) and drives capd purely by telling it
// "start this" / "stop that" over capd's local Unix socket. capd itself
// never touches Postgres — see internal/capd.
package capdsync

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Akosh36/p1-3server/internal/models"
)

type startRequest struct {
	CaptureID int64  `json:"capture_id"`
	MAC       string `json:"mac"`
	FilePath  string `json:"file_path"`
}

type stopResult struct {
	CaptureID int64  `json:"capture_id"`
	FilePath  string `json:"file_path"`
	SizeBytes int64  `json:"size_bytes"`
}

type captureStatus struct {
	CaptureID      int64   `json:"capture_id"`
	MAC            string  `json:"mac"`
	FilePath       string  `json:"file_path"`
	SizeBytes      int64   `json:"size_bytes"`
	StartedAt      string  `json:"started_at"`
	ElapsedSeconds float64 `json:"elapsed_seconds"`
	NeedsRotation  bool    `json:"needs_rotation"`
	RotationReason string  `json:"rotation_reason,omitempty"`
}

type statusResponse struct {
	Captures []captureStatus `json:"captures"`
	Evicted  []string        `json:"evicted,omitempty"`
}

// NewFilePath picks where a capture segment's pcap file lives under
// captureDir, one directory per device. Used both for a brand-new capture
// (internal/httpapi's start handler) and for a rotation's continuation
// segment (RotateCapture below), so the naming scheme exists in exactly
// one place.
func NewFilePath(captureDir string, deviceID int64) string {
	return filepath.Join(captureDir, fmt.Sprintf("device_%d", deviceID), fmt.Sprintf("%d.pcap", time.Now().UnixNano()))
}

// Run reconciles traffic_captures against capd's live state every interval,
// until ctx is cancelled. Same resilience pattern as aclsync/discovery/
// lbsync: if capd isn't running, this logs a warning once and keeps
// retrying rather than crashing the API.
func Run(ctx context.Context, pool *pgxpool.Pool, socketPath, captureDir string, interval time.Duration) {
	client := newUnixSocketClient(socketPath)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	warnedUnreachable := false
	for {
		if err := tick(ctx, pool, client, socketPath, captureDir); err != nil {
			if !warnedUnreachable {
				slog.Warn("capdsync: capd unreachable, capture state will not update until it is running", "socket", socketPath, "error", err)
				warnedUnreachable = true
			}
		} else {
			warnedUnreachable = false
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

type recordingRow struct {
	id               int64
	deviceID         int64
	mac              string
	filePath         string
	startedByAdminID *int64
}

func tick(ctx context.Context, pool *pgxpool.Pool, client *http.Client, socketPath, captureDir string) error {
	rows, err := loadRecordingRows(ctx, pool)
	if err != nil {
		return fmt.Errorf("load recording rows: %w", err)
	}

	status, err := fetchStatus(ctx, client)
	if err != nil {
		return fmt.Errorf("fetch capd status: %w", err)
	}

	if err := handleEvictions(ctx, pool, status.Evicted); err != nil {
		slog.Error("capdsync: failed to record quota eviction", "error", err)
	}

	active := make(map[int64]captureStatus, len(status.Captures))
	for _, c := range status.Captures {
		active[c.CaptureID] = c
	}

	for _, row := range rows {
		live, running := active[row.id]
		switch {
		case !running:
			// Not actually running under capd yet — either the API just
			// inserted this row and hasn't reached capd yet, or a previous
			// start attempt failed while capd was briefly unreachable.
			// Retrying every tick until it succeeds is deliberate and safe:
			// capd's Start rejects a capture_id it already has running, so
			// this never double-starts once it catches on.
			if err := startCaptureWith(ctx, client, row.id, row.mac, row.filePath); err != nil {
				slog.Warn("capdsync: failed to (re)start capture", "capture_id", row.id, "error", err)
			}
		case live.NeedsRotation:
			reason := live.RotationReason
			if err := RotateCapture(ctx, pool, socketPath, captureDir, row.id, row.deviceID, row.mac, row.startedByAdminID, models.CaptureRotated, reason); err != nil {
				slog.Error("capdsync: failed to rotate capture", "capture_id", row.id, "reason", reason, "error", err)
			}
		default:
			if _, err := pool.Exec(ctx, `UPDATE traffic_captures SET size_bytes = $1 WHERE id = $2`, live.SizeBytes, row.id); err != nil {
				slog.Error("capdsync: failed to update live capture size", "capture_id", row.id, "error", err)
			}
		}
	}

	return nil
}

func loadRecordingRows(ctx context.Context, pool *pgxpool.Pool) ([]recordingRow, error) {
	rows, err := pool.Query(ctx, `
		SELECT tc.id, tc.device_id, d.mac_address::text, tc.file_path, tc.started_by_admin_id
		FROM traffic_captures tc
		JOIN devices d ON d.id = tc.device_id
		WHERE tc.status = 'recording'
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []recordingRow
	for rows.Next() {
		var r recordingRow
		if err := rows.Scan(&r.id, &r.deviceID, &r.mac, &r.filePath, &r.startedByAdminID); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// RotateCapture ends captureID's currently-recording segment — marking it
// newStatus ('rotated' for an automatic size/time limit, 'downloaded' for
// an admin's explicit Download click) — and immediately starts a fresh
// continuation segment for the same device, so recording never visibly
// stops except via an explicit Stop. Shared by tick's automatic rotation
// and internal/httpapi's download handler so both paths insert/start a
// continuation row identically.
func RotateCapture(ctx context.Context, pool *pgxpool.Pool, socketPath, captureDir string, captureID, deviceID int64, mac string, startedByAdminID *int64, newStatus models.CaptureStatus, reason string) error {
	client := newUnixSocketClient(socketPath)
	result, err := stopCaptureWith(ctx, client, captureID)
	if err != nil {
		return fmt.Errorf("stop: %w", err)
	}

	if _, err := pool.Exec(ctx, `
		UPDATE traffic_captures SET status = $1, stopped_at = now(), size_bytes = $2, rotation_reason = $3
		WHERE id = $4
	`, newStatus, result.SizeBytes, reason, captureID); err != nil {
		return fmt.Errorf("update row: %w", err)
	}

	newPath := NewFilePath(captureDir, deviceID)
	var newID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO traffic_captures (device_id, started_by_admin_id, file_path, status)
		VALUES ($1, $2, $3, 'recording') RETURNING id
	`, deviceID, startedByAdminID, newPath).Scan(&newID); err != nil {
		return fmt.Errorf("insert continuation row: %w", err)
	}

	if err := startCaptureWith(ctx, client, newID, mac, newPath); err != nil {
		return fmt.Errorf("start continuation: %w", err)
	}
	return nil
}

func handleEvictions(ctx context.Context, pool *pgxpool.Pool, evicted []string) error {
	for _, path := range evicted {
		if _, err := pool.Exec(ctx, `
			UPDATE traffic_captures
			SET status = 'error', rotation_reason = COALESCE(rotation_reason || '; ', '') || 'quota_evicted'
			WHERE file_path = $1 AND status != 'error'
		`, path); err != nil {
			return err
		}
	}
	return nil
}

func fetchStatus(ctx context.Context, client *http.Client) (statusResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://capd/captures/status", nil)
	if err != nil {
		return statusResponse{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return statusResponse{}, err
	}
	defer resp.Body.Close()

	var status statusResponse
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return statusResponse{}, err
	}
	return status, nil
}

func startCaptureWith(ctx context.Context, client *http.Client, captureID int64, mac, filePath string) error {
	body, err := json.Marshal(startRequest{CaptureID: captureID, MAC: mac, FilePath: filePath})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://capd/captures/start", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("capd returned status %d: %s", resp.StatusCode, msg)
	}
	return nil
}

func stopCaptureWith(ctx context.Context, client *http.Client, captureID int64) (stopResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("http://capd/captures/%d/stop", captureID), nil)
	if err != nil {
		return stopResult{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return stopResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(resp.Body)
		return stopResult{}, fmt.Errorf("capd returned status %d: %s", resp.StatusCode, msg)
	}

	var result stopResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return stopResult{}, err
	}
	return result, nil
}

// StartCapture and StopCapture are the ad-hoc counterparts to the
// reconciliation loop above, called directly by internal/httpapi for an
// admin's immediate "Start" / "Stop" click rather than waiting for the
// next tick.

func StartCapture(ctx context.Context, socketPath string, captureID int64, mac, filePath string) error {
	return startCaptureWith(ctx, newUnixSocketClient(socketPath), captureID, mac, filePath)
}

func StopCapture(ctx context.Context, socketPath string, captureID int64) (sizeBytes int64, err error) {
	result, err := stopCaptureWith(ctx, newUnixSocketClient(socketPath), captureID)
	if err != nil {
		return 0, err
	}
	return result.SizeBytes, nil
}

// ReadCaptureFile fetches a completed capture's raw bytes from capd, for
// internal/httpapi to stream back as a download without needing its own
// filesystem access to CAPTURE_DIR (the API and capd may not even share a
// filesystem — see docs/deploy.md).
func ReadCaptureFile(ctx context.Context, socketPath, path string) ([]byte, error) {
	client := newUnixSocketClient(socketPath)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://capd/captures/file?path="+url.QueryEscape(path), nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("capd returned status %d: %s", resp.StatusCode, msg)
	}
	return io.ReadAll(resp.Body)
}

func newUnixSocketClient(socketPath string) *http.Client {
	return &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, "unix", socketPath)
			},
		},
	}
}
