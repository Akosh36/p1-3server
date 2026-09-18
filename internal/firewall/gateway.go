package firewall

import (
	"fmt"
	"os"
)

// EnableIPForwarding turns on the kernel's IPv4 forwarding, without which
// nftables rules alone cannot make this host route traffic between
// interfaces — the packets would never reach the forward chain at all.
// Idempotent: writing "1" when it's already "1" is a no-op.
func EnableIPForwarding() error {
	const path = "/proc/sys/net/ipv4/ip_forward"
	if err := os.WriteFile(path, []byte("1\n"), 0644); err != nil {
		return fmt.Errorf("enable ip forwarding (%s): %w", path, err)
	}
	return nil
}
