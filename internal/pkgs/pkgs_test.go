package pkgs

import (
	"os"
	"testing"
)

func TestNameVersion(t *testing.T) {
	cases := []struct{ in, name, version string }{
		{"beszel-0.18.7", "beszel", "0.18.7"},
		{"openssh-10.4p1", "openssh", "10.4p1"},
		{"node_exporter-1.11.1", "node_exporter", "1.11.1"},
		{"prometheus-json-exporter-0.7.0", "prometheus-json-exporter", "0.7.0"},
		{"coreutils-9.11", "coreutils", "9.11"},
		{"cpupower-6.18.34-unstable_20260604", "cpupower", "6.18.34-unstable_20260604"},
		{"unit-script-nix-gc-start", "unit-script-nix-gc-start", ""},
	}
	for _, c := range cases {
		n, v := nameVersion(c.in)
		if n != c.name || v != c.version {
			t.Errorf("nameVersion(%q) = (%q, %q), want (%q, %q)", c.in, n, v, c.name, c.version)
		}
	}
}

// A unit is rarely named after its option. beszel-agent is configured through
// services.beszel, and restic-backups-prometheus through services.restic.
func TestCandidates(t *testing.T) {
	services := []string{"beszel", "restic", "prometheus", "kula", "openssh", "res"}
	units := []string{"beszel-agent", "restic-backups-prometheus", "prometheus", "kula", "sshd"}
	got := candidates(units, services)

	want := map[string]string{
		"beszel-agent":              "beszel",
		"restic-backups-prometheus": "restic",
		"prometheus":                "prometheus",
		"kula":                      "kula",
	}
	for u, svc := range want {
		if got[u] != svc {
			t.Errorf("candidates[%q] = %q, want %q", u, got[u], svc)
		}
	}
	if _, ok := got["sshd"]; ok {
		t.Error("sshd matched a service; it is configured through services.openssh and has no name in common")
	}
}

// The longest prefix wins, so "res" never steals a unit from "restic".
func TestCandidatesPrefersLongestPrefix(t *testing.T) {
	got := candidates([]string{"restic-backups-x"}, []string{"res", "restic"})
	if got["restic-backups-x"] != "restic" {
		t.Errorf("got %q, want restic", got["restic-backups-x"])
	}
}

// A dirty tree gets a new store hash on every evaluation, so provenance has to
// rest on the path inside the snapshot rather than the hash.
func TestMineIgnoresStoreHash(t *testing.T) {
	dir := t.TempDir()
	if err := writeFile(dir + "/flake.nix"); err != nil {
		t.Fatal(err)
	}
	if mine(dir, "/nix/store/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-source/flake.nix") != true {
		t.Error("a file present in the flake was not recognised")
	}
	if mine(dir, "/nix/store/bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb-source/nixos/modules/services/x.nix") {
		t.Error("a nixpkgs module was claimed by the flake")
	}
	if mine(dir, "/nix/store/cccccccccccccccccccccccccccccccc-source/absent.nix") {
		t.Error("an absent file was claimed by the flake")
	}
}

func writeFile(p string) error {
	f, err := os.Create(p)
	if err != nil {
		return err
	}
	return f.Close()
}
