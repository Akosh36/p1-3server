// Package lb implements lbd's L4 TCP load balancer: one listener per
// server group's VIP, proxying to a pool of backends chosen by
// round-robin or least-connections, with active health checking. Like
// netdiscd and fwctl, it never touches Postgres — internal/lbsync
// (control-plane) pushes the desired groups/backends over lbd's local Unix
// socket and pulls health results back the same way.
package lb

import (
	"net"
	"strconv"
)

// Backend is one target a group's VIP can proxy to.
type Backend struct {
	IP     string `json:"ip"`
	Port   int    `json:"port"`
	Weight int    `json:"weight"`
}

func (b Backend) addr() string {
	return net.JoinHostPort(b.IP, strconv.Itoa(b.Port))
}

// Group is one server_groups row: a VIP that proxies to a pool of backends.
type Group struct {
	Nickname   string    `json:"nickname"` // for logs only, not used as a key
	VIPAddress string    `json:"vip_address"`
	VIPPort    int       `json:"vip_port"`
	Algorithm  string    `json:"algorithm"` // "round_robin" | "least_conn"
	Backends   []Backend `json:"backends"`
}

func (g Group) vipAddr() string {
	return net.JoinHostPort(g.VIPAddress, strconv.Itoa(g.VIPPort))
}

// key identifies a running listener independent of which backends/algorithm
// it currently has — changing those swaps the pool in place; only a
// VIP:port change requires stopping and restarting the listener itself.
func (g Group) key() string {
	return g.vipAddr()
}

// BackendHealth is what GET /status reports per backend, and what
// internal/lbsync writes back into backend_servers (is_healthy,
// last_check_at, response_time_ms).
type BackendHealth struct {
	IP             string  `json:"ip"`
	Port           int     `json:"port"`
	IsHealthy      bool    `json:"is_healthy"`
	LastCheckAt    string  `json:"last_check_at"`
	ResponseTimeMs float64 `json:"response_time_ms"`
	ActiveConns    int64   `json:"active_conns"`
}

type GroupStatus struct {
	VIPAddress string          `json:"vip_address"`
	VIPPort    int             `json:"vip_port"`
	Backends   []BackendHealth `json:"backends"`
}

type Status struct {
	Groups []GroupStatus `json:"groups"`
}

// ClientTraffic is one client IP's accumulated byte counts through one
// group's VIP since the last drain. lbd knows neither which Postgres
// device a client IP belongs to nor which server_groups row a VIP
// address maps to — internal/lbsync resolves both when it polls
// GET /traffic and attributes the bytes into device_traffic_stats.
type ClientTraffic struct {
	VIPAddress string `json:"vip_address"`
	ClientIP   string `json:"client_ip"`
	BytesUp    int64  `json:"bytes_up"`   // client -> backend (request/upload)
	BytesDown  int64  `json:"bytes_down"` // backend -> client (response/download)
}

type TrafficResponse struct {
	Entries []ClientTraffic `json:"entries"`
}
