// Package metrics periodically samples the host's own CPU/RAM/disk/network
// usage (via internal/hostmetrics) and appends it to system_metrics, so the
// "Server" card of the admin panel has a real, continuously-updating
// history to chart.
package metrics

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Akosh36/p1-3server/internal/hostmetrics"
)

// Run samples system metrics every `interval` until ctx is cancelled. It is
// meant to be started once as a background goroutine from cmd/api.
func Run(ctx context.Context, pool *pgxpool.Pool, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	sampler := hostmetrics.NewSampler()

	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			sample, ok := sampler.Sample(now)
			if !ok {
				continue
			}
			if err := insert(ctx, pool, sample); err != nil {
				slog.Error("metrics: failed to insert sample", "error", err)
			}
		}
	}
}

func insert(ctx context.Context, pool *pgxpool.Pool, s hostmetrics.Sample) error {
	_, err := pool.Exec(ctx, `
		INSERT INTO system_metrics
			(cpu_percent, mem_percent, disk_read_bps, disk_write_bps, net_in_bps, net_out_bps, disk_percent)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, s.CPUPercent, s.MemPercent, s.DiskReadBps, s.DiskWriteBps, s.NetInBps, s.NetOutBps, s.DiskPercent)
	return err
}
