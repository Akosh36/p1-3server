package firewall

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
)

// Apply loads ruleset (as produced by BuildRuleset) into the kernel via
// `nft -f -`, which nft treats as a single atomic transaction — either
// every command in it takes effect or none does. This is the only place in
// the codebase that shells out to nft.
func Apply(ctx context.Context, ruleset string) error {
	cmd := exec.CommandContext(ctx, "nft", "-f", "-")
	cmd.Stdin = bytes.NewBufferString(ruleset)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("nft -f: %w: %s", err, stderr.String())
	}
	return nil
}

// Teardown removes the table entirely (used on clean shutdown / tests so a
// dev machine isn't left with a stale default-deny table after fwctl exits).
func Teardown(ctx context.Context, cfg RulesetConfig) error {
	cmd := exec.CommandContext(ctx, "nft", "delete", "table", "inet", cfg.tableName())
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("nft delete table: %w: %s", err, stderr.String())
	}
	return nil
}
