package netdisc

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// RunHostapdCollector polls hostapd's control interface for currently
// associated wireless stations. hostapd creates one control socket per AP
// interface under socketDir (typically /var/run/hostapd/<iface>); every
// socket found there is polled. If socketDir doesn't exist (no local WiFi AP
// configured — decision #5 in CLAUDE.md), this simply does nothing.
func RunHostapdCollector(ctx context.Context, store *Store, socketDir string, ssid string, interval time.Duration) {
	if socketDir == "" {
		return
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		sockets, err := listHostapdSockets(socketDir)
		if err != nil {
			if !os.IsNotExist(err) {
				slog.Warn("netdiscd: failed to list hostapd sockets", "dir", socketDir, "error", err)
			}
		} else {
			now := time.Now()
			for _, sockPath := range sockets {
				macs, err := queryHostapdStations(sockPath)
				if err != nil {
					slog.Warn("netdiscd: hostapd query failed", "socket", sockPath, "error", err)
					continue
				}
				for _, mac := range macs {
					store.ObserveWireless(mac, ssid, now)
				}
			}
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func listHostapdSockets(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var sockets []string
	for _, e := range entries {
		if !e.IsDir() {
			sockets = append(sockets, filepath.Join(dir, e.Name()))
		}
	}
	return sockets, nil
}

// queryHostapdStations speaks hostapd's control-interface protocol (a
// connectionless Unix datagram socket): STA-FIRST returns the first
// associated station's MAC as the first line of the reply, STA-NEXT <mac>
// returns the next one, until the reply is "FAIL" (no more stations).
func queryHostapdStations(socketPath string) ([]string, error) {
	localPath := filepath.Join(os.TempDir(), fmt.Sprintf("netdiscd-hostapd-%d.sock", os.Getpid()))
	os.Remove(localPath)
	localAddr, err := net.ResolveUnixAddr("unixgram", localPath)
	if err != nil {
		return nil, err
	}
	conn, err := net.ListenUnixgram("unixgram", localAddr)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	defer os.Remove(localPath)

	remoteAddr, err := net.ResolveUnixAddr("unixgram", socketPath)
	if err != nil {
		return nil, err
	}

	var macs []string
	cmd := "STA-FIRST"
	for {
		if _, err := conn.WriteToUnix([]byte(cmd), remoteAddr); err != nil {
			return macs, err
		}
		conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		buf := make([]byte, 4096)
		n, err := conn.Read(buf)
		if err != nil {
			return macs, err
		}
		reply := strings.TrimSpace(string(buf[:n]))
		if reply == "" || strings.HasPrefix(reply, "FAIL") || strings.HasPrefix(reply, "N/A") {
			break
		}
		mac := strings.ToLower(strings.SplitN(reply, "\n", 2)[0])
		macs = append(macs, mac)
		cmd = "STA-NEXT " + mac
	}
	return macs, nil
}
