// Package revs builds the revision-only report: which input revisions moved,
// and which nixpkgs each input pins.
package revs

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/oschrenk/thaw/internal/flake"
	"github.com/oschrenk/thaw/internal/lock"
)

// Pin is a nixpkgs an input carries of its own, reported one level down.
type Pin struct {
	Name     string
	Locked   string
	Upstream string
}

type Input struct {
	Name     string `json:"name"`
	Ref      string `json:"ref,omitempty"`
	Locked   string `json:"locked,omitempty"`
	Upstream string `json:"upstream,omitempty"`
	Follows  string `json:"follows,omitempty"`
	Pins     []Pin  `json:"pins,omitempty"`
	Err      error  `json:"-"`
}

func (i Input) Moved() bool { return i.Upstream != "" && i.Locked != i.Upstream }

type Report struct {
	Inputs []Input
}

func (r Report) Moved() int {
	n := 0
	for _, i := range r.Inputs {
		if i.Moved() {
			n++
		}
	}
	return n
}

// pinned names the inputs worth reporting one level down. A transitive nixpkgs
// decides every package under the input that pins it, which is the fact
// rpi-update-preview.sh exists to report.
var pinned = []string{"nixpkgs"}

// Build reads the lock, then asks each input's upstream for its current
// revision. Overrides map an input name to a flake ref, replacing the upstream
// the lock names.
func Build(ctx context.Context, r *flake.Runner, l *lock.Lock, overrides map[string]string) (*Report, error) {
	rootInputs, err := l.RootInputs()
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(rootInputs))
	for name := range rootInputs {
		names = append(names, name)
	}
	sort.Strings(names)

	out := make([]Input, len(names))
	var wg sync.WaitGroup
	for idx, name := range names {
		wg.Add(1)
		go func(idx int, name string) {
			defer wg.Done()
			out[idx] = resolve(ctx, r, l, name, rootInputs[name], overrides[name])
		}(idx, name)
	}
	wg.Wait()

	return &Report{Inputs: out}, nil
}

func resolve(ctx context.Context, r *flake.Runner, l *lock.Lock, name, key, override string) Input {
	in := Input{Name: name}

	node := l.Nodes[key]
	if node.Locked != nil {
		in.Locked = node.Locked.Rev
	}

	ref := override
	if ref == "" {
		ref, in.Err = flake.Ref(node.Original)
		if in.Err != nil {
			return in
		}
	}

	in.Ref = ref
	meta, err := r.MetadataOf(ctx, ref)
	if err != nil {
		in.Err = err
		return in
	}
	in.Upstream = meta.Locked.Rev

	for _, p := range pinned {
		ref, ok := node.Inputs[p]
		if !ok {
			continue
		}
		// A follows edge is not a pin. The input takes whatever the root
		// resolves p to, so reporting the upstream flake's own p would name a
		// revision this flake never uses.
		if ref.Follow != nil {
			in.Follows = "follows " + joinPath(ref.Follow)
			continue
		}
		lockedPin, err1 := l.Rev(name, p)
		if err1 != nil {
			continue
		}
		upstreamPin := ""
		if meta.Locks.Root != "" {
			if rev, err2 := meta.Locks.Rev(p); err2 == nil {
				upstreamPin = rev
			}
		}
		in.Pins = append(in.Pins, Pin{Name: p, Locked: lockedPin, Upstream: upstreamPin})
	}
	return in
}

func joinPath(p []string) string {
	s := ""
	for i, seg := range p {
		if i > 0 {
			s += "."
		}
		s += seg
	}
	return s
}

func Short(rev string) string {
	if len(rev) > 7 {
		return rev[:7]
	}
	if rev == "" {
		return "?"
	}
	return rev
}

func (p Pin) Moved() bool { return p.Upstream != "" && p.Locked != p.Upstream }

func (p Pin) String() string {
	return fmt.Sprintf("pins %s %s -> %s", p.Name, Short(p.Locked), Short(p.Upstream))
}
