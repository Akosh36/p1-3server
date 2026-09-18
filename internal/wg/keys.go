package wg

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ensurePrivateKey returns this gateway's WireGuard private key, generating
// one with `wg genkey` and persisting it (mode 0600) the first time wgd
// runs. Keeping the same key across restarts matters: remote peers
// identify this gateway by its derived public key, so a new key on every
// restart would silently break every existing tunnel.
func ensurePrivateKey(path string) (string, error) {
	if data, err := os.ReadFile(path); err == nil {
		return strings.TrimSpace(string(data)), nil
	}

	key, err := runWGCapture("genkey")
	if err != nil {
		return "", fmt.Errorf("generate private key: %w", err)
	}
	key = strings.TrimSpace(key)

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", fmt.Errorf("create key directory: %w", err)
	}
	if err := os.WriteFile(path, []byte(key+"\n"), 0o600); err != nil {
		return "", fmt.Errorf("write private key: %w", err)
	}
	return key, nil
}

// publicKeyFor derives the public key matching a private key, the same way
// `wg pubkey` does (Curve25519 base-point multiplication) — shelled out to
// the real `wg` binary rather than reimplementing the crypto, consistent
// with this project's "build on ready-made tools" approach (nft, tcpdump,
// dnsmasq).
func publicKeyFor(privateKey string) (string, error) {
	cmd := exec.Command("wg", "pubkey")
	cmd.Stdin = strings.NewReader(privateKey + "\n")
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("wg pubkey: %w: %s", err, stderr.String())
	}
	return strings.TrimSpace(out.String()), nil
}

func runWGCapture(args ...string) (string, error) {
	cmd := exec.Command("wg", args...)
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("wg %v: %w: %s", args, err, stderr.String())
	}
	return out.String(), nil
}
