package netdisc

import "testing"

func TestMacFromOIDSuffix(t *testing.T) {
	cases := []struct {
		oid     string
		want    string
		wantErr bool
	}{
		{".1.3.6.1.2.1.17.4.3.1.2.170.187.204.221.238.1", "aa:bb:cc:dd:ee:01", false},
		{".1.3.6.1.2.1.17.4.3.1.2.0.0.0.0.0.0", "00:00:00:00:00:00", false},
		{".1.3.6.1.2.1.17.4.3.1.2.1.2", "", true}, // too short
	}
	for _, c := range cases {
		got, err := macFromOIDSuffix(c.oid, oidDot1dTpFdbPort)
		if c.wantErr {
			if err == nil {
				t.Errorf("macFromOIDSuffix(%q) expected error, got %q", c.oid, got)
			}
			continue
		}
		if err != nil {
			t.Fatalf("macFromOIDSuffix(%q) unexpected error: %v", c.oid, err)
		}
		if got != c.want {
			t.Errorf("macFromOIDSuffix(%q) = %q, want %q", c.oid, got, c.want)
		}
	}
}

func TestLastOIDComponent(t *testing.T) {
	idx, err := lastOIDComponent(".1.3.6.1.2.1.2.2.1.8.4")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if idx != 4 {
		t.Errorf("lastOIDComponent = %d, want 4", idx)
	}
}
