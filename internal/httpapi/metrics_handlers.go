package httpapi

import (
	"net/http"
	"strconv"

	"github.com/Akosh36/p1-3server/internal/models"
)

// handleSelfMetrics returns the most recent sample collected by
// internal/metrics.Run for the "Server" card's live gauges.
func (s *Server) handleSelfMetrics(w http.ResponseWriter, r *http.Request) {
	var m models.SystemMetric
	err := s.pool.QueryRow(r.Context(), `
		SELECT time, cpu_percent, mem_percent, disk_read_bps, disk_write_bps, net_in_bps, net_out_bps, disk_percent
		FROM system_metrics ORDER BY time DESC LIMIT 1
	`).Scan(&m.Time, &m.CPUPercent, &m.MemPercent, &m.DiskReadBps, &m.DiskWriteBps, &m.NetInBps, &m.NetOutBps, &m.DiskPercent)
	if err != nil {
		writeJSON(w, http.StatusOK, nil) // no sample collected yet
		return
	}
	writeJSON(w, http.StatusOK, m)
}

// handleMetricsHistory returns the last N samples (default 120, ~ the last
// 20 minutes at a 10s collection interval) for the response-time / system
// health charts.
func (s *Server) handleMetricsHistory(w http.ResponseWriter, r *http.Request) {
	limit := 120
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 2000 {
			limit = n
		}
	}

	rows, err := s.pool.Query(r.Context(), `
		SELECT time, cpu_percent, mem_percent, disk_read_bps, disk_write_bps, net_in_bps, net_out_bps, disk_percent
		FROM system_metrics ORDER BY time DESC LIMIT $1
	`, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load metrics history")
		return
	}
	defer rows.Close()

	history := []models.SystemMetric{}
	for rows.Next() {
		var m models.SystemMetric
		if err := rows.Scan(&m.Time, &m.CPUPercent, &m.MemPercent, &m.DiskReadBps, &m.DiskWriteBps, &m.NetInBps, &m.NetOutBps, &m.DiskPercent); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to read metrics row")
			return
		}
		history = append(history, m)
	}
	writeJSON(w, http.StatusOK, history)
}
