package render

import (
	"fmt"
	"io"
	"strings"

	"github.com/oschrenk/thaw/internal/opts"
)

// A removal of an option this flake sets breaks the next rebuild. A removal of
// one nothing here sets is harmless, so the two take different marks.
func (s Style) optMark(k opts.Change) string {
	switch k {
	case opts.Added:
		if s.Fancy {
			return "+"
		}
		return "A"
	case opts.RemovedAndSet:
		if s.Fancy {
			return "~"
		}
		return "!"
	}
	if s.Fancy {
		return "-"
	}
	return "D"
}

func Options(w io.Writer, r *opts.Report, s Style) error {
	out := &ew{w: w}
	var added, removed, breaking int

	for _, g := range r.Groups {
		if s.Fancy {
			out.line("  " + s.paint(bold, g.Prefix))
		} else {
			out.line(g.Prefix)
		}
		for _, row := range g.Rows {
			switch row.Kind {
			case opts.Added:
				added++
			case opts.Removed:
				removed++
			case opts.RemovedAndSet:
				breaking++
			}
			note := ""
			if row.Kind == opts.RemovedAndSet {
				note = s.paint(dim, strings.Join(row.Subjects, " ")+" sets this")
			}
			lead := "    "
			if !s.Fancy {
				lead = ""
			}
			out.printf("%s%s %-52s %s\n", lead, s.optMark(row.Kind), row.Path, note)
		}
		out.line("")
	}

	if rule := s.rule(46); rule != "" {
		out.line(s.paint(dim, rule))
	}
	summary := fmt.Sprintf("%d removed options are set here   %d added   %d removed", breaking, added, removed)
	if s.Fancy {
		out.printf("%s %s\n", s.marker(breaking > 0), summary)
	} else {
		out.line(summary)
	}
	for _, sk := range r.Skipped {
		out.printf("%s\n", s.paint(dim, "  skipped "+sk+": it would not evaluate"))
	}
	return out.err
}
