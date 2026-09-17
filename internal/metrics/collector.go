// Package metrics periodically samples the host's own CPU/RAM/disk/network
// usage (gopsutil) and appends it to system_metrics, so the "Server" card of
// the admin panel has a real, continuously-updating history to chart.
package metrics

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/net"
)

// Run samples system metrics every `interval` until ctx is cancelled. It is
// meant to be started once as a background goroutine from cmd/api.
func Run(ctx context.Context, pool *pgxpool.Pool, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var lastNetIn, lastNetOut uint64
	var lastDiskRead, lastDiskWrite uint64
	var lastSampleAt time.Time
	haveBaseline := false

	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			netIn, netOut, _ := sumNetIO()
			diskRead, diskWrite, _ := sumDiskIO()

			if haveBaseline {
				elapsed := now.Sub(lastSampleAt).Seconds()
				if elapsed > 0 {
					s := collectOne(elapsed, lastNetIn, lastNetOut, netIn, netOut, lastDiskRead, lastDiskWrite, diskRead, diskWrite)
					if err := insert(ctx, pool, s); err != nil {
						slog.Error("metrics: failed to insert sample", "error", err)
					}
				}
			}

			lastNetIn, lastNetOut = netIn, netOut
			lastDiskRead, lastDiskWrite = diskRead, diskWrite
			lastSampleAt = now
			haveBaseline = true
		}
	}
}

type sample struct {
	cpuPercent   float64
	memPercent   float64
	diskPercent  float64
	diskReadBps  int64
	diskWriteBps int64
	netInBps     int64
	netOutBps    int64
}

func sumNetIO() (bytesRecv, bytesSent uint64, err error) {
	stats, err := net.IOCounters(false)
	if err != nil || len(stats) == 0 {
		return 0, 0, err
	}
	return stats[0].BytesRecv, stats[0].BytesSent, nil
}

func sumDiskIO() (readBytes, writeBytes uint64, err error) {
	stats, err := disk.IOCounters()
	if err != nil {
		return 0, 0, err
	}
	for _, d := range stats {
		readBytes += d.ReadBytes
		writeBytes += d.WriteBytes
	}
	return readBytes, writeBytes, nil
}

func collectOne(elapsedSeconds float64, prevNetIn, prevNetOut, netIn, netOut, prevDiskRead, prevDiskWrite, diskRead, diskWrite uint64) sample {
	var s sample

	if cpuPercents, err := cpu.Percent(0, false); err == nil && len(cpuPercents) > 0 {
		s.cpuPercent = cpuPercents[0]
	}
	if vm, err := mem.VirtualMemory(); err == nil {
		s.memPercent = vm.UsedPercent
	}
	if du, err := disk.Usage("/"); err == nil {
		s.diskPercent = du.UsedPercent
	}

	s.netInBps = rateBps(prevNetIn, netIn, elapsedSeconds)
	s.netOutBps = rateBps(prevNetOut, netOut, elapsedSeconds)
	s.diskReadBps = rateBps(prevDiskRead, diskRead, elapsedSeconds)
	s.diskWriteBps = rateBps(prevDiskWrite, diskWrite, elapsedSeconds)

	return s
}

// rateBps computes a bytes-per-second rate from two cumulative counter
// readings, treating a counter that went backwards (interface reset, wrap)
// as "no data this tick" instead of underflowing to a huge uint64 delta.
func rateBps(prev, cur uint64, elapsedSeconds float64) int64 {
	if cur < prev {
		return 0
	}
	return int64(float64(cur-prev) / elapsedSeconds)
}

func insert(ctx context.Context, pool *pgxpool.Pool, s sample) error {
	_, err := pool.Exec(ctx, `
		INSERT INTO system_metrics
			(cpu_percent, mem_percent, disk_read_bps, disk_write_bps, net_in_bps, net_out_bps, disk_percent)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, s.cpuPercent, s.memPercent, s.diskReadBps, s.diskWriteBps, s.netInBps, s.netOutBps, s.diskPercent)
	return err
}
