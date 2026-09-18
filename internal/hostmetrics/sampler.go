// Package hostmetrics samples the local host's CPU/RAM/disk/network usage
// via gopsutil. It's shared by two callers that otherwise have nothing to
// do with each other: internal/metrics (the gateway's own self-monitoring,
// run inside cmd/api) and cmd/backendagentd (the Phase 5 push-agent that
// runs on each load-balanced backend server) — both just want "the current
// host's numbers," so the sampling and byte-rate math is written once here.
package hostmetrics

import (
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/net"
)

// Sample is one point-in-time reading. Disk/network fields are rates
// (bytes/sec), not cumulative counters, computed by Sampler from two
// successive readings.
type Sample struct {
	CPUPercent   float64 `json:"cpu_percent"`
	MemPercent   float64 `json:"mem_percent"`
	DiskPercent  float64 `json:"disk_percent"`
	DiskReadBps  int64   `json:"disk_read_bps"`
	DiskWriteBps int64   `json:"disk_write_bps"`
	NetInBps     int64   `json:"net_in_bps"`
	NetOutBps    int64   `json:"net_out_bps"`
}

// Sampler turns gopsutil's cumulative disk/network counters into
// bytes-per-second rates by remembering the previous reading. The first
// call after construction has no baseline yet, so it returns ok=false.
type Sampler struct {
	lastNetIn, lastNetOut       uint64
	lastDiskRead, lastDiskWrite uint64
	lastSampleAt                time.Time
	haveBaseline                bool
}

func NewSampler() *Sampler {
	return &Sampler{}
}

// Sample takes a reading at time `now` and returns the computed rates since
// the previous call. ok is false on the first call (no prior reading to
// diff against) and whenever `now` doesn't advance past the last sample.
func (s *Sampler) Sample(now time.Time) (sample Sample, ok bool) {
	netIn, netOut, _ := sumNetIO()
	diskRead, diskWrite, _ := sumDiskIO()

	if s.haveBaseline {
		elapsed := now.Sub(s.lastSampleAt).Seconds()
		if elapsed > 0 {
			sample = collectOne(elapsed, s.lastNetIn, s.lastNetOut, netIn, netOut, s.lastDiskRead, s.lastDiskWrite, diskRead, diskWrite)
			ok = true
		}
	}

	s.lastNetIn, s.lastNetOut = netIn, netOut
	s.lastDiskRead, s.lastDiskWrite = diskRead, diskWrite
	s.lastSampleAt = now
	s.haveBaseline = true
	return sample, ok
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

func collectOne(elapsedSeconds float64, prevNetIn, prevNetOut, netIn, netOut, prevDiskRead, prevDiskWrite, diskRead, diskWrite uint64) Sample {
	var s Sample

	if cpuPercents, err := cpu.Percent(0, false); err == nil && len(cpuPercents) > 0 {
		s.CPUPercent = cpuPercents[0]
	}
	if vm, err := mem.VirtualMemory(); err == nil {
		s.MemPercent = vm.UsedPercent
	}
	if du, err := disk.Usage("/"); err == nil {
		s.DiskPercent = du.UsedPercent
	}

	s.NetInBps = rateBps(prevNetIn, netIn, elapsedSeconds)
	s.NetOutBps = rateBps(prevNetOut, netOut, elapsedSeconds)
	s.DiskReadBps = rateBps(prevDiskRead, diskRead, elapsedSeconds)
	s.DiskWriteBps = rateBps(prevDiskWrite, diskWrite, elapsedSeconds)

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
