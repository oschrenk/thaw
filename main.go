// Command thaw previews what an update would bring, before anything writes a
// lock file.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/oschrenk/thaw/internal/diff"
	"github.com/oschrenk/thaw/internal/flake"
	"github.com/oschrenk/thaw/internal/lock"
	"github.com/oschrenk/thaw/internal/opts"
	"github.com/oschrenk/thaw/internal/pkgs"
	"github.com/oschrenk/thaw/internal/render"
	"github.com/oschrenk/thaw/internal/revs"
	"github.com/oschrenk/thaw/internal/subject"
)

type options struct {
	sub           string
	dir           string
	revOnly       bool
	check         bool
	all           bool
	asJSON        bool
	withUnchanged bool
	forName       string
	input         string
	to            map[string]string
}

func main() {
	root, code := newRoot()
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "thaw:", err)
		os.Exit(1)
	}
	os.Exit(*code)
}

// newRoot builds the command tree. The exit code travels through a pointer,
// because RunE returns only an error and --check exits 1 without one.
func newRoot() (*cobra.Command, *int) {
	o := &options{to: map[string]string{}}
	code := new(int)
	var toPairs []string

	prepare := func(sub string, revOnly bool) func(*cobra.Command, []string) error {
		return func(_ *cobra.Command, args []string) error {
			o.sub, o.dir = sub, "."
			if revOnly {
				o.revOnly = true
			}
			if len(args) > 0 {
				o.dir = args[0]
			}
			for _, t := range toPairs {
				name, ref, ok := strings.Cut(t, "=")
				if !ok || name == "" || ref == "" {
					return fmt.Errorf("--to wants <input>=<ref>, got %q", t)
				}
				o.to[name] = ref
			}
			c, err := execute(o)
			*code = c
			return err
		}
	}

	completeFor := func(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
		dir := "."
		if len(args) > 0 {
			dir = args[0]
		}
		set, err := subject.Enumerate(context.Background(), flake.NewRunner(), dir)
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}
		return set.Names(), cobra.ShellCompDirectiveNoFileComp
	}
	completeInput := func(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
		dir := "."
		if len(args) > 0 {
			dir = args[0]
		}
		l, err := lock.Load(filepath.Join(dir, "flake.lock"))
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}
		inputs, err := l.RootInputs()
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}
		names := make([]string, 0, len(inputs))
		for name := range inputs {
			names = append(names, name)
		}
		sort.Strings(names)
		return names, cobra.ShellCompDirectiveNoFileComp
	}

	common := func(c *cobra.Command) {
		c.Flags().BoolVar(&o.asJSON, "json", false, "machine-readable output")
		c.Flags().BoolVar(&o.check, "check", false, "exit non-zero when anything moves")
		c.Flags().StringVar(&o.input, "input", "", "report only what comes from this input")
		c.Flags().StringArrayVar(&toPairs, "to", nil, "compare one input against a ref instead of upstream HEAD")
		_ = c.RegisterFlagCompletionFunc("input", completeInput)
	}
	forFlag := func(c *cobra.Command) {
		c.Flags().StringVar(&o.forName, "for", "", "report only this subject: a host, devshell, a package")
		_ = c.RegisterFlagCompletionFunc("for", completeFor)
	}

	root := &cobra.Command{
		Use:   "thaw [flake]",
		Short: "preview what an update would bring, before anything writes a lock",
		Long: "thaw previews what an update would bring, before anything writes a lock.\n" +
			"Bare thaw runs packages --rev-only. The flake argument defaults to \".\".",
		Args:          cobra.MaximumNArgs(1),
		RunE:          prepare("packages", true),
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	common(root)
	forFlag(root)
	root.Flags().BoolVar(&o.revOnly, "rev-only", false,
		"report which input revisions moved, and evaluate nothing")

	packagesCmd := &cobra.Command{
		Use:   "packages [flake]",
		Short: "which package versions change, appear and disappear",
		Args:  cobra.MaximumNArgs(1),
		RunE:  prepare("packages", false),
	}
	common(packagesCmd)
	forFlag(packagesCmd)
	packagesCmd.Flags().BoolVar(&o.revOnly, "rev-only", false,
		"report which input revisions moved, and evaluate nothing")
	packagesCmd.Flags().BoolVar(&o.all, "all", false,
		"report the whole build closure, build tools included")
	packagesCmd.Flags().BoolVar(&o.withUnchanged, "with-unchanged", false,
		"print unchanged packages too, not only their count")

	optionsCmd := &cobra.Command{
		Use:   "options [flake]",
		Short: "which NixOS options appear and disappear",
		Args:  cobra.MaximumNArgs(1),
		RunE:  prepare("options", false),
	}
	common(optionsCmd)
	forFlag(optionsCmd)

	subjectsCmd := &cobra.Command{
		Use:   "subjects [flake]",
		Short: "which targets --for accepts",
		Args:  cobra.MaximumNArgs(1),
		RunE:  prepare("subjects", false),
	}
	subjectsCmd.Flags().BoolVar(&o.asJSON, "json", false, "machine-readable output")

	root.AddCommand(packagesCmd, optionsCmd, subjectsCmd)
	return root, code
}

func execute(o *options) (int, error) {
	runner := flake.NewRunner()
	ctx := context.Background()

	// subjects only enumerates, so it needs no lock and no upstream lookup.
	if o.sub == "subjects" {
		set, err := subject.Enumerate(ctx, runner, o.dir)
		if err != nil {
			return 0, err
		}
		if o.asJSON {
			return 0, emit(set)
		}
		return 0, render.Subjects(os.Stdout, set, render.StyleFor(os.Stdout))
	}

	if o.forName != "" && o.revOnly {
		// A revision report has no subject to narrow: an input moves for the
		// whole flake or not at all.
		return 0, fmt.Errorf("--for has no meaning with --rev-only, which reports inputs rather than subjects")
	}

	l, err := lock.Load(filepath.Join(o.dir, "flake.lock"))
	if err != nil {
		return 0, err
	}

	report, err := revs.Build(ctx, runner, l, o.to)
	if err != nil {
		return 0, err
	}

	style := render.StyleFor(os.Stdout)
	style.WithUnchanged = o.withUnchanged

	if o.revOnly {
		if o.asJSON {
			if err := emit(report); err != nil {
				return 0, err
			}
			if o.check && report.Moved() > 0 {
				return 1, nil
			}
			return 0, nil
		}
		if err := render.Revs(os.Stdout, report, style); err != nil {
			return 0, err
		}
		if o.check && report.Moved() > 0 {
			return 1, nil
		}
		return 0, nil
	}

	set, err := subject.Enumerate(ctx, runner, o.dir)
	if err != nil {
		return 0, err
	}
	subs := set.Default()
	if o.forName != "" {
		one, err := set.Lookup(o.forName)
		if err != nil {
			return 0, err
		}
		subs = []subject.Subject{one}
	}

	if o.input != "" {
		var kept []revs.Input
		for _, in := range report.Inputs {
			if in.Name == o.input {
				kept = append(kept, in)
			}
		}
		if len(kept) == 0 {
			return 0, fmt.Errorf("no input named %q in flake.lock", o.input)
		}
		report.Inputs = kept
	}

	if o.sub == "options" {
		over := map[string]string{}
		for _, in := range report.Inputs {
			if in.Moved() && in.Ref != "" {
				over[in.Name] = in.Ref
			}
		}
		if len(over) == 0 {
			fmt.Println("no input moved, so no option can have changed")
			return 0, nil
		}
		prefixes := map[string][]string{}
		for _, sub := range subs {
			p, err := pkgs.MinePrefixes(ctx, runner, o.dir, sub)
			if err != nil {
				continue
			}
			prefixes[sub.Name] = p
		}
		isMine := func(f string) bool { return pkgs.Mine(o.dir, f) }
		or, err := opts.Build(ctx, runner, o.dir, subs, prefixes, over, isMine)
		if err != nil {
			return 0, err
		}
		if o.asJSON {
			return 0, emit(or)
		}
		if err := render.Options(os.Stdout, or, style); err != nil {
			return 0, err
		}
		return 0, nil
	}

	// The counter goes to stderr and only when that is a terminal, so a
	// pipe, a redirect and --json all stay clean.
	var progress diff.Progress
	if render.StyleFor(os.Stderr).Fancy {
		progress = func(done, total int, sub, input string) {
			against := input
			if against == "" {
				against = "the lock"
			}
			fmt.Fprintf(os.Stderr, "\r\033[K  evaluating %d/%d: %s against %s", done, total, sub, against)
		}
	}
	d, err := diff.Build(ctx, runner, o.dir, subs, report, o.all, progress)
	if progress != nil {
		fmt.Fprint(os.Stderr, "\r\033[K")
	}
	if err != nil {
		return 0, err
	}
	if o.asJSON {
		if err := emit(d); err != nil {
			return 0, err
		}
	} else if err := render.Packages(os.Stdout, d, style); err != nil {
		return 0, err
	}
	c, a, rm, _ := d.Counts()
	if o.check && c+a+rm > 0 {
		return 1, nil
	}
	return 0, nil
}

func emit(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
