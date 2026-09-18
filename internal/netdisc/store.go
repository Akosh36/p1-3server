package netdisc

import (
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Store is netdiscd's thread-safe in-memory view of the LAN. Each collector
// (arp.go, dnsmasq.go, snmp.go, hostapd.go) calls one of the Observe*
// methods as it runs; Snapshot() renders the current merged state for the
// control socket to serve.
//
// A device is never deleted once seen — going quiet only clears IsOnline
// (via Prune). Admins still need to see, and revoke access from, a device
// that has stepped away, per the "default-deny" model in CLAUDE.md.
type Store struct {
	mu          sync.Mutex
	devices     map[string]*ObservedDevice     // key: normalized MAC
	switchPorts map[string]*ObservedSwitchPort // key: "switchName/portNumber"
}

func NewStore() *Store {
	return &Store{
		devices:     make(map[string]*ObservedDevice),
		switchPorts: make(map[string]*ObservedSwitchPort),
	}
}

func normalizeMAC(mac string) string {
	return strings.ToLower(strings.TrimSpace(mac))
}

func (s *Store) getOrCreate(mac string) *ObservedDevice {
	mac = normalizeMAC(mac)
	d, ok := s.devices[mac]
	if !ok {
		// LastSeenAt starts at "now" (discovery time) even for a device only
		// known from a DHCP lease, so it never renders as epoch/year-0001 in
		// the UI. Only an actual ARP/wireless sighting advances it further —
		// IsOnline stays false until one does.
		d = &ObservedDevice{MAC: mac, ConnType: "wired", LastSeenAt: time.Now()}
		s.devices[mac] = d
	}
	return d
}

// ObserveARP records that mac is currently resolvable at ip (from the
// kernel's ARP/neighbor table). This is the primary "is it actually here
// right now" signal for wired devices.
func (s *Store) ObserveARP(mac, ip string, seenAt time.Time) {
	if mac == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	d := s.getOrCreate(mac)
	if ip != "" {
		d.IP = ip
	}
	d.IsOnline = true
	d.LastSeenAt = seenAt
}

// ObserveDHCPLease records the hostname a device reported when it requested
// its lease. A lease alone does not mean the device is online right now
// (it may be asleep) — only ARP/wireless sightings set IsOnline.
func (s *Store) ObserveDHCPLease(mac, ip, hostname string) {
	if mac == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	d := s.getOrCreate(mac)
	if hostname != "" && hostname != "*" {
		d.Hostname = hostname
	}
	if ip != "" && d.IP == "" {
		d.IP = ip
	}
}

// ObserveWireless records that mac is currently associated with the local
// access point on the given SSID (from hostapd's station list).
func (s *Store) ObserveWireless(mac, ssid string, seenAt time.Time) {
	if mac == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	d := s.getOrCreate(mac)
	d.ConnType = "wireless_local"
	if ssid != "" {
		d.SSID = ssid
	}
	d.IsOnline = true
	d.LastSeenAt = seenAt
}

// ObserveSwitchFDB records that mac was found in a managed switch's
// forwarding database on the given port (SNMP dot1dTpFdbTable), i.e. it is
// a wired device behind that specific port.
func (s *Store) ObserveSwitchFDB(mac, switchName string, port int) {
	if mac == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	d := s.getOrCreate(mac)
	d.ConnType = "wired"
	d.SwitchName = switchName
	d.PortNumber = port
}

// SetSwitchPort upserts a physical switch port's current link state
// (SNMP IF-MIB polling), independent of which device (if any) sits behind it.
func (s *Store) SetSwitchPort(switchName string, port int, label string, vlan int, linkStatus bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := switchPortKey(switchName, port)
	p, ok := s.switchPorts[key]
	if !ok {
		p = &ObservedSwitchPort{SwitchName: switchName, PortNumber: port}
		s.switchPorts[key] = p
	}
	p.Label = label
	p.VLAN = vlan
	p.LinkStatus = linkStatus
}

func switchPortKey(switchName string, port int) string {
	return switchName + "/" + strconv.Itoa(port)
}

// Prune marks any device not observed within staleAfter as offline. It never
// deletes a device — see the Store doc comment.
func (s *Store) Prune(staleAfter time.Duration, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, d := range s.devices {
		if d.IsOnline && now.Sub(d.LastSeenAt) > staleAfter {
			d.IsOnline = false
		}
	}
}

// Snapshot renders the current merged state, sorted for stable diffs/tests.
func (s *Store) Snapshot() Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()

	devices := make([]ObservedDevice, 0, len(s.devices))
	for _, d := range s.devices {
		devices = append(devices, *d)
	}
	sort.Slice(devices, func(i, j int) bool { return devices[i].MAC < devices[j].MAC })

	ports := make([]ObservedSwitchPort, 0, len(s.switchPorts))
	for _, p := range s.switchPorts {
		ports = append(ports, *p)
	}
	sort.Slice(ports, func(i, j int) bool {
		if ports[i].SwitchName != ports[j].SwitchName {
			return ports[i].SwitchName < ports[j].SwitchName
		}
		return ports[i].PortNumber < ports[j].PortNumber
	})

	return Snapshot{Devices: devices, SwitchPorts: ports, GeneratedAt: time.Now()}
}
