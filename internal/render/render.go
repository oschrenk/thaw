// Package render writes a report to a terminal or to a pipe.
//
// A terminal gets glyphs, colour and box rules. Anything else gets ASCII with
// git-style letter markers, so `grep '^M'` finds what moved.
package render

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/oschrenk/thaw/internal/revs"
)

type Style struct {
	Fancy bool
	// WithUnchanged prints unchanged package rows; the default hides them
	// behind a per-group count.
	WithUnchanged bool
}

func StyleFor(w io.Writer) Style {
	f, ok := w.(*os.File)
	if !ok {
		return Style{}
	}
	info, err := f.Stat()
	if err != nil {
		return Style{}
	}
	return Style{Fancy: info.Mode()&os.ModeCharDevice != 0}
}

const (
	dim   = "\033[2m"
	bold  = "\033[1m"
	green = "\033[32m"
	reset = "\033[0m"
)

func (s Style) paint(code, text string) string {
	if !s.Fancy {
		return text
	}
	return code + text + reset
}

func (s Style) arrow() string {
	if s.Fancy {
		return "→"
	}
	return "->"
}

func (s Style) rule(n int) string {
	if !s.Fancy {
		return ""
	}
	return "  " + strings.Repeat("─", n)
}

// marker is the leading column: a glyph on a terminal, a git-style letter
// elsewhere.
func (s Style) marker(moved bool) string {
	switch {
	case s.Fancy && moved:
		return "  ↑"
	case s.Fancy:
		return "  ·"
	case moved:
		return "M"
	}
	return " "
}

// ew records the first write error so a renderer can stay readable instead of
// checking every call.
type ew struct {
	w   io.Writer
	err error
}

func (e *ew) printf(format string, a ...any) {
	if e.err != nil {
		return
	}
	_, e.err = fmt.Fprintf(e.w, format, a...)
}

func (e *ew) line(s string) { e.printf("%s\n", s) }

func Revs(w io.Writer, r *revs.Report, s Style) error {
	out := &ew{w: w}
	width := len("INPUT")
	for _, in := range r.Inputs {
		if len(in.Name) > width {
			width = len(in.Name)
		}
	}

	head := fmt.Sprintf("  %-*s  %-10s %-10s", width, "INPUT", "LOCKED", "UPSTREAM")
	out.line(s.paint(bold, strings.TrimRight(head, " ")))
	if rule := s.rule(width + 26); rule != "" {
		out.line(s.paint(dim, rule))
	}

	for _, in := range r.Inputs {
		if in.Err != nil {
			out.printf("%s %-*s  %s\n", s.marker(false), width, in.Name,
				s.paint(dim, "unreadable: "+lastLine(in.Err.Error())))
			continue
		}
		note := ""
		if in.Follows != "" {
			note = s.paint(dim, in.Follows)
		}
		// Pad before painting: escape bytes count toward %-10s but carry no
		// width on screen, so a painted revision would push the note left.
		up := revs.Short(in.Upstream)
		pad := strings.Repeat(" ", max(0, 10-len(up)))
		if in.Moved() {
			up = s.paint(green, up)
		}
		line := fmt.Sprintf("%s %-*s  %-10s %s%s %s",
			s.marker(in.Moved()), width, in.Name, revs.Short(in.Locked), up, pad, note)
		out.line(strings.TrimRight(line, " "))

		for _, p := range in.Pins {
			if !p.Moved() {
				continue
			}
			lead := "    "
			if s.Fancy {
				lead = "      \u2514 "
			}
			out.printf("%s%s %s %s %s\n", lead, s.paint(dim, "pins "+p.Name),
				revs.Short(p.Locked), s.arrow(), s.paint(green, revs.Short(p.Upstream)))
		}
	}

	if rule := s.rule(width + 26); rule != "" {
		out.line(s.paint(dim, rule))
	}
	moved := r.Moved()
	summary := fmt.Sprintf("%d of %d inputs moved", moved, len(r.Inputs))
	if s.Fancy {
		out.printf("%s %s\n", s.marker(moved > 0), summary)
	} else {
		out.printf("\n%s\n", summary)
	}
	return out.err
}

func lastLine(s string) string {
	parts := strings.Split(s, "\n")
	return strings.TrimSpace(parts[len(parts)-1])
}
