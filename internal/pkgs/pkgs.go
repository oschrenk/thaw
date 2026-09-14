// Package pkgs collects the packages a subject runs, and decides which of them
// this flake asks for.
package pkgs

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/oschrenk/thaw/internal/flake"
	"github.com/oschrenk/thaw/internal/subject"
)

type Pkg struct {
	Name    string
	Version string
	Unit    string
	Service string
}

type Set struct {
	Pkgs    []Pkg
	Skipped []string
}

var storePkg = regexp.MustCompile(`/nix/store/[a-z0-9]{32}-([A-Za-z0-9._+-]+)`)

// nameVersion splits a store path basename into a package name and version.
// firefox-121.0 splits at the last dash before a digit; coreutils-9.11-bin
// keeps the output suffix out of the version.
func nameVersion(base string) (string, string) {
	parts := strings.Split(base, "-")
	for i := 1; i < len(parts); i++ {
		if parts[i] == "" {
			continue
		}
		if c := parts[i][0]; c >= '0' && c <= '9' {
			return strings.Join(parts[:i], "-"), strings.Join(parts[i:], "-")
		}
	}
	return base, ""
}

// mine reports whether a defining file sits inside the flake. Store paths of a
// dirty tree change between evaluations, so the store hash proves nothing; the
// path relative to the source snapshot is what carries.
func mine(dir, file string) bool {
	_, rel, ok := strings.Cut(file, "-source/")
	if !ok {
		return false
	}
	if strings.HasPrefix(rel, "nixos/modules/") || strings.HasPrefix(rel, "pkgs/") {
		return false
	}
	st, err := os.Stat(filepath.Join(dir, rel))
	return err == nil && !st.IsDir()
}

// candidates pairs each systemd unit with the services.<name> subtree most
// likely to configure it: an exact name, or a prefix at a dash boundary, so
// beszel-agent finds beszel and restic-backups-prometheus finds restic.
func candidates(units []string, services []string) map[string]string {
	known := map[string]bool{}
	for _, s := range services {
		known[s] = true
	}
	out := map[string]string{}
	for _, u := range units {
		if known[u] {
			out[u] = u
			continue
		}
		best := ""
		for s := range known {
			if strings.HasPrefix(u, s+"-") && len(s) > len(best) {
				best = s
			}
		}
		if best != "" {
			out[u] = best
		}
	}
	return out
}

// Collect returns the packages a subject runs that this flake asks for.
func Collect(ctx context.Context, r *flake.Runner, dir string, sub subject.Subject) (*Set, error) {
	return CollectWith(ctx, r, dir, sub, nil)
}

// CollectWith collects under overridden inputs, which is how a candidate
// revision is previewed without writing a lock.
func CollectWith(ctx context.Context, r *flake.Runner, dir string, sub subject.Subject, over map[string]string) (*Set, error) {
	if sub.Kind == subject.DarwinHost {
		return collectDarwin(ctx, r, dir, sub, over)
	}
	if sub.Kind != subject.Host {
		return collectShell(ctx, r, dir, sub, over)
	}

	var units map[string]string
	if err := r.EvalWith(ctx, dir, sub.Attr+".config.systemd.services", unitsExpr, over, &units); err != nil {
		return nil, err
	}

	var services []string
	if err := r.EvalWith(ctx, dir, sub.Attr+".options.services", "builtins.attrNames", over, &services); err != nil {
		return nil, err
	}

	unitNames := make([]string, 0, len(units))
	for u, exec := range units {
		if exec != "" {
			unitNames = append(unitNames, u)
		}
	}
	sort.Strings(unitNames)

	pairs := candidates(unitNames, services)
	wanted := map[string]bool{}
	for _, svc := range pairs {
		wanted[svc] = true
	}

	// One sweep over option metadata catches every service whose enable this
	// flake sets, including those no unit name reaches.
	var sweep map[string][]string
	if err := r.EvalWith(ctx, dir, sub.Attr+".options", enableSweepExpr, over, &sweep); err == nil {
		for svc, files := range sweep {
			for _, f := range files {
				if mine(dir, f) {
					wanted[svc] = true
					break
				}
			}
		}
	}

	var mu sync.Mutex
	ours := map[string]bool{}
	var skipped []string
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup
	for svc := range wanted {
		wg.Add(1)
		go func(svc string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			var files []string
			if err := r.EvalWith(ctx, dir, sub.Attr+".options.services."+svc, scanExpr, over, &files); err != nil {
				mu.Lock()
				skipped = append(skipped, svc)
				mu.Unlock()
				return
			}
			for _, f := range files {
				if mine(dir, f) {
					mu.Lock()
					ours[svc] = true
					mu.Unlock()
					return
				}
			}
		}(svc)
	}
	wg.Wait()

	set := &Set{Skipped: skipped}
	sort.Strings(set.Skipped)
	seen := map[string]bool{}
	for _, u := range unitNames {
		svc, ok := pairs[u]
		if !ok || !ours[svc] {
			continue
		}
		m := storePkg.FindStringSubmatch(units[u])
		if m == nil {
			continue
		}
		name, version := nameVersion(m[1])
		if strings.HasPrefix(name, "unit-script") || version == "" {
			continue
		}
		if seen[name] {
			continue
		}
		seen[name] = true
		set.Pkgs = append(set.Pkgs, Pkg{Name: name, Version: version, Unit: u, Service: svc})
	}
	// A service of ours that no unit name reached still has a package option.
	matched := map[string]bool{}
	for _, svc := range pairs {
		matched[svc] = true
	}
	var unmatched []string
	for svc := range ours {
		if !matched[svc] {
			unmatched = append(unmatched, svc)
		}
	}
	sort.Strings(unmatched)
	for _, svc := range unmatched {
		var line string
		if err := r.EvalWith(ctx, dir, sub.Attr+".config.services."+svc, pkgOfExpr, over, &line); err != nil {
			continue
		}
		name, version, _ := strings.Cut(line, "\t")
		if name == "" || name == "?" || version == "" || seen[name] {
			continue
		}
		seen[name] = true
		set.Pkgs = append(set.Pkgs, Pkg{Name: name, Version: version, Service: svc})
	}

	sort.Slice(set.Pkgs, func(i, j int) bool { return set.Pkgs[i].Name < set.Pkgs[j].Name })
	return set, nil
}

func collectShell(ctx context.Context, r *flake.Runner, dir string, sub subject.Subject, over map[string]string) (*Set, error) {
	var lines []string
	attr := sub.Attr
	if sub.Kind == subject.DevShell {
		attr += ".nativeBuildInputs"
	}
	if err := r.EvalWith(ctx, dir, attr, sysPkgsExpr, over, &lines); err != nil {
		return nil, err
	}
	return setFromLines(lines), nil
}

// collectDarwin reports the flat package list a darwin host declares:
// environment.systemPackages plus each home-manager user's home.packages.
// systemd provenance has no darwin analog, so no service column here.
func collectDarwin(ctx context.Context, r *flake.Runner, dir string, sub subject.Subject, over map[string]string) (*Set, error) {
	var lines []string
	if err := r.EvalWith(ctx, dir, sub.Attr+".config.environment.systemPackages", sysPkgsExpr, over, &lines); err != nil {
		return nil, err
	}
	// A host without the home-manager module lacks this attribute, which is
	// not an error: the system packages still report.
	var home []string
	if err := r.EvalWith(ctx, dir, sub.Attr+".config.home-manager.users", homePkgsExpr, over, &home); err == nil {
		lines = append(lines, home...)
	}
	return setFromLines(lines), nil
}

func setFromLines(lines []string) *Set {
	set := &Set{}
	seen := map[string]bool{}
	for _, l := range lines {
		name, version, _ := strings.Cut(l, "\t")
		if name == "" || name == "?" || seen[name] {
			continue
		}
		seen[name] = true
		set.Pkgs = append(set.Pkgs, Pkg{Name: name, Version: version})
	}
	sort.Slice(set.Pkgs, func(i, j int) bool { return set.Pkgs[i].Name < set.Pkgs[j].Name })
	return set
}

// MinePrefixes returns the module prefixes this flake configures on a subject.
// It reuses the enable sweep, which is one evaluation.
func MinePrefixes(ctx context.Context, r *flake.Runner, dir string, sub subject.Subject) ([]string, error) {
	if sub.Kind != subject.Host {
		return nil, nil
	}
	var sweep map[string][]string
	if err := r.Eval(ctx, dir, sub.Attr+".options", enableSweepExpr, &sweep); err != nil {
		return nil, err
	}
	var out []string
	for svc, files := range sweep {
		for _, f := range files {
			if mine(dir, f) {
				out = append(out, "services."+svc)
				break
			}
		}
	}
	sort.Strings(out)
	return out, nil
}

// Mine reports whether a defining file sits inside the flake at dir.
func Mine(dir, file string) bool { return mine(dir, file) }
