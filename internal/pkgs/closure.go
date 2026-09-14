package pkgs

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	"github.com/oschrenk/thaw/internal/flake"
	"github.com/oschrenk/thaw/internal/subject"
)

type drv struct {
	Env map[string]string `json:"env"`
}

type derivations struct {
	Derivations map[string]drv `json:"derivations"`
}

// Closure reads every derivation a subject builds from, with the version each
// carries. This is the build closure, so it holds the tools a package needs to
// compile as well as what ends up running. Nix finds the runtime closure by
// scanning built output, so no evaluation separates the two.
func Closure(ctx context.Context, r *flake.Runner, dir string, sub subject.Subject, over map[string]string) (*Set, error) {
	out, err := r.DerivationShow(ctx, dir, sub.Toplevel(), over)
	if err != nil {
		return nil, err
	}
	var d derivations
	if err := json.Unmarshal(out, &d); err != nil {
		return nil, err
	}

	set := &Set{}
	seen := map[string]bool{}
	for _, v := range d.Derivations {
		name, version := v.Env["pname"], v.Env["version"]
		if name == "" || version == "" || seen[name] {
			continue
		}
		if strings.HasPrefix(name, "unit-script") {
			continue
		}
		seen[name] = true
		set.Pkgs = append(set.Pkgs, Pkg{Name: name, Version: version})
	}
	sort.Slice(set.Pkgs, func(i, j int) bool { return set.Pkgs[i].Name < set.Pkgs[j].Name })
	return set, nil
}
