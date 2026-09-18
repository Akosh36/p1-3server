package firewall

import (
	"strings"
	"testing"
)

func TestBuildRuleset_ContainsExpectedElements(t *testing.T) {
	ruleset := BuildRuleset(
		RulesetConfig{TableName: "p13test", ManagementPorts: []int{8080, 22}},
		DesiredState{
			UserMACs:  []string{"AA:BB:CC:DD:EE:01", "aa:bb:cc:dd:ee:02"},
			AdminMACs: []string{"11:22:33:44:55:66"},
		},
	)

	mustContain(t, ruleset, "add table inet p13test")
	mustContain(t, ruleset, "flush table inet p13test")
	mustContain(t, ruleset, "flush set inet p13test allowed_user_mac")
	mustContain(t, ruleset, "flush set inet p13test allowed_admin_mac")
	mustContain(t, ruleset, "add element inet p13test allowed_user_mac { aa:bb:cc:dd:ee:01, aa:bb:cc:dd:ee:02 }")
	mustContain(t, ruleset, "add element inet p13test allowed_admin_mac { 11:22:33:44:55:66 }")
	mustContain(t, ruleset, "type filter hook forward priority filter; policy drop;")
	mustContain(t, ruleset, "type filter hook input priority filter; policy drop;")
	mustContain(t, ruleset, "tcp dport { 8080, 22 }")

	// `flush table` alone does NOT clear existing named sets' elements
	// (verified against a real nft binary — see the comment in
	// BuildRuleset) so each set needs its own explicit `flush set` before
	// `add element`, or a revoked device's MAC would never be removed.
	flushTableIdx := strings.Index(ruleset, "flush table")
	flushSetIdx := strings.Index(ruleset, "flush set inet p13test allowed_user_mac")
	elementIdx := strings.Index(ruleset, "add element")
	if flushTableIdx < 0 || flushSetIdx < 0 || elementIdx < 0 || flushTableIdx > flushSetIdx || flushSetIdx > elementIdx {
		t.Fatalf("expected 'flush table' before 'flush set' before 'add element', got:\n%s", ruleset)
	}
}

func TestBuildRuleset_RevocationFlushesSetEvenWithNoRemainingElements(t *testing.T) {
	// A device that had a role and now has none must produce a ruleset
	// that still flushes the sets — this is the exact bug this test
	// guards against: BuildRuleset used to only emit "add element" when a
	// list was non-empty, with nothing to clear a now-stale element for a
	// revoked MAC.
	ruleset := BuildRuleset(RulesetConfig{TableName: "p13test"}, DesiredState{})
	mustContain(t, ruleset, "flush set inet p13test allowed_user_mac")
	mustContain(t, ruleset, "flush set inet p13test allowed_admin_mac")
}

func TestBuildRuleset_EmptyStateStillDeniesByDefault(t *testing.T) {
	ruleset := BuildRuleset(RulesetConfig{}, DesiredState{})

	mustContain(t, ruleset, "policy drop")
	mustNotContain(t, ruleset, "add element")
	// No devices granted yet: the sets exist (so the chain's @set
	// references resolve) but are empty, so every rule referencing them
	// simply never matches, leaving `policy drop` as the only outcome.
}

func TestBuildRuleset_NoManagementPortsOmitsAdminInputRule(t *testing.T) {
	ruleset := BuildRuleset(RulesetConfig{}, DesiredState{AdminMACs: []string{"aa:bb:cc:dd:ee:01"}})
	mustNotContain(t, ruleset, "tcp dport")
}

func TestBuildRuleset_NormalizesCaseAndSortsForStableDiffs(t *testing.T) {
	a := BuildRuleset(RulesetConfig{}, DesiredState{UserMACs: []string{"BB:BB:BB:BB:BB:BB", "aa:aa:aa:aa:aa:aa"}})
	b := BuildRuleset(RulesetConfig{}, DesiredState{UserMACs: []string{"aa:aa:aa:aa:aa:aa", "bb:bb:bb:bb:bb:bb"}})
	if a != b {
		t.Fatalf("expected identical output regardless of input order/case:\na=%s\nb=%s", a, b)
	}
}

func TestBuildRuleset_NoWANInterfaceMeansNoGatewayMode(t *testing.T) {
	ruleset := BuildRuleset(RulesetConfig{}, DesiredState{AdminMACs: []string{"aa:bb:cc:dd:ee:01"}})
	mustNotContain(t, ruleset, "nat_postrouting")
	mustNotContain(t, ruleset, "masquerade")
	mustNotContain(t, ruleset, "iifname")
}

func TestBuildRuleset_WANInterfaceAddsNATAndHardening(t *testing.T) {
	ruleset := BuildRuleset(
		RulesetConfig{TableName: "p13test", ManagementPorts: []int{8080}, WANInterface: "eth0"},
		DesiredState{UserMACs: []string{"aa:bb:cc:dd:ee:01"}, AdminMACs: []string{"11:22:33:44:55:66"}},
	)

	mustContain(t, ruleset, `add chain inet p13test nat_postrouting { type nat hook postrouting priority srcnat; }`)
	mustContain(t, ruleset, `add rule inet p13test nat_postrouting oifname "eth0" masquerade`)

	// Every MAC-allow rule in both chains must reject the WAN interface as
	// the arriving interface, or a spoofed MAC coming from the internet
	// side would still match.
	mustContain(t, ruleset, `lan_forward iifname != "eth0" ether saddr @allowed_admin_mac`)
	mustContain(t, ruleset, `lan_forward iifname != "eth0" ether saddr @allowed_user_mac`)
	mustContain(t, ruleset, `management_input iifname != "eth0" ether saddr @allowed_admin_mac`)

	// The NAT chain must be part of the same flush+rebuild transaction as
	// everything else, or it could silently fall out of sync on a partial
	// apply.
	flushIdx := strings.Index(ruleset, "flush table")
	natIdx := strings.Index(ruleset, "nat_postrouting")
	if flushIdx < 0 || natIdx < 0 || flushIdx > natIdx {
		t.Fatalf("expected nat_postrouting to be built after the table flush, got:\n%s", ruleset)
	}
}

func mustContain(t *testing.T, haystack, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Errorf("expected ruleset to contain %q, got:\n%s", needle, haystack)
	}
}

func mustNotContain(t *testing.T, haystack, needle string) {
	t.Helper()
	if strings.Contains(haystack, needle) {
		t.Errorf("expected ruleset to NOT contain %q, got:\n%s", needle, haystack)
	}
}
