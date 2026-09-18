package netdisc

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/gosnmp/gosnmp"
)

// Standard MIB-II / BRIDGE-MIB OIDs used to poll a managed switch.
const (
	oidIfDescr        = ".1.3.6.1.2.1.2.2.1.2"    // IF-MIB::ifDescr
	oidIfOperStatus   = ".1.3.6.1.2.1.2.2.1.8"    // IF-MIB::ifOperStatus (1=up, 2=down)
	oidDot1dTpFdbPort = ".1.3.6.1.2.1.17.4.3.1.2" // BRIDGE-MIB::dot1dTpFdbPort — OID suffix is the learned MAC
)

const ifOperStatusUp = 1

// SNMPConfig points netdiscd at a managed switch to poll for port link
// status (IF-MIB) and, best-effort, the MAC-address-to-port table
// (BRIDGE-MIB dot1dTpFdbTable — not every switch populates this over SNMP,
// so a failure here only skips wired MAC-to-port mapping, it never stops
// the ifTable poll).
type SNMPConfig struct {
	SwitchName string
	Target     string // "ip:port", e.g. "192.168.1.2:161"
	Community  string
	Timeout    time.Duration
}

// RunSNMPCollector polls the configured switch every interval. If cfg.Target
// is empty, it does nothing — SNMP switch polling is optional (decision #5
// in CLAUDE.md: only meaningful when the LAN actually has a managed switch).
func RunSNMPCollector(ctx context.Context, store *Store, cfg SNMPConfig, interval time.Duration) {
	if cfg.Target == "" {
		return
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		if err := pollSwitch(store, cfg); err != nil {
			slog.Warn("netdiscd: SNMP poll failed", "target", cfg.Target, "error", err)
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func pollSwitch(store *Store, cfg SNMPConfig) error {
	host, portStr, err := splitHostPort(cfg.Target)
	if err != nil {
		return fmt.Errorf("invalid snmp target %q: %w", cfg.Target, err)
	}
	port, err := strconv.ParseUint(portStr, 10, 16)
	if err != nil {
		return fmt.Errorf("invalid snmp port %q: %w", portStr, err)
	}

	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 3 * time.Second
	}

	client := &gosnmp.GoSNMP{
		Target:    host,
		Port:      uint16(port),
		Community: cfg.Community,
		Version:   gosnmp.Version2c,
		Timeout:   timeout,
		Retries:   1,
	}
	if err := client.Connect(); err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer client.Conn.Close()

	if err := pollInterfaceTable(store, client, cfg.SwitchName); err != nil {
		return fmt.Errorf("poll interface table: %w", err)
	}

	// Best-effort: not every switch exposes the bridge forwarding table.
	if err := pollForwardingTable(store, client, cfg.SwitchName); err != nil {
		slog.Debug("netdiscd: SNMP forwarding table unavailable", "target", cfg.Target, "error", err)
	}

	return nil
}

// pollInterfaceTable walks IF-MIB's ifDescr and ifOperStatus to populate the
// "Portlar" table's labels and link status, using ifIndex as the port
// number. On simple/unmanaged-in-software switches ifIndex lines up 1:1
// with the physical port; larger enterprise switches may need a
// switch-specific ifIndex→physical-port mapping, which is out of scope for
// Phase 1.
func pollInterfaceTable(store *Store, client *gosnmp.GoSNMP, switchName string) error {
	descrs := map[int]string{}
	err := client.BulkWalk(oidIfDescr, func(pdu gosnmp.SnmpPDU) error {
		idx, err := lastOIDComponent(pdu.Name)
		if err != nil {
			return nil
		}
		descrs[idx] = pduString(pdu)
		return nil
	})
	if err != nil {
		return err
	}

	return client.BulkWalk(oidIfOperStatus, func(pdu gosnmp.SnmpPDU) error {
		idx, err := lastOIDComponent(pdu.Name)
		if err != nil {
			return nil
		}
		status := gosnmp.ToBigInt(pdu.Value).Int64()
		store.SetSwitchPort(switchName, idx, descrs[idx], 0, status == ifOperStatusUp)
		return nil
	})
}

// pollForwardingTable walks BRIDGE-MIB's dot1dTpFdbTable. Each returned
// OID's final 6 sub-identifiers ARE the learned MAC address (the standard
// SNMP table-indexing trick for this MIB), and the PDU value is the bridge
// port it was learned on — so a single walk gives the full MAC-to-port map
// without a second FdbAddress walk.
func pollForwardingTable(store *Store, client *gosnmp.GoSNMP, switchName string) error {
	return client.BulkWalk(oidDot1dTpFdbPort, func(pdu gosnmp.SnmpPDU) error {
		mac, err := macFromOIDSuffix(pdu.Name, oidDot1dTpFdbPort)
		if err != nil {
			return nil
		}
		port := int(gosnmp.ToBigInt(pdu.Value).Int64())
		if port <= 0 {
			return nil
		}
		store.ObserveSwitchFDB(mac, switchName, port)
		return nil
	})
}

func splitHostPort(target string) (host, port string, err error) {
	i := strings.LastIndex(target, ":")
	if i < 0 {
		return target, "161", nil
	}
	return target[:i], target[i+1:], nil
}

func lastOIDComponent(oid string) (int, error) {
	parts := strings.Split(strings.TrimPrefix(oid, "."), ".")
	if len(parts) == 0 {
		return 0, fmt.Errorf("empty oid")
	}
	return strconv.Atoi(parts[len(parts)-1])
}

// macFromOIDSuffix extracts a MAC address from the last 6 dotted-decimal
// components of an OID whose prefix is knownPrefix, e.g.
// ".1.3.6.1.2.1.17.4.3.1.2.170.187.204.221.238.1" -> "aa:bb:cc:dd:ee:01".
func macFromOIDSuffix(oid, knownPrefix string) (string, error) {
	rest := strings.TrimPrefix(strings.TrimPrefix(oid, "."), strings.TrimPrefix(knownPrefix, "."))
	parts := strings.Split(strings.Trim(rest, "."), ".")
	if len(parts) < 6 {
		return "", fmt.Errorf("oid %q too short for a MAC suffix", oid)
	}
	bytes := parts[len(parts)-6:]
	octets := make([]string, 6)
	for i, b := range bytes {
		n, err := strconv.Atoi(b)
		if err != nil || n < 0 || n > 255 {
			return "", fmt.Errorf("invalid MAC octet %q", b)
		}
		octets[i] = fmt.Sprintf("%02x", n)
	}
	return strings.Join(octets, ":"), nil
}

func pduString(pdu gosnmp.SnmpPDU) string {
	if b, ok := pdu.Value.([]byte); ok {
		return string(b)
	}
	return fmt.Sprintf("%v", pdu.Value)
}
