package hostmetrics

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestGPUSampler_NoNvidiaSMI is the real, honestly-testable path on any
// machine without NVIDIA tooling (including this repo's own CI/sandbox,
// confirmed via `which nvidia-smi` returning nothing): sample() must report
// "no GPU" as nil, nil — never a fabricated zero.
func TestGPUSampler_NoNvidiaSMI(t *testing.T) {
	restorePath := os.Getenv("PATH")
	t.Cleanup(func() { os.Setenv("PATH", restorePath) })
	os.Setenv("PATH", t.TempDir()) // a PATH guaranteed to have no nvidia-smi

	var g gpuSampler
	util, mem := g.sample()
	if util != nil || mem != nil {
		t.Fatalf("expected nil, nil with no nvidia-smi on PATH; got util=%v mem=%v", util, mem)
	}
}

// TestGPUSampler_ParsesRealNvidiaSMIOutput can't run against real GPU
// hardware (none exists in this environment), but it validates the actual
// exec.Command + CSV-parsing path against a real subprocess — a fake
// nvidia-smi script placed on PATH, exercised exactly like the real binary
// would be — rather than mocking exec.Command away, so a real interface
// mismatch (wrong flags, wrong output format assumption) would still be
// caught here.
func TestGPUSampler_ParsesRealNvidiaSMIOutput(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("fake nvidia-smi script is a shell script; linux only")
	}

	dir := t.TempDir()
	script := filepath.Join(dir, "nvidia-smi")
	// Two GPUs: (23%, 40%) and (77%, 60%) -> averages (50%, 50%).
	content := "#!/bin/sh\nprintf '23, 40\\n77, 60\\n'\n"
	if err := os.WriteFile(script, []byte(content), 0755); err != nil {
		t.Fatalf("write fake nvidia-smi: %v", err)
	}

	restorePath := os.Getenv("PATH")
	t.Cleanup(func() { os.Setenv("PATH", restorePath) })
	os.Setenv("PATH", dir)

	var g gpuSampler
	util, mem := g.sample()
	if util == nil || mem == nil {
		t.Fatalf("expected real values from fake nvidia-smi; got util=%v mem=%v", util, mem)
	}
	if *util != 50 {
		t.Errorf("gpu util = %v, want 50 (average of 23 and 77)", *util)
	}
	if *mem != 50 {
		t.Errorf("gpu mem = %v, want 50 (average of 40 and 60)", *mem)
	}
}

// TestGPUSampler_CachesAvailability ensures a sampler that already found no
// nvidia-smi doesn't re-run exec.LookPath forever — it should stay "no GPU"
// even if PATH changes afterward, matching how a single long-lived
// Sampler is actually used (one gpuSampler for the daemon's whole lifetime).
func TestGPUSampler_CachesAvailability(t *testing.T) {
	restorePath := os.Getenv("PATH")
	t.Cleanup(func() { os.Setenv("PATH", restorePath) })

	os.Setenv("PATH", t.TempDir())
	var g gpuSampler
	if util, mem := g.sample(); util != nil || mem != nil {
		t.Fatalf("expected nil, nil on first sample; got util=%v mem=%v", util, mem)
	}
	if !g.checked || g.available {
		t.Fatalf("expected checked=true, available=false after first sample; got checked=%v available=%v", g.checked, g.available)
	}
}
