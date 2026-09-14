// Package subject enumerates the flake outputs thaw can evaluate.
//
// The namespace is flat and built from the flake on every run, so an output
// added to flake.nix answers to --for with no edit here.
package subject

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/oschrenk/thaw/internal/flake"
)

type Kind int

const (
	Host Kind = iota
	DarwinHost
	DevShell
	Package
)

func (k Kind) MarshalJSON() ([]byte, error) {
	return []byte(`"` + k.String() + `"`), nil
}

func (k Kind) String() string {
	switch k {
	case Host:
		return "host"
	case DarwinHost:
		return "darwin"
	case DevShell:
		return "devshell"
	case Package:
		return "package"
	}
	return "unknown"
}

type Subject struct {
	Name string
	Kind Kind
	Attr string
}

// Toplevel is the attribute that realises a subject, which is what a package
// diff compares. nix-darwin exposes the same system closure attribute NixOS
// does.
func (s Subject) Toplevel() string {
	if s.Kind == Host || s.Kind == DarwinHost {
		return s.Attr + ".config.system.build.toplevel"
	}
	return s.Attr
}

type Set struct {
	Subjects []Subject
	System   string
}

// Default is the subject set a bare run reports: every nixosConfigurations
// entry and the devShell. The packages outputs answer to --for instead.
// pi-image derives from nixosConfigurations.pi, which the default covers, and
// linux-builder is a nixpkgs passthrough.
func (s Set) Default() []Subject {
	var out []Subject
	for _, sub := range s.Subjects {
		if sub.Kind == Host || sub.Kind == DarwinHost || sub.Kind == DevShell {
			out = append(out, sub)
		}
	}
	return out
}

func (s Set) Names() []string {
	out := make([]string, 0, len(s.Subjects))
	for _, sub := range s.Subjects {
		out = append(out, sub.Name)
	}
	return out
}

// Lookup finds a subject by name. Two kinds may not share a name, because
// --for would then be ambiguous.
func (s Set) Lookup(name string) (Subject, error) {
	var hits []Subject
	for _, sub := range s.Subjects {
		if sub.Name == name {
			hits = append(hits, sub)
		}
	}
	switch len(hits) {
	case 1:
		return hits[0], nil
	case 0:
		return Subject{}, fmt.Errorf("no subject named %q; the flake offers %s",
			name, strings.Join(s.Names(), ", "))
	default:
		kinds := make([]string, 0, len(hits))
		for _, h := range hits {
			kinds = append(kinds, h.Kind.String()+" "+h.Attr)
		}
		return Subject{}, fmt.Errorf("%q names more than one subject: %s",
			name, strings.Join(kinds, ", "))
	}
}

// Build assembles a Set from the raw attribute names of each output kind.
func Build(system string, hosts, darwinHosts, shells, packages []string) Set {
	set := Set{System: system}
	for _, h := range hosts {
		set.Subjects = append(set.Subjects, Subject{
			Name: h, Kind: Host, Attr: "nixosConfigurations." + h,
		})
	}
	for _, h := range darwinHosts {
		set.Subjects = append(set.Subjects, Subject{
			Name: h, Kind: DarwinHost, Attr: "darwinConfigurations." + h,
		})
	}
	for _, sh := range shells {
		name := "devshell"
		if sh != "default" {
			name = "devshell-" + sh
		}
		set.Subjects = append(set.Subjects, Subject{
			Name: name, Kind: DevShell,
			Attr: fmt.Sprintf("devShells.%s.%s", system, sh),
		})
	}
	for _, p := range packages {
		if p == "default" {
			continue
		}
		set.Subjects = append(set.Subjects, Subject{
			Name: p, Kind: Package,
			Attr: fmt.Sprintf("packages.%s.%s", system, p),
		})
	}
	sort.Slice(set.Subjects, func(i, j int) bool {
		if set.Subjects[i].Kind != set.Subjects[j].Kind {
			return set.Subjects[i].Kind < set.Subjects[j].Kind
		}
		return set.Subjects[i].Name < set.Subjects[j].Name
	})
	return set
}

// Enumerate reads the four output kinds from the flake. An absent kind is not
// an error: a flake with no nixosConfigurations still has a devShell.
func Enumerate(ctx context.Context, r *flake.Runner, dir string) (Set, error) {
	system, err := r.CurrentSystem(ctx)
	if err != nil {
		return Set{}, err
	}

	var hosts, darwinHosts, shells, packages []string
	attrs := []struct {
		attr string
		out  *[]string
	}{
		{"nixosConfigurations", &hosts},
		{"darwinConfigurations", &darwinHosts},
		{"devShells." + system, &shells},
		{"packages." + system, &packages},
	}

	var wg sync.WaitGroup
	for _, a := range attrs {
		wg.Add(1)
		go func(attr string, out *[]string) {
			defer wg.Done()
			var names []string
			if err := r.Eval(ctx, dir, attr, "builtins.attrNames", &names); err != nil {
				return
			}
			*out = names
		}(a.attr, a.out)
	}
	wg.Wait()

	set := Build(system, hosts, darwinHosts, shells, packages)
	if len(set.Subjects) == 0 {
		return set, fmt.Errorf("the flake offers no nixosConfigurations, darwinConfigurations, devShells or packages for %s", system)
	}
	return set, nil
}
