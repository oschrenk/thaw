// Package flake runs nix and reads the JSON it prints.
package flake

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strings"

	"github.com/oschrenk/thaw/internal/lock"
)

// Ref reconstructs a flake reference from a lock entry's original block, which
// is what names the upstream to ask for a current revision.
func Ref(o *lock.Locked) (string, error) {
	if o == nil {
		return "", fmt.Errorf("input has no original entry")
	}
	switch o.Type {
	case "github", "gitlab", "sourcehut":
		if o.Owner == "" || o.Repo == "" {
			return "", fmt.Errorf("%s ref names no owner or repo", o.Type)
		}
		s := fmt.Sprintf("%s:%s/%s", o.Type, o.Owner, o.Repo)
		if o.Ref != "" {
			s += "/" + o.Ref
		}
		return s, nil
	case "git", "tarball", "file", "path":
		if o.URL == "" {
			return "", fmt.Errorf("%s ref names no url", o.Type)
		}
		return fmt.Sprintf("%s:%s", o.Type, o.URL), nil
	default:
		return "", fmt.Errorf("unsupported input type %q", o.Type)
	}
}

// Metadata is the part of `nix flake metadata --json` thaw reads.
type Metadata struct {
	Locked   lock.Locked `json:"locked"`
	Original lock.Locked `json:"original"`
	Locks    lock.Lock   `json:"locks"`
}

type Runner struct {
	Bin   string
	Extra []string
}

func NewRunner() *Runner {
	return &Runner{Bin: "nix", Extra: []string{"--no-warn-dirty", "--extra-experimental-features", "nix-command flakes"}}
}

func (r *Runner) run(ctx context.Context, args ...string) ([]byte, error) {
	full := append(append([]string{}, args...), r.Extra...)
	cmd := exec.CommandContext(ctx, r.Bin, full...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			return nil, fmt.Errorf("nix %s: %w", strings.Join(args, " "), err)
		}
		return nil, fmt.Errorf("nix %s: %w: %s", strings.Join(args, " "), err, lastLine(msg))
	}
	return out, nil
}

func lastLine(s string) string {
	lines := strings.Split(s, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if t := strings.TrimSpace(lines[i]); t != "" {
			return t
		}
	}
	return s
}

// MetadataOf reads `nix flake metadata --json` for a flake reference.
func (r *Runner) MetadataOf(ctx context.Context, ref string) (*Metadata, error) {
	out, err := r.run(ctx, "flake", "metadata", ref, "--json")
	if err != nil {
		return nil, err
	}
	var m Metadata
	if err := json.Unmarshal(out, &m); err != nil {
		return nil, fmt.Errorf("parse metadata of %s: %w", ref, err)
	}
	return &m, nil
}

// Eval reads `nix eval --json` for an attribute path, optionally through an
// apply expression, into v.
func (r *Runner) Eval(ctx context.Context, dir, attr, apply string, v any) error {
	return r.EvalWith(ctx, dir, attr, apply, nil, v)
}

// EvalWith evaluates under overridden inputs. The overrides live in memory:
// nothing writes a lock.
func (r *Runner) EvalWith(ctx context.Context, dir, attr, apply string, overrides map[string]string, v any) error {
	args := []string{"eval", "--json", dir + "#" + attr}
	if apply != "" {
		args = append(args, "--apply", apply)
	}
	names := make([]string, 0, len(overrides))
	for name := range overrides {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		args = append(args, "--override-input", name, overrides[name])
	}
	out, err := r.run(ctx, args...)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(out, v); err != nil {
		return fmt.Errorf("parse eval of %s: %w", attr, err)
	}
	return nil
}

// CurrentSystem asks nix which system it is running on.
func (r *Runner) CurrentSystem(ctx context.Context) (string, error) {
	out, err := r.run(ctx, "eval", "--impure", "--raw", "--expr", "builtins.currentSystem")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// DerivationShow reads the recursive derivation graph of an installable. The
// output runs to tens of megabytes, so callers decode it once and keep only
// what they need.
func (r *Runner) DerivationShow(ctx context.Context, dir, attr string, overrides map[string]string) ([]byte, error) {
	args := []string{"derivation", "show", "-r", dir + "#" + attr}
	names := make([]string, 0, len(overrides))
	for name := range overrides {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		args = append(args, "--override-input", name, overrides[name])
	}
	return r.run(ctx, args...)
}
