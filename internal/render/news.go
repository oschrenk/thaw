package render

import (
	"fmt"
	"io"
	"strings"

	"github.com/oschrenk/thaw/internal/news"
	"github.com/oschrenk/thaw/internal/revs"
)

// News prints the entries a candidate revision adds, one dated block each.
func News(w io.Writer, r *news.Report, s Style) error {
	out := &ew{w: w}
	head := fmt.Sprintf("%s  %s %s %s", r.Input, revs.Short(r.Locked), s.arrow(), revs.Short(r.Upstream))
	if s.Fancy {
		out.line("  " + s.paint(bold, head))
	} else {
		out.line(head)
	}
	if rule := s.rule(46); rule != "" {
		out.line(s.paint(dim, rule))
	}

	for _, e := range r.Entries {
		date, _, _ := strings.Cut(e.Time, "T")
		out.line("")
		out.printf("  %s\n", s.paint(bold, date))
		for _, line := range strings.Split(strings.TrimRight(e.Message, "\n"), "\n") {
			out.printf("  %s\n", line)
		}
	}

	out.line("")
	if rule := s.rule(46); rule != "" {
		out.line(s.paint(dim, rule))
	}
	summary := fmt.Sprintf("%d new entries", len(r.Entries))
	if len(r.Entries) == 1 {
		summary = "1 new entry"
	}
	if s.Fancy {
		out.printf("%s %s\n", s.marker(len(r.Entries) > 0), summary)
	} else {
		out.line(summary)
	}
	return out.err
}
