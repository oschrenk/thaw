package render

import (
	"fmt"
	"io"
	"strings"

	"github.com/oschrenk/thaw/internal/subject"
)

// Subjects lists the --for namespace: every target, its kind, and the flake
// attribute it evaluates.
func Subjects(w io.Writer, set subject.Set, s Style) error {
	out := &ew{w: w}
	width := len("SUBJECT")
	for _, sub := range set.Subjects {
		if len(sub.Name) > width {
			width = len(sub.Name)
		}
	}

	head := fmt.Sprintf("  %-*s  %-10s %s", width, "SUBJECT", "KIND", "ATTR")
	out.line(s.paint(bold, strings.TrimRight(head, " ")))
	if rule := s.rule(width + 26); rule != "" {
		out.line(s.paint(dim, rule))
	}
	for _, sub := range set.Subjects {
		out.printf("  %-*s  %-10s %s\n", width, sub.Name, sub.Kind.String(),
			s.paint(dim, sub.Attr))
	}
	return out.err
}
