package capd

import (
	"context"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"time"
)

const quotaCheckInterval = 10 * time.Second

// RunQuotaEnforcement periodically scans captureDir and deletes the oldest
// (by mtime) *.pcap files until total usage is back under maxTotalBytes,
// skipping any file a capture is currently writing to. It scans the whole
// directory rather than only files capd started itself, so the quota holds
// even across a capd restart. Deleted paths are queued for Status() to
// report, so the control-plane can mark the corresponding traffic_captures
// rows honestly (status='error') instead of leaving a dangling file_path.
func (m *Manager) RunQuotaEnforcement(ctx context.Context) {
	ticker := time.NewTicker(quotaCheckInterval)
	defer ticker.Stop()
	for {
		m.enforceQuota()
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

type fileEntry struct {
	path    string
	size    int64
	modTime time.Time
}

func (m *Manager) enforceQuota() {
	if m.maxTotalBytes <= 0 {
		return // quota disabled
	}

	var files []fileEntry
	var total int64
	_ = filepath.WalkDir(m.captureDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(path) != ".pcap" {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		files = append(files, fileEntry{path: path, size: info.Size(), modTime: info.ModTime()})
		total += info.Size()
		return nil
	})

	if total <= m.maxTotalBytes {
		return
	}

	m.mu.Lock()
	activePaths := make(map[string]bool, len(m.running))
	for _, rc := range m.running {
		activePaths[rc.filePath] = true
	}
	m.mu.Unlock()

	sort.Slice(files, func(i, j int) bool { return files[i].modTime.Before(files[j].modTime) })

	var evicted []string
	for _, f := range files {
		if total <= m.maxTotalBytes {
			break
		}
		if activePaths[f.path] {
			continue // never delete a file mid-write
		}
		if err := os.Remove(f.path); err != nil {
			slog.Warn("capd: quota eviction failed", "file", f.path, "error", err)
			continue
		}
		slog.Info("capd: evicted file for disk quota", "file", f.path, "size_bytes", f.size)
		total -= f.size
		evicted = append(evicted, f.path)
	}

	if len(evicted) > 0 {
		m.mu.Lock()
		m.evicted = append(m.evicted, evicted...)
		m.mu.Unlock()
	}
}
