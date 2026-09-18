package lb

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
)

// VIPs are added as /32 secondary addresses directly on the LAN-facing
// interface (never a separate dummy interface): the kernel then answers
// ARP for a VIP the same way it would for the interface's primary address,
// so LAN clients on the same subnet reach it exactly like any other host —
// no proxy_arp tricks, matching the "one IP per group" design in CLAUDE.md
// §2.3/§4.

func addVIP(ctx context.Context, iface, vip string) error {
	return runIP(ctx, "addr", "add", vip+"/32", "dev", iface)
}

func delVIP(ctx context.Context, iface, vip string) error {
	return runIP(ctx, "addr", "del", vip+"/32", "dev", iface)
}

func runIP(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, "ip", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ip %v: %w: %s", args, err, stderr.String())
	}
	return nil
}
