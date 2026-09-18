package httpapi

import "testing"

func TestValidateAllowedIP(t *testing.T) {
	valid := []string{"192.168.1.5", "10.0.0.0/24", "::1", "2001:db8::/32"}
	for _, v := range valid {
		if err := validateAllowedIP(v); err != nil {
			t.Errorf("validateAllowedIP(%q) = %v, want nil", v, err)
		}
	}

	invalid := []string{"", "not-an-ip", "999.999.999.999", "192.168.1.5/99"}
	for _, v := range invalid {
		if err := validateAllowedIP(v); err == nil {
			t.Errorf("validateAllowedIP(%q) = nil, want an error", v)
		}
	}
}

func TestValidateAllowedMAC(t *testing.T) {
	valid := []string{"aa:bb:cc:dd:ee:ff", "AA:BB:CC:DD:EE:FF", "aa-bb-cc-dd-ee-ff"}
	for _, v := range valid {
		if err := validateAllowedMAC(v); err != nil {
			t.Errorf("validateAllowedMAC(%q) = %v, want nil", v, err)
		}
	}

	invalid := []string{"", "not-a-mac", "aa:bb:cc:dd:ee", "192.168.1.5"}
	for _, v := range invalid {
		if err := validateAllowedMAC(v); err == nil {
			t.Errorf("validateAllowedMAC(%q) = nil, want an error", v)
		}
	}
}
