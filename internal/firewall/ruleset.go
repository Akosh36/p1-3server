// Package firewall builds and applies the nftables ruleset that enforces
// the platform's access matrix (CLAUDE.md §4):
//
//	User  -> forwarded LAN traffic: allowed. Traffic to this host itself
//	         (the "o'rtadagi server"): denied.
//	Admin -> forwarded LAN traffic: allowed. Traffic to this host's
//	         management ports: allowed.
//	Anyone else (no access_grants row) -> denied everywhere. This is the
//	         chains' DROP policy, not an explicit rule — the default-deny
//	         the whole platform is built around.
//
// BuildRuleset is a pure function (string in, string out) so the rule logic
// is unit-testable without root or a real nft binary; Apply is the only
// place that actually shells out to nft, and it is exercised by a real
// end-to-end test in a throwaway network namespace (see ruleset_netns_test.go).
package firewall

import (
	"fmt"
	"sort"
	"strings"
)

// DDoS baseline (decision #16 in CLAUDE.md: "aqlli standart qiymatlar" —
// smart defaults, documented here rather than left as magic numbers).
// These are meant to blunt a single misbehaving or compromised device, not
// to withstand a real distributed attack — that needs upstream scrubbing,
// which is out of scope for a single gateway box.
const (
	// icmpEchoRateLimit caps ping floods without blocking normal
	// diagnostic pings.
	icmpEchoRateLimit = "5/second"

	// adminConnRateLimit/adminConnBurst bound how many NEW connections a
	// single admin MAC may open per second through the gateway. Set high
	// enough that a busy admin session (e.g. opening many short-lived
	// connections to the management API) never trips it in normal use.
	adminConnRateLimit = "100/second"
	adminConnBurst     = "30"

	// userConnRateLimit/userConnBurst is the same idea for ordinary user
	// devices, set lower since a normal client browsing/streaming rarely
	// opens more than a few dozen new connections per second.
	userConnRateLimit = "50/second"
	userConnBurst     = "20"

	// mgmtSynRateLimit/mgmtSynBurst bounds new-connection attempts to this
	// host's own management ports per source IP — the last line of
	// defense if an admin MAC is spoofed or an admin's own machine is
	// compromised.
	mgmtSynRateLimit = "20/second"
	mgmtSynBurst     = "5"
)

// RulesetConfig is the deployment-specific knobs; DesiredState is the
// part that changes every reconciliation tick (which MACs are currently
// granted which role).
type RulesetConfig struct {
	// TableName namespaces every object this package creates, so
	// `nft flush table inet <TableName>` never touches anything else on
	// the host. Defaults to "p13server" if empty.
	TableName string

	// ManagementPorts are TCP ports on this host (the "o'rtadagi server")
	// that only admin-MAC traffic may reach — e.g. the control-plane API
	// and SSH. Loopback traffic is always allowed regardless.
	ManagementPorts []int

	// WANInterface is the interface facing the internet (Phase 3: full
	// gateway mode). When set:
	//   - LAN->internet forwarded traffic is masqueraded (NAT) behind this
	//     host's WAN address, so granted devices get real internet access
	//     through a single public/upstream IP.
	//   - New forwarded connections must arrive on a *different* interface
	//     than WANInterface — a MAC allow-list alone can't stop a spoofed
	//     source MAC arriving from the WAN side, so this closes that gap.
	//   - Management-port access is refused from WANInterface outright,
	//     even for an admin MAC: the "o'rtadagi server" must never be
	//     reachable from the internet side of the gateway.
	// Left empty, this host still enforces the LAN access matrix (Phase 2
	// behavior) but does not act as an internet gateway.
	WANInterface string
}

type DesiredState struct {
	UserMACs  []string
	AdminMACs []string
}

func (c RulesetConfig) tableName() string {
	if c.TableName == "" {
		return "p13server"
	}
	return c.TableName
}

// BuildRuleset renders a complete nftables ruleset as `nft -f`-compatible
// *commands* (add/flush/add element/add rule), not a declarative block —
// this is what makes repeated reconciliation idempotent and atomic:
//
//   - `add table` is a no-op if the table already exists (first run creates
//     it, every later run leaves it alone).
//   - `flush table` then empties every chain/set inside it, without
//     dropping the table itself.
//   - Everything after that re-adds the chains/sets/elements/rules from
//     scratch, so a device whose access was just revoked has its MAC gone
//     from every set, and rules never pile up across syncs.
//
// nft treats one `-f` invocation as a single kernel transaction, so packets
// in flight see either the old ruleset or the new one, never a half-applied
// state in between.
func BuildRuleset(cfg RulesetConfig, state DesiredState) string {
	table := cfg.tableName()
	userMACs := normalizeAndSort(state.UserMACs)
	adminMACs := normalizeAndSort(state.AdminMACs)

	var b strings.Builder
	fmt.Fprintf(&b, "add table inet %s\n", table)
	fmt.Fprintf(&b, "flush table inet %s\n\n", table)

	// `flush table` clears chains/rules but — contrary to what its name
	// suggests, verified against a real nft binary — leaves existing named
	// sets' *elements* untouched. Without the explicit `flush set` below,
	// a MAC removed from access_grants would keep matching @allowed_*_mac
	// forever: `add set` on an already-identical set is a no-op, so a
	// revoked device's stale element would never be re-added but also
	// never removed.
	fmt.Fprintf(&b, "add set inet %s allowed_user_mac { type ether_addr; }\n", table)
	fmt.Fprintf(&b, "add set inet %s allowed_admin_mac { type ether_addr; }\n", table)
	fmt.Fprintf(&b, "flush set inet %s allowed_user_mac\n", table)
	fmt.Fprintf(&b, "flush set inet %s allowed_admin_mac\n", table)
	writeElements(&b, table, "allowed_user_mac", userMACs)
	writeElements(&b, table, "allowed_admin_mac", adminMACs)

	// Rate-limit meters must be pre-declared as their own dynamic sets and
	// referenced from rules with `update @name { ... }`, NOT the inline
	// `meter name { ... }` shorthand: the shorthand implicitly creates the
	// meter the first time a rule runs, but `flush table` (used below on
	// every reconciliation) clears rules and set *elements* without
	// deleting the meter/set definitions themselves — so the next sync's
	// implicit re-creation collides with the one still sitting there
	// ("Error: File exists"). Pre-declaring with plain `add set` sidesteps
	// this because `add` on an already-identical set is a no-op.
	fmt.Fprintf(&b, "add set inet %s admin_conn_meter { type ether_addr; flags dynamic; }\n", table)
	fmt.Fprintf(&b, "add set inet %s user_conn_meter { type ether_addr; flags dynamic; }\n", table)
	fmt.Fprintf(&b, "add set inet %s mgmt_syn_meter { type ipv4_addr; flags dynamic; }\n", table)

	// notFromWAN is "" when there's no WAN interface configured (nothing to
	// guard against) or `iifname != "<wan>" ` (trailing space kept) when
	// there is — spliced directly before the MAC-match in each accept rule
	// below so a spoofed-MAC packet arriving ON the WAN interface can never
	// match, whether or not NAT/gateway mode is otherwise active.
	notFromWAN := ""
	if cfg.WANInterface != "" {
		notFromWAN = fmt.Sprintf("iifname != %q ", cfg.WANInterface)
	}

	fmt.Fprintf(&b, "\nadd chain inet %s lan_forward { type filter hook forward priority filter; policy drop; }\n", table)
	fmt.Fprintf(&b, "add rule inet %s lan_forward ct state invalid drop\n", table)
	fmt.Fprintf(&b, "add rule inet %s lan_forward ct state established,related accept\n", table)
	fmt.Fprintf(&b, "add rule inet %s lan_forward %sether saddr @allowed_admin_mac ct state new update @admin_conn_meter { ether saddr limit rate %s burst %s packets } accept\n",
		table, notFromWAN, adminConnRateLimit, adminConnBurst)
	fmt.Fprintf(&b, "add rule inet %s lan_forward %sether saddr @allowed_admin_mac accept\n", table, notFromWAN)
	fmt.Fprintf(&b, "add rule inet %s lan_forward %sether saddr @allowed_user_mac ct state new update @user_conn_meter { ether saddr limit rate %s burst %s packets } accept\n",
		table, notFromWAN, userConnRateLimit, userConnBurst)
	fmt.Fprintf(&b, "add rule inet %s lan_forward %sether saddr @allowed_user_mac accept\n", table, notFromWAN)

	fmt.Fprintf(&b, "\nadd chain inet %s management_input { type filter hook input priority filter; policy drop; }\n", table)
	fmt.Fprintf(&b, "add rule inet %s management_input iif lo accept\n", table)
	fmt.Fprintf(&b, "add rule inet %s management_input ct state invalid drop\n", table)
	fmt.Fprintf(&b, "add rule inet %s management_input ct state established,related accept\n", table)
	fmt.Fprintf(&b, "add rule inet %s management_input icmp type echo-request limit rate %s accept\n", table, icmpEchoRateLimit)
	fmt.Fprintf(&b, "add rule inet %s management_input icmpv6 type echo-request limit rate %s accept\n", table, icmpEchoRateLimit)
	if len(cfg.ManagementPorts) > 0 {
		ports := formatPortList(cfg.ManagementPorts)
		fmt.Fprintf(&b, "add rule inet %s management_input %sether saddr @allowed_admin_mac tcp dport %s ct state new update @mgmt_syn_meter { ip saddr limit rate %s burst %s packets } accept\n",
			table, notFromWAN, ports, mgmtSynRateLimit, mgmtSynBurst)
	}

	// Phase 3: full gateway mode. NAT lives in its own chain (nftables
	// requires a dedicated `type nat` chain — it can't be folded into
	// lan_forward's `type filter` chain) but the same atomic flush+rebuild
	// transaction covers it, so it can never drift out of sync with the
	// filter rules above.
	if cfg.WANInterface != "" {
		fmt.Fprintf(&b, "\nadd chain inet %s nat_postrouting { type nat hook postrouting priority srcnat; }\n", table)
		fmt.Fprintf(&b, "add rule inet %s nat_postrouting oifname %q masquerade\n", table, cfg.WANInterface)
	}

	return b.String()
}

func writeElements(b *strings.Builder, table, setName string, macs []string) {
	if len(macs) == 0 {
		return
	}
	fmt.Fprintf(b, "add element inet %s %s { %s }\n", table, setName, strings.Join(macs, ", "))
}

func formatPortList(ports []int) string {
	strs := make([]string, len(ports))
	for i, p := range ports {
		strs[i] = fmt.Sprintf("%d", p)
	}
	return "{ " + strings.Join(strs, ", ") + " }"
}

func normalizeAndSort(macs []string) []string {
	out := make([]string, 0, len(macs))
	for _, m := range macs {
		m = strings.ToLower(strings.TrimSpace(m))
		if m != "" {
			out = append(out, m)
		}
	}
	sort.Strings(out)
	return out
}
