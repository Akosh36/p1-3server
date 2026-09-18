// Package netdisc implements netdiscd's collectors: passive ARP-table
// reading, DHCP lease parsing, SNMP switch-port polling, and hostapd
// wireless-client queries. It has no database access at all — see
// docs/deploy.md's data-plane/control-plane split — it only ever produces a
// point-in-time Snapshot that the control-plane API pulls over a local Unix
// socket and reconciles into Postgres (internal/discovery).
package netdisc

import "time"

// ObservedDevice is one row of netdiscd's in-memory view of a device it has
// seen on the LAN, keyed by MAC address. It intentionally does not carry a
// database ID: netdiscd doesn't know about Postgres, and the control-plane
// resolves identity by mac_address on upsert.
type ObservedDevice struct {
	MAC        string    `json:"mac_address"`
	IP         string    `json:"ip_address,omitempty"`
	Hostname   string    `json:"hostname,omitempty"`
	ConnType   string    `json:"conn_type"` // "wired" | "wireless_local"
	SSID       string    `json:"ssid,omitempty"`
	SwitchName string    `json:"switch_name,omitempty"`
	PortNumber int       `json:"port_number,omitempty"`
	IsOnline   bool      `json:"is_online"`
	LastSeenAt time.Time `json:"last_seen_at"`
}

// ObservedSwitchPort is one row of the "Portlar" table — a physical switch
// port and whatever netdiscd currently knows about its link state.
type ObservedSwitchPort struct {
	SwitchName string `json:"switch_name"`
	PortNumber int    `json:"port_number"`
	Label      string `json:"label,omitempty"`
	VLAN       int    `json:"vlan,omitempty"`
	LinkStatus bool   `json:"link_status"`
}

// Snapshot is the full payload netdiscd serves over its control socket.
type Snapshot struct {
	Devices     []ObservedDevice     `json:"devices"`
	SwitchPorts []ObservedSwitchPort `json:"switch_ports"`
	GeneratedAt time.Time            `json:"generated_at"`
}
