package hostmetrics

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const gpuQueryTimeout = 2 * time.Second

// gpuSampler shells out to nvidia-smi for real GPU utilization/memory
// figures. There is no cross-vendor equivalent to gopsutil for GPUs, and
// this platform has no requirement to support anything but NVIDIA, so
// "nvidia-smi present" is the only supported path — anything else (no GPU,
// an AMD/Intel-only box, nvidia-smi not on PATH) reports nil, never a
// fabricated value.
type gpuSampler struct {
	checked   bool
	available bool
}

// sample returns (nil, nil) whenever no NVIDIA GPU tooling is available.
// When multiple GPUs are present, it averages across them — good enough
// for a single "GPU load" gauge; per-GPU breakdown isn't part of this
// platform's scope.
func (g *gpuSampler) sample() (gpuPercent, gpuMemPercent *float64) {
	if !g.checked {
		_, err := exec.LookPath("nvidia-smi")
		g.available = err == nil
		g.checked = true
	}
	if !g.available {
		return nil, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), gpuQueryTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "nvidia-smi",
		"--query-gpu=utilization.gpu,utilization.memory",
		"--format=csv,noheader,nounits").Output()
	if err != nil {
		return nil, nil
	}

	var sumUtil, sumMem float64
	var n int
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		fields := strings.Split(line, ",")
		if len(fields) != 2 {
			continue
		}
		util, errU := strconv.ParseFloat(strings.TrimSpace(fields[0]), 64)
		mem, errM := strconv.ParseFloat(strings.TrimSpace(fields[1]), 64)
		if errU != nil || errM != nil {
			continue
		}
		sumUtil += util
		sumMem += mem
		n++
	}
	if n == 0 {
		return nil, nil
	}
	avgUtil := sumUtil / float64(n)
	avgMem := sumMem / float64(n)
	return &avgUtil, &avgMem
}
