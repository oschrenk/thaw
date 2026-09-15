// Package news reports the home-manager news entries a candidate revision
// adds over the locked one, filtered to the entries whose condition holds for
// this configuration.
package news

import (
	"context"
	"sort"

	"github.com/oschrenk/thaw/internal/flake"
	"github.com/oschrenk/thaw/internal/subject"
)

type Entry struct {
	Time    string `json:"time"`
	Message string `json:"message"`
}

type Report struct {
	Input    string
	Locked   string
	Upstream string
	Entries  []Entry
}

// entriesExpr flattens every home-manager user's condition-passing news
// entries. Each user is guarded: one broken entry must not empty the report.
const entriesExpr = `us: builtins.concatLists (map (u:
  let r = builtins.tryEval (map (e: { time = e.time; message = e.message; })
    (builtins.filter (e: e.condition) us.${u}.news.entries));
  in if r.success then r.value else []) (builtins.attrNames us))`

func collect(ctx context.Context, r *flake.Runner, dir string, sub subject.Subject, over map[string]string) ([]Entry, error) {
	var out []Entry
	err := r.EvalWith(ctx, dir, sub.Attr+".config.home-manager.users", entriesExpr, over, &out)
	return out, err
}

// Build evaluates the entries under the lock and under the candidate revision
// of one input, and reports what the candidate adds.
func Build(ctx context.Context, r *flake.Runner, dir string, subs []subject.Subject, input, locked, upstream, ref string) (*Report, error) {
	over := map[string]string{input: ref}

	var base, cand []Entry
	for _, sub := range subs {
		b, err := collect(ctx, r, dir, sub, nil)
		if err != nil {
			return nil, err
		}
		base = append(base, b...)
		c, err := collect(ctx, r, dir, sub, over)
		if err != nil {
			return nil, err
		}
		cand = append(cand, c...)
	}

	return &Report{
		Input: input, Locked: locked, Upstream: upstream,
		Entries: Diff(base, cand),
	}, nil
}

// Diff returns the entries in cand that base lacks, sorted by time. The key
// is time plus message, because entries have no id.
func Diff(base, cand []Entry) []Entry {
	seen := map[Entry]bool{}
	for _, e := range base {
		seen[e] = true
	}
	var out []Entry
	dup := map[Entry]bool{}
	for _, e := range cand {
		if seen[e] || dup[e] {
			continue
		}
		dup[e] = true
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Time < out[j].Time })
	return out
}
