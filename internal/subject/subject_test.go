package subject

import (
	"strings"
	"testing"
)

func homelab() Set {
	return Build("aarch64-darwin",
		[]string{"hetzner-1", "pi", "pi-1", "pi-2", "pi-3"},
		nil,
		[]string{"default"},
		[]string{"linux-builder"})
}

func TestNamespaceIsFlat(t *testing.T) {
	set := homelab()
	want := []string{"hetzner-1", "pi", "pi-1", "pi-2", "pi-3", "devshell", "linux-builder"}
	got := set.Names()
	if len(got) != len(want) {
		t.Fatalf("names = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("names[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// A host is evaluated through its system closure, everything else directly.
func TestToplevel(t *testing.T) {
	set := homelab()
	cases := map[string]string{
		"pi-2":          "nixosConfigurations.pi-2.config.system.build.toplevel",
		"devshell":      "devShells.aarch64-darwin.default",
		"linux-builder": "packages.aarch64-darwin.linux-builder",
	}
	for name, want := range cases {
		sub, err := set.Lookup(name)
		if err != nil {
			t.Fatalf("Lookup(%q): %v", name, err)
		}
		if got := sub.Toplevel(); got != want {
			t.Errorf("%s toplevel = %q, want %q", name, got, want)
		}
	}
}

// The default report covers the hosts and the devShell. packages answer to
// --for, because pi-image derives from nixosConfigurations.pi and
// linux-builder is a nixpkgs passthrough.
func TestDefaultExcludesPackages(t *testing.T) {
	set := homelab()
	for _, sub := range set.Default() {
		if sub.Kind == Package {
			t.Errorf("default report holds package %q", sub.Name)
		}
	}
	if len(set.Default()) != 6 {
		t.Errorf("default holds %d subjects, want 6", len(set.Default()))
	}
	if _, err := set.Lookup("linux-builder"); err != nil {
		t.Errorf("linux-builder is unreachable through --for: %v", err)
	}
}

// A darwin host evaluates through the same system closure attribute a NixOS
// host does, and the default report covers it.
func TestDarwinHost(t *testing.T) {
	set := Build("aarch64-darwin", nil, []string{"Olivers-MaxBook"}, []string{"default"}, nil)
	sub, err := set.Lookup("Olivers-MaxBook")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	want := "darwinConfigurations.Olivers-MaxBook.config.system.build.toplevel"
	if got := sub.Toplevel(); got != want {
		t.Errorf("toplevel = %q, want %q", got, want)
	}
	if len(set.Default()) != 2 {
		t.Errorf("default holds %d subjects, want the darwin host and the devshell", len(set.Default()))
	}
}

func TestLookupUnknown(t *testing.T) {
	set := homelab()
	_, err := set.Lookup("pi-9")
	if err == nil {
		t.Fatal("Lookup of an absent subject returned no error")
	}
	if !strings.Contains(err.Error(), "pi-2") {
		t.Errorf("error names no alternatives: %v", err)
	}
}

// --for takes one word, so two kinds sharing a name has no answer but an error.
func TestLookupAmbiguous(t *testing.T) {
	set := Build("aarch64-darwin", []string{"pi-image"}, nil, nil, []string{"pi-image"})
	_, err := set.Lookup("pi-image")
	if err == nil {
		t.Fatal("an ambiguous name returned no error")
	}
	if !strings.Contains(err.Error(), "more than one") {
		t.Errorf("error does not name the ambiguity: %v", err)
	}
}

func TestPackagesDefaultIsSkipped(t *testing.T) {
	set := Build("aarch64-darwin", nil, nil, nil, []string{"default", "pi-image"})
	if _, err := set.Lookup("default"); err == nil {
		t.Error("packages.default entered the namespace; it is an alias")
	}
	if _, err := set.Lookup("pi-image"); err != nil {
		t.Errorf("pi-image is missing: %v", err)
	}
}
