// Package diff compares a subject's packages under the locked inputs against
// the same subject under a candidate revision, and groups the result by the
// input a bump would move.
package diff

import (
	"context"
	"sort"
	"sync"

	"github.com/oschrenk/thaw/internal/flake"
	"github.com/oschrenk/thaw/internal/pkgs"
	"github.com/oschrenk/thaw/internal/revs"
	"github.com/oschrenk/thaw/internal/subject"
)

type Change int

// Unchanged sorts last so movement stays at the top of a group.
const (
	Changed Change = iota
	Added
	Removed
	Unchanged
)

func (c Change) String() string {
	switch c {
	case Added:
		return "added"
	case Removed:
		return "removed"
	case Unchanged:
		return "unchanged"
	}
	return "changed"
}

func (c Change) MarshalJSON() ([]byte, error) {
	return []byte(`"` + c.String() + `"`), nil
}

type Row struct {
	Name     string
	From     string
	To       string
	Kind     Change
	Subjects []string
}

// Group holds every change one input would bring. An input that can never
// contribute a package says so, which reads differently from moving nothing.
type Group struct {
	Input       string
	Locked      string
	Upstream    string
	PinName     string
	PinLocked   string
	PinUpstream string
	Rows        []Row
	Moved       bool
	Err         error
}

type Report struct {
	Groups  []Group
	Skipped []string
}

func (r Report) Counts() (changed, added, removed, unchanged int) {
	for _, g := range r.Groups {
		for _, row := range g.Rows {
			switch row.Kind {
			case Changed:
				changed++
			case Added:
				added++
			case Removed:
				removed++
			case Unchanged:
				unchanged++
			}
		}
	}
	return
}

type baseline map[string]map[string]string // subject -> pkg -> version

// Progress reports the start of one evaluation. An empty input means the
// baseline pass, the one against the lock.
type Progress func(done, total int, subject, input string)

// Build evaluates every subject once against the lock, then once per moved
// input under an override, and reports the difference.
func Build(ctx context.Context, r *flake.Runner, dir string, subs []subject.Subject, rr *revs.Report, all bool, progress Progress) (*Report, error) {
	base := baseline{}
	var mu sync.Mutex
	var skipped []string

	// The total is known before work starts: one baseline pass, plus one
	// pass per input that moved and resolves to a ref.
	movedRefs := 0
	for _, in := range rr.Inputs {
		if in.Moved() && in.Err == nil && in.Ref != "" {
			movedRefs++
		}
	}
	total := len(subs) * (1 + movedRefs)
	done := 0
	step := func(sub, input string) {
		if progress == nil {
			return
		}
		mu.Lock()
		done++
		progress(done, total, sub, input)
		mu.Unlock()
	}

	gather := pkgs.CollectWith
	if all {
		gather = pkgs.Closure
	}

	collect := func(sub subject.Subject, over map[string]string) map[string]string {
		set, err := gather(ctx, r, dir, sub, over)
		if err != nil {
			mu.Lock()
			skipped = append(skipped, sub.Name)
			mu.Unlock()
			return nil
		}
		out := map[string]string{}
		for _, p := range set.Pkgs {
			out[p.Name] = p.Version
		}
		return out
	}

	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	for _, sub := range subs {
		wg.Add(1)
		go func(sub subject.Subject) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			step(sub.Name, "")
			got := collect(sub, nil)
			mu.Lock()
			base[sub.Name] = got
			mu.Unlock()
		}(sub)
	}
	wg.Wait()

	report := &Report{}
	for _, in := range rr.Inputs {
		g := Group{Input: in.Name, Locked: in.Locked, Upstream: in.Upstream, Moved: in.Moved(), Err: in.Err}
		for _, p := range in.Pins {
			g.PinName, g.PinLocked, g.PinUpstream = p.Name, p.Locked, p.Upstream
		}
		report.Groups = append(report.Groups, g)
	}

	for gi := range report.Groups {
		g := &report.Groups[gi]
		if !g.Moved || g.Err != nil {
			continue
		}
		ref := refFor(rr, g.Input)
		if ref == "" {
			continue
		}
		over := map[string]string{g.Input: ref}

		cand := map[string]map[string]string{}
		var wg2 sync.WaitGroup
		for _, sub := range subs {
			wg2.Add(1)
			go func(sub subject.Subject) {
				defer wg2.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				step(sub.Name, g.Input)
				got := collect(sub, over)
				mu.Lock()
				cand[sub.Name] = got
				mu.Unlock()
			}(sub)
		}
		wg2.Wait()

		g.Rows = rows(base, cand, subs)
	}

	sort.Strings(skipped)
	report.Skipped = dedupe(skipped)
	return report, nil
}

type key struct{ name, from, to string }

func rows(base, cand baseline, subs []subject.Subject) []Row {
	acc := map[key]*Row{}

	for _, sub := range subs {
		b, c := base[sub.Name], cand[sub.Name]
		if b == nil || c == nil {
			continue
		}
		for name, bv := range b {
			cv, ok := c[name]
			switch {
			case !ok:
				add(acc, key{name, bv, ""}, Removed, sub.Name)
			case bv != cv:
				add(acc, key{name, bv, cv}, Changed, sub.Name)
			default:
				add(acc, key{name, bv, cv}, Unchanged, sub.Name)
			}
		}
		for name, cv := range c {
			if _, ok := b[name]; !ok {
				add(acc, key{name, "", cv}, Added, sub.Name)
			}
		}
	}

	out := make([]Row, 0, len(acc))
	for _, row := range acc {
		sort.Strings(row.Subjects)
		out = append(out, *row)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func add(acc map[key]*Row, k key, kind Change, sub string) {
	if r, ok := acc[k]; ok {
		r.Subjects = append(r.Subjects, sub)
		return
	}
	acc[k] = &Row{Name: k.name, From: k.from, To: k.to, Kind: kind, Subjects: []string{sub}}
}

func refFor(rr *revs.Report, input string) string {
	for _, in := range rr.Inputs {
		if in.Name == input {
			return in.Ref
		}
	}
	return ""
}

func dedupe(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := in[:1]
	for _, s := range in[1:] {
		if s != out[len(out)-1] {
			out = append(out, s)
		}
	}
	return out
}
