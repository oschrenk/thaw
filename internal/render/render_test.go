package render

import (
	"bytes"
	"regexp"
	"strings"
	"testing"

	"github.com/oschrenk/thaw/internal/diff"
	"github.com/oschrenk/thaw/internal/revs"
	"github.com/oschrenk/thaw/internal/subject"
)

func sample() *revs.Report {
	return &revs.Report{Inputs: []revs.Input{
		{Name: "disko", Locked: "ff8702b1111111", Upstream: "ff8702b1111111", Follows: "follows nixpkgs"},
		{Name: "nixos-raspberrypi", Locked: "7e395081111111", Upstream: "8e7f1a41111111",
			Pins: []revs.Pin{{Name: "nixpkgs", Locked: "597283a1111111", Upstream: "21a67dc1111111"}}},
		{Name: "nixpkgs", Locked: "aff8a0b1111111", Upstream: "02f56961111111"},
	}}
}

// A pipe or a redirect must grep and diff cleanly, so nothing outside
// printable ASCII may reach it.
func TestPlainIsASCII(t *testing.T) {
	var buf bytes.Buffer
	if err := Revs(&buf, sample(), Style{Fancy: false}); err != nil {
		t.Fatalf("Revs: %v", err)
	}
	for i, b := range buf.Bytes() {
		if b == '\n' {
			continue
		}
		if b < 0x20 || b > 0x7e {
			t.Fatalf("byte %d is 0x%02x, outside printable ASCII: %q", i, b, buf.String())
		}
	}
	if strings.Contains(buf.String(), "→") || strings.Contains(buf.String(), "─") {
		t.Error("plain output holds a glyph")
	}
}

// grep '^M' is the documented way to find what moved, so the marker has to
// anchor at column 0 and only moved rows may carry it.
func TestPlainMarkerAnchors(t *testing.T) {
	var buf bytes.Buffer
	if err := Revs(&buf, sample(), Style{Fancy: false}); err != nil {
		t.Fatalf("Revs: %v", err)
	}
	var moved []string
	for _, line := range strings.Split(buf.String(), "\n") {
		if strings.HasPrefix(line, "M") {
			moved = append(moved, strings.Fields(line)[1])
		}
	}
	want := []string{"nixos-raspberrypi", "nixpkgs"}
	if len(moved) != len(want) {
		t.Fatalf("grep '^M' found %v, want %v", moved, want)
	}
	for i := range want {
		if moved[i] != want[i] {
			t.Errorf("moved[%d] = %q, want %q", i, moved[i], want[i])
		}
	}
}

func TestFancyUsesGlyphs(t *testing.T) {
	var buf bytes.Buffer
	if err := Revs(&buf, sample(), Style{Fancy: true}); err != nil {
		t.Fatalf("Revs: %v", err)
	}
	for _, want := range []string{"↑", "·", "─", "→", "└"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("fancy output lacks %q", want)
		}
	}
}

// The follows note must sit in one column whether the row's revision is
// painted green or not, because escape bytes carry no width on screen.
func TestFancyFollowsAligns(t *testing.T) {
	r := &revs.Report{Inputs: []revs.Input{
		{Name: "home-manager", Locked: "17bf3751111111", Upstream: "cda90fd1111111", Follows: "follows nixpkgs"},
		{Name: "nix-darwin", Locked: "4cff07d1111111", Upstream: "4cff07d1111111", Follows: "follows nixpkgs"},
	}}
	var buf bytes.Buffer
	if err := Revs(&buf, r, Style{Fancy: true}); err != nil {
		t.Fatalf("Revs: %v", err)
	}
	ansi := regexp.MustCompile(`\x1b\[[0-9;]*m`)
	var cols []int
	for _, line := range strings.Split(ansi.ReplaceAllString(buf.String(), ""), "\n") {
		if i := strings.Index(line, "follows"); i >= 0 {
			// Rune count, not byte offset: the ↑ and · markers differ in
			// UTF-8 width but each fills one screen column.
			cols = append(cols, len([]rune(line[:i])))
		}
	}
	if len(cols) != 2 {
		t.Fatalf("want 2 follows notes, got %d: %q", len(cols), buf.String())
	}
	if cols[0] != cols[1] {
		t.Errorf("follows sits at column %d on the moved row and %d on the unmoved one", cols[0], cols[1])
	}
}

// The subjects table names every target with its kind and attribute, in
// plain ASCII when piped.
func TestSubjectsTable(t *testing.T) {
	set := subject.Build("aarch64-darwin",
		[]string{"pi-2"}, []string{"Olivers-MaxBook"}, []string{"default"}, nil)
	var buf bytes.Buffer
	if err := Subjects(&buf, set, Style{Fancy: false}); err != nil {
		t.Fatalf("Subjects: %v", err)
	}
	out := buf.String()
	for _, want := range []string{
		"SUBJECT",
		"pi-2",
		"host",
		"nixosConfigurations.pi-2",
		"Olivers-MaxBook",
		"darwin",
		"devShells.aarch64-darwin.default",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("table lacks %q: %q", want, out)
		}
	}
	for _, b := range buf.Bytes() {
		if b != '\n' && (b < 0x20 || b > 0x7e) {
			t.Fatalf("plain table holds byte 0x%02x: %q", b, out)
		}
	}
}

func packagesSample() *diff.Report {
	return &diff.Report{Groups: []diff.Group{{
		Input: "nixpkgs", Locked: "02f56961111111", Upstream: "c7def041111111", Moved: true,
		Rows: []diff.Row{
			{Name: "go", From: "1.26.6", To: "1.26.7", Kind: diff.Changed, Subjects: []string{"devshell"}},
			{Name: "gopls", From: "0.23.0", To: "0.23.0", Kind: diff.Unchanged, Subjects: []string{"devshell"}},
		},
	}}}
}

// The default report hides unchanged rows behind a per-group count, so one
// real change does not drown in equal versions. The counts stay complete.
func TestPackagesHidesUnchangedByDefault(t *testing.T) {
	var buf bytes.Buffer
	if err := Packages(&buf, packagesSample(), Style{Fancy: false}); err != nil {
		t.Fatalf("Packages: %v", err)
	}
	out := buf.String()
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "=") {
			t.Errorf("default report prints an unchanged row: %q", line)
		}
	}
	if !strings.Contains(out, "1 unchanged\n") {
		t.Errorf("group lacks the hidden-row count: %q", out)
	}
	if !strings.Contains(out, "1 changed  0 added  0 removed  1 unchanged") {
		t.Errorf("summary lacks the unchanged count: %q", out)
	}
}

// --with-unchanged restores the unchanged rows, with the same version on both
// sides. The "=" mark keeps them out of grep '^M'.
func TestPackagesWithUnchangedShowsRows(t *testing.T) {
	var buf bytes.Buffer
	if err := Packages(&buf, packagesSample(), Style{Fancy: false, WithUnchanged: true}); err != nil {
		t.Fatalf("Packages: %v", err)
	}
	out := buf.String()
	var row string
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "=") {
			row = line
		}
	}
	if row == "" {
		t.Fatalf("no unchanged row in the verbose report: %q", out)
	}
	for _, want := range []string{"gopls", "0.23.0", "-> 0.23.0"} {
		if !strings.Contains(row, want) {
			t.Errorf("unchanged row lacks %q: %q", want, row)
		}
	}
}

// A pin is reported one level under the input that carries it.
func TestPinIndented(t *testing.T) {
	var buf bytes.Buffer
	if err := Revs(&buf, sample(), Style{Fancy: false}); err != nil {
		t.Fatalf("Revs: %v", err)
	}
	for _, line := range strings.Split(buf.String(), "\n") {
		if strings.Contains(line, "pins nixpkgs") {
			if !strings.HasPrefix(line, "    ") {
				t.Errorf("pin line is not indented: %q", line)
			}
			if !strings.Contains(line, "597283a -> 21a67dc") {
				t.Errorf("pin line lacks the revisions: %q", line)
			}
			return
		}
	}
	t.Error("no pin line in the report")
}
