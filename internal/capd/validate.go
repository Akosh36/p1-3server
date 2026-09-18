package capd

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

var macRE = regexp.MustCompile(`^([0-9a-fA-F]{2}:){5}[0-9a-fA-F]{2}$`)

func validMAC(mac string) bool {
	return macRE.MatchString(mac)
}

// validatePath resolves a requested file path and rejects anything outside
// captureDir — the control-plane is trusted, but a capture file path is
// handed to `tcpdump -w` and later read back for a download, so a path
// that escaped the capture directory (via "..", a symlink-like typo, etc.)
// is refused rather than silently followed.
func validatePath(captureDir, requested string) (string, error) {
	clean := filepath.Clean(requested)
	base := filepath.Clean(captureDir)
	if clean != base && !strings.HasPrefix(clean, base+string(filepath.Separator)) {
		return "", fmt.Errorf("file_path %q is outside capture directory %q", requested, captureDir)
	}
	return clean, nil
}
