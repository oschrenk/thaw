package render

import (
	"fmt"
	"io"
	"strings"

	"github.com/oschrenk/thaw/internal/diff"
	"github.com/oschrenk/thaw/internal/revs"
)

// An unchanged row marks "=" in plain output so `grep '^M'` keeps finding
// only what moved.
func (s Style) changeMark(k diff.Change) string {
	if s.Fancy {
		switch k {
		case diff.Added:
			return "+"
		case diff.Removed:
			return "-"
		case diff.Unchanged:
			return "·"
		}
		return "↑"
	}
	switch k {
	case diff.Added:
		return "A"
	case diff.Removed:
		return "D"
	case diff.Unchanged:
		return "="
	}
	return "M"
}

func Packages(w io.Writer, r *diff.Report, s Style) error {
	out := &ew{w: w}
	for _, g := range r.Groups {
		head := fmt.Sprintf("%s  %s %s %s", g.Input, revs.Short(g.Locked), s.arrow(), revs.Short(g.Upstream))
		if s.Fancy {
			out.line("  " + s.paint(bold, head))
		} else {
			out.line(head)
		}
		if g.PinName != "" {
			pin := fmt.Sprintf("pins %s        %s %s %s", g.PinName,
				revs.Short(g.PinLocked), s.arrow(), revs.Short(g.PinUpstream))
			if s.Fancy {
				out.line("  " + s.paint(dim, pin))
			} else {
				out.line("  " + pin)
			}
		}
		if rule := s.rule(46); rule != "" {
			out.line(s.paint(dim, rule))
		}

		switch {
		case g.Err != nil:
			out.printf("  %s\n", s.paint(dim, "unreadable: "+lastLine(g.Err.Error())))
		case !g.Moved:
			out.printf("  %s %s\n", s.marker(false), s.paint(dim, "unchanged"))
		case len(g.Rows) == 0:
			out.printf("  %s %s\n", s.marker(false), s.paint(dim, "moved, no package changed"))
		default:
			width := 0
			for _, row := range g.Rows {
				if len(row.Name) > width {
					width = len(row.Name)
				}
			}
			hidden := 0
			for _, row := range g.Rows {
				if row.Kind == diff.Unchanged && !s.WithUnchanged {
					hidden++
					continue
				}
				from, to := row.From, row.To
				if from == "" {
					from = emptyVersion(s)
				}
				if to == "" {
					to = emptyVersion(s)
				}
				mark := s.changeMark(row.Kind)
				lead := "    "
				if !s.Fancy {
					lead = ""
				}
				if row.Kind == diff.Unchanged {
					line := fmt.Sprintf("%s%s %-*s  %-12s %s %-12s %s", lead, mark, width, row.Name,
						from, s.arrow(), to, strings.Join(row.Subjects, " "))
					out.line(s.paint(dim, line))
					continue
				}
				out.printf("%s%s %-*s  %-12s %s %-12s %s\n", lead, mark, width, row.Name,
					from, s.arrow(), to, s.paint(dim, strings.Join(row.Subjects, " ")))
			}
			if hidden > 0 {
				out.printf("  %s %s\n", s.marker(false),
					s.paint(dim, fmt.Sprintf("%d unchanged", hidden)))
			}
		}
		out.line("")
	}

	changed, added, removed, unchanged := r.Counts()
	summary := fmt.Sprintf("%d changed  %d added  %d removed  %d unchanged", changed, added, removed, unchanged)
	if rule := s.rule(46); rule != "" {
		out.line(s.paint(dim, rule))
	}
	if s.Fancy {
		out.printf("%s %s\n", s.marker(changed+added+removed > 0), summary)
	} else {
		out.line(summary)
	}
	for _, sk := range r.Skipped {
		out.printf("%s\n", s.paint(dim, "  skipped "+sk+": it would not evaluate"))
	}
	return out.err
}

func emptyVersion(s Style) string {
	if s.Fancy {
		return "—"
	}
	return "-"
}
