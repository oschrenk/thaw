// Package opts diffs the NixOS option set a subject declares.
//
// The scope rule is the module prefix, not the option. Filtering to options a
// configuration sets would report a removal and never an addition, because
// nothing sets an option that does not exist yet.
package opts

import (
	"context"
	"sort"
	"strings"
	"sync"

	"github.com/oschrenk/thaw/internal/flake"
	"github.com/oschrenk/thaw/internal/subject"
)

type Change int

const (
	Added Change = iota
	Removed
	RemovedAndSet
)

type Row struct {
	Path     string
	Kind     Change
	Subjects []string
}

type Group struct {
	Prefix string
	Rows   []Row
}

type Report struct {
	Groups  []Group
	Skipped []string
}

type scan struct {
	paths map[string]bool // every option path under the prefix
	set   map[string]bool // the ones a file in this flake defines
}

// Prefixes truncates each assigned option path to its module prefix. A path
// under services takes two segments, so services.prometheus.retentionTime
// becomes services.prometheus; anything else takes one.
func Prefixes(paths []string) []string {
	seen := map[string]bool{}
	for _, p := range paths {
		parts := strings.Split(p, ".")
		n := 1
		if parts[0] == "services" && len(parts) > 1 {
			n = 2
		}
		if len(parts) < n {
			continue
		}
		seen[strings.Join(parts[:n], ".")] = true
	}
	out := make([]string, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

func readScan(ctx context.Context, r *flake.Runner, dir, attr string, over map[string]string, isMine func(string) bool) (*scan, error) {
	var lines []string
	if err := r.EvalWith(ctx, dir, attr, pathsExpr, over, &lines); err != nil {
		return nil, err
	}
	s := &scan{paths: map[string]bool{}, set: map[string]bool{}}
	for _, l := range lines {
		path, files, hasFiles := strings.Cut(l, "\t")
		s.paths[path] = true
		if !hasFiles {
			continue
		}
		for _, f := range strings.Split(files, ",") {
			if isMine(f) {
				s.set[path] = true
				break
			}
		}
	}
	return s, nil
}

// Build diffs each prefix's option subtree between the lock and a candidate.
func Build(ctx context.Context, r *flake.Runner, dir string, subs []subject.Subject,
	prefixes map[string][]string, over map[string]string, isMine func(string) bool) (*Report, error) {

	type result struct {
		prefix string
		sub    string
		rows   []Row
		err    error
	}

	var jobs []struct{ sub, prefix, attr string }
	for _, sub := range subs {
		for _, p := range prefixes[sub.Name] {
			jobs = append(jobs, struct{ sub, prefix, attr string }{sub.Name, p, sub.Attr + ".options." + p})
		}
	}

	results := make([]result, len(jobs))
	sem := make(chan struct{}, 6)
	var wg sync.WaitGroup
	for i, j := range jobs {
		wg.Add(1)
		go func(i int, sub, prefix, attr string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			before, err := readScan(ctx, r, dir, attr, nil, isMine)
			if err != nil {
				results[i] = result{prefix: prefix, sub: sub, err: err}
				return
			}
			after, err := readScan(ctx, r, dir, attr, over, isMine)
			if err != nil {
				results[i] = result{prefix: prefix, sub: sub, err: err}
				return
			}

			// The scan starts at the prefix, so its paths are relative to it.
			full := func(p string) string {
				if p == "" {
					return prefix
				}
				return prefix + "." + p
			}
			var rows []Row
			for p := range after.paths {
				if !before.paths[p] {
					rows = append(rows, Row{Path: full(p), Kind: Added, Subjects: []string{sub}})
				}
			}
			for p := range before.paths {
				if after.paths[p] {
					continue
				}
				kind := Removed
				if before.set[p] {
					kind = RemovedAndSet
				}
				rows = append(rows, Row{Path: full(p), Kind: kind, Subjects: []string{sub}})
			}
			results[i] = result{prefix: prefix, sub: sub, rows: rows}
		}(i, j.sub, j.prefix, j.attr)
	}
	wg.Wait()

	byPrefix := map[string]map[string]*Row{}
	var skipped []string
	for _, res := range results {
		if res.err != nil {
			skipped = append(skipped, res.prefix)
			continue
		}
		if byPrefix[res.prefix] == nil {
			byPrefix[res.prefix] = map[string]*Row{}
		}
		for _, row := range res.rows {
			if existing, ok := byPrefix[res.prefix][row.Path]; ok {
				existing.Subjects = append(existing.Subjects, row.Subjects...)
				if row.Kind == RemovedAndSet {
					existing.Kind = RemovedAndSet
				}
				continue
			}
			r := row
			byPrefix[res.prefix][row.Path] = &r
		}
	}

	report := &Report{}
	for prefix, rows := range byPrefix {
		if len(rows) == 0 {
			continue
		}
		g := Group{Prefix: prefix}
		for _, row := range rows {
			sort.Strings(row.Subjects)
			g.Rows = append(g.Rows, *row)
		}
		sort.Slice(g.Rows, func(i, j int) bool {
			if g.Rows[i].Kind != g.Rows[j].Kind {
				return g.Rows[i].Kind < g.Rows[j].Kind
			}
			return g.Rows[i].Path < g.Rows[j].Path
		})
		report.Groups = append(report.Groups, g)
	}
	sort.Slice(report.Groups, func(i, j int) bool { return report.Groups[i].Prefix < report.Groups[j].Prefix })
	sort.Strings(skipped)
	report.Skipped = skipped
	return report, nil
}
