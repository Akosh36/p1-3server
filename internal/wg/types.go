// Package wg is the data-plane half of Phase 7: a real site-to-site
// WireGuard tunnel per remote LAN. Like netdiscd/fwctl/lbd/capd, it never
// touches Postgres — the control-plane's internal/wgsync tells it the
// desired peer list over a local Unix socket, and reads live handshake
// status back the same way.
//
// wgd is deliberately "dumb" about anything beyond the tunnel itself: it
// doesn't know which admin created a peer or which lan_networks row it
// belongs to — internal/wgsync owns that mapping and decides what
// "desired" means (a peer is only pushed as active when both its own
// vpn_peers.is_active and its lan_network's is_active are true).
package wg

// Peer is one remote site's tunnel configuration, as internal/wgsync wants
// it applied to the local WireGuard interface.
type Peer struct {
	PublicKey     string `json:"public_key"`
	AllowedSubnet string `json:"allowed_subnet"` // CIDR of the remote LAN, e.g. "192.168.50.0/24"
	Endpoint      string `json:"endpoint,omitempty"`
}

// SyncRequest is the desired full set of active peers — anything currently
// configured on the interface but not in this list is removed.
type SyncRequest struct {
	Peers []Peer `json:"peers"`
}

// PeerStatus is one peer's live handshake state.
type PeerStatus struct {
	PublicKey       string `json:"public_key"`
	LastHandshakeAt string `json:"last_handshake_at,omitempty"` // RFC3339, empty if no handshake yet
	IsReachable     bool   `json:"is_reachable"`
}

// Status is GET /status's body — the local tunnel identity (so an admin
// can hand the public key to the remote site's admin) plus every
// currently-configured peer's live state.
type Status struct {
	LocalPublicKey string       `json:"local_public_key"`
	ListenPort     int          `json:"listen_port"`
	Peers          []PeerStatus `json:"peers"`
}
