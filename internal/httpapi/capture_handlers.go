package httpapi

import (
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/Akosh36/p1-3server/internal/capdsync"
	"github.com/Akosh36/p1-3server/internal/models"
)

// handleListCaptures feeds the Logs page's pcap table: every capture
// segment ever recorded, newest first, with enough device context
// (nickname/MAC) to display without a second round-trip.
func (s *Server) handleListCaptures(w http.ResponseWriter, r *http.Request) {
	rows, err := s.pool.Query(r.Context(), `
		SELECT tc.id, tc.device_id, d.nickname, d.mac_address::text, tc.file_path,
		       tc.started_at, tc.stopped_at, tc.size_bytes, tc.status, tc.rotation_reason
		FROM traffic_captures tc
		JOIN devices d ON d.id = tc.device_id
		ORDER BY tc.started_at DESC
		LIMIT 200
	`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list captures")
		return
	}
	defer rows.Close()

	captures := []models.TrafficCapture{}
	for rows.Next() {
		var c models.TrafficCapture
		if err := rows.Scan(&c.ID, &c.DeviceID, &c.DeviceNickname, &c.DeviceMAC, &c.FilePath,
			&c.StartedAt, &c.StoppedAt, &c.SizeBytes, &c.Status, &c.RotationReason); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to read capture row")
			return
		}
		captures = append(captures, c)
	}
	writeJSON(w, http.StatusOK, captures)
}

// handleStartCapture begins a new on-demand recording of one device's
// traffic (the Userlar page's "● Trafik yozish" button). It only writes
// the traffic_captures row and asks capd to start immediately as a
// best-effort optimization — if capd is briefly unreachable, the next
// internal/capdsync tick retries automatically, so a transient failure
// here is never fatal to the request.
func (s *Server) handleStartCapture(w http.ResponseWriter, r *http.Request) {
	deviceID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid device id")
		return
	}

	ctx := r.Context()
	claims := claimsFromContext(r)

	var mac string
	if err := s.pool.QueryRow(ctx, `SELECT mac_address::text FROM devices WHERE id = $1`, deviceID).Scan(&mac); err != nil {
		writeError(w, http.StatusNotFound, "device not found")
		return
	}

	var alreadyRecording bool
	_ = s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM traffic_captures WHERE device_id = $1 AND status = 'recording')`, deviceID).Scan(&alreadyRecording)
	if alreadyRecording {
		writeError(w, http.StatusConflict, "this device already has an active capture")
		return
	}

	filePath := capdsync.NewFilePath(s.cfg.CaptureDir, deviceID)

	var newID int64
	if err := s.pool.QueryRow(ctx, `
		INSERT INTO traffic_captures (device_id, started_by_admin_id, file_path, status)
		VALUES ($1, $2, $3, 'recording') RETURNING id
	`, deviceID, claims.AdminID, filePath).Scan(&newID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create capture record")
		return
	}

	if err := capdsync.StartCapture(ctx, s.cfg.CapdSocket, newID, mac, filePath); err != nil {
		slog.Warn("capture will be picked up by the next capdsync tick", "capture_id", newID, "error", err)
	}

	_ = s.recordAudit(ctx, claims, "capture.start", "traffic_capture", &newID, map[string]interface{}{"device_id": deviceID})
	writeJSON(w, http.StatusCreated, map[string]interface{}{"id": newID})
}

// handleStopCapture ends a capture outright — unlike a rotation or a
// download, no continuation segment follows. The DB row is flipped to
// 'stopped' before capd is even asked to stop the process, so a
// concurrent capdsync tick never races to "helpfully" restart it (tick
// only touches rows still marked 'recording').
func (s *Server) handleStopCapture(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid capture id")
		return
	}

	ctx := r.Context()
	claims := claimsFromContext(r)

	tag, err := s.pool.Exec(ctx, `
		UPDATE traffic_captures SET status = 'stopped', stopped_at = now()
		WHERE id = $1 AND status = 'recording'
	`, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to stop capture")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusConflict, "capture is not currently recording")
		return
	}

	sizeBytes, err := capdsync.StopCapture(ctx, s.cfg.CapdSocket, id)
	if err != nil {
		slog.Warn("capdsync: failed to stop capd process (database already marked it stopped)", "capture_id", id, "error", err)
	} else {
		_, _ = s.pool.Exec(ctx, `UPDATE traffic_captures SET size_bytes = $1 WHERE id = $2`, sizeBytes, id)
	}

	_ = s.recordAudit(ctx, claims, "capture.stop", "traffic_capture", &id, nil)
	writeJSON(w, http.StatusOK, map[string]string{"status": "stopped"})
}

// handleDownloadCapture implements the spec's "Download bosilganda: yozib
// olish vaqtincha to'xtaydi, fayl to'liq holatga keltirilib yuklanadi,
// so'ng yangi faylga yozish davom etadi": if the capture is still
// 'recording', it's finalized (via the same RotateCapture path an
// automatic size/time rotation uses, with status='downloaded' and a
// continuation segment started immediately) before serving the file — a
// capture that already ended some other way is just served as-is.
func (s *Server) handleDownloadCapture(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid capture id")
		return
	}

	ctx := r.Context()
	claims := claimsFromContext(r)

	var deviceID int64
	var mac, filePath string
	var status models.CaptureStatus
	var startedByAdminID *int64
	err = s.pool.QueryRow(ctx, `
		SELECT tc.device_id, d.mac_address::text, tc.file_path, tc.status, tc.started_by_admin_id
		FROM traffic_captures tc JOIN devices d ON d.id = tc.device_id
		WHERE tc.id = $1
	`, id).Scan(&deviceID, &mac, &filePath, &status, &startedByAdminID)
	if err != nil {
		writeError(w, http.StatusNotFound, "capture not found")
		return
	}

	if status == models.CaptureRecording {
		if err := capdsync.RotateCapture(ctx, s.pool, s.cfg.CapdSocket, s.cfg.CaptureDir, id, deviceID, mac, startedByAdminID, models.CaptureDownloaded, "manual_download"); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to finalize capture for download: "+err.Error())
			return
		}
	}

	data, err := capdsync.ReadCaptureFile(ctx, s.cfg.CapdSocket, filePath)
	if err != nil {
		writeError(w, http.StatusGone, "capture file is no longer available (likely evicted by disk quota): "+err.Error())
		return
	}

	_ = s.recordAudit(ctx, claims, "capture.download", "traffic_capture", &id, nil)

	w.Header().Set("Content-Type", "application/vnd.tcpdump.pcap")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="capture_%d.pcap"`, id))
	w.Write(data)
}
