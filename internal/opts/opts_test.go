package opts

import "testing"

// A services path keeps two segments so services.prometheus stays distinct
// from services.perses. Anything else keeps one.
func TestPrefixes(t *testing.T) {
	got := Prefixes([]string{
		"services.prometheus.retentionTime",
		"services.prometheus.port",
		"services.openssh.enable",
		"networking.hostName",
		"networking.firewall.allowedTCPPorts",
		"boot.loader.grub.enable",
	})
	want := []string{"boot", "networking", "services.openssh", "services.prometheus"}
	if len(got) != len(want) {
		t.Fatalf("Prefixes = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Prefixes[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestPrefixesDedupes(t *testing.T) {
	got := Prefixes([]string{"services.x.a", "services.x.b", "services.x.c"})
	if len(got) != 1 || got[0] != "services.x" {
		t.Errorf("Prefixes = %v, want [services.x]", got)
	}
}
