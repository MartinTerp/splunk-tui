package multiview

import "github.com/charmbracelet/x/ansi"

// truncate shortens s to at most max terminal cells, appending "…" when cut.
// It is ANSI- and Unicode-aware, so it is safe on lipgloss-styled strings.
func truncate(s string, max int) string {
	if max <= 0 {
		return ""
	}
	if ansi.StringWidth(s) <= max {
		return s
	}
	if max == 1 {
		return ansi.Truncate(s, 1, "")
	}
	return ansi.Truncate(s, max, "…")
}
