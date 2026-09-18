package wg

import (
	"bytes"
	"fmt"
	"os/exec"
	"time"
)

// bringUpInterface creates the WireGuard network interface, preferring the
// native kernel implementation (mainline since Linux 5.6) and falling back
// to the userspace `wireguard-go` implementation exactly like `wg-quick`
// does when the kernel module isn't loadable — a real production fallback
// (some minimal/container kernels ship without it), not just a test shim.
//
// When the fallback is used, wireguard-go is run with -f (foreground)
// rather than left to daemonize, so its lifecycle can be managed the same
// deliberate way capd manages tcpdump: one goroutine owns Wait() (see
// internal/capd/manager.go's doc comment for why calling Wait() from two
// places can hang forever), which the caller reads back via done.
func bringUpInterface(iface string) (userspace *exec.Cmd, done chan struct{}, err error) {
	if runIP("link", "add", iface, "type", "wireguard") == nil {
		return nil, nil, nil
	}

	cmd := exec.Command("wireguard-go", "-f", iface)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if startErr := cmd.Start(); startErr != nil {
		return nil, nil, fmt.Errorf("kernel WireGuard unavailable and starting wireguard-go fallback failed: %w: %s", startErr, stderr.String())
	}

	doneCh := make(chan struct{})
	go func() {
		cmd.Wait()
		close(doneCh)
	}()

	if waitErr := waitForInterface(iface, 5*time.Second); waitErr != nil {
		_ = cmd.Process.Kill()
		<-doneCh
		return nil, nil, waitErr
	}
	return cmd, doneCh, nil
}

func waitForInterface(iface string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if runIP("link", "show", iface) == nil {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("interface %s did not appear within %s", iface, timeout)
}

func runIP(args ...string) error {
	cmd := exec.Command("ip", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ip %v: %w: %s", args, err, stderr.String())
	}
	return nil
}

func runWG(args ...string) error {
	cmd := exec.Command("wg", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("wg %v: %w: %s", args, err, stderr.String())
	}
	return nil
}
