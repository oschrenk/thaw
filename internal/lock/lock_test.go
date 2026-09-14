package lock

import "testing"

func load(t *testing.T) *Lock {
	t.Helper()
	l, err := Load("testdata/homelab.lock.json")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return l
}

// The fixture holds three distinct nixpkgs nodes. root.inputs.nixpkgs points
// at "nixpkgs_2", so a lookup by node name returns the wrong revision.
func TestAliasedNodeName(t *testing.T) {
	l := load(t)

	key, err := l.Resolve("nixpkgs")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if key != "nixpkgs_2" {
		t.Errorf("root nixpkgs resolves to %q, want %q", key, "nixpkgs_2")
	}

	rev, err := l.Rev("nixpkgs")
	if err != nil {
		t.Fatalf("Rev: %v", err)
	}
	const want = "aff8a0b28396750446e5537a96461bc4facdb287"
	if rev != want {
		t.Errorf("root nixpkgs rev = %q, want %q", rev, want)
	}

	naive := l.Nodes["nixpkgs"].Locked.Rev
	if naive == rev {
		t.Fatal("fixture no longer exercises aliasing: node \"nixpkgs\" and root nixpkgs agree")
	}
}

// The pis build from the nixpkgs nixos-raspberrypi pins, not the root one.
func TestTransitiveInput(t *testing.T) {
	l := load(t)

	key, err := l.Resolve("nixos-raspberrypi", "nixpkgs")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if key != "nixpkgs" {
		t.Errorf("nixos-raspberrypi nixpkgs resolves to %q, want %q", key, "nixpkgs")
	}

	root, err := l.Resolve("nixpkgs")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if key == root {
		t.Error("the pi nixpkgs and the root nixpkgs resolve to one node")
	}
}

// disko.inputs.nixpkgs is ["nixpkgs"], a path followed from the root.
func TestFollowsResolvesToRoot(t *testing.T) {
	l := load(t)

	key, err := l.Resolve("disko", "nixpkgs")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	root, err := l.Resolve("nixpkgs")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if key != root {
		t.Errorf("disko nixpkgs resolves to %q, want the root nixpkgs %q", key, root)
	}
}

// nixos-images.nixos-stable follows ["nixos-raspberrypi","nixpkgs"], a
// multi-segment path, so it lands on the pi nixpkgs rather than the root one.
func TestFollowsMultiSegment(t *testing.T) {
	l := load(t)

	key, err := l.Resolve("nixos-raspberrypi", "nixos-images", "nixos-stable")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	pi, err := l.Resolve("nixos-raspberrypi", "nixpkgs")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if key != pi {
		t.Errorf("nixos-stable resolves to %q, want the pi nixpkgs %q", key, pi)
	}
}

func TestRootInputs(t *testing.T) {
	l := load(t)

	got, err := l.RootInputs()
	if err != nil {
		t.Fatalf("RootInputs: %v", err)
	}
	want := map[string]string{
		"disko":             "disko",
		"flake-utils":       "flake-utils",
		"nixos-raspberrypi": "nixos-raspberrypi",
		"nixpkgs":           "nixpkgs_2",
		"opnix":             "opnix",
	}
	if len(got) != len(want) {
		t.Fatalf("RootInputs returned %d entries, want %d: %v", len(got), len(want), got)
	}
	for name, key := range want {
		if got[name] != key {
			t.Errorf("RootInputs[%q] = %q, want %q", name, got[name], key)
		}
	}
}

func TestUnknownInput(t *testing.T) {
	l := load(t)

	if _, err := l.Resolve("no-such-input"); err == nil {
		t.Error("Resolve of an unknown input returned no error")
	}
}
