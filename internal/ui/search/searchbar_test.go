package search

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"splunk-tui/internal/ui/theme"
)

func TestSearchBarTabNavigation(t *testing.T) {
	th := theme.DefaultTheme()
	sb := NewSearchBar(th, "-24h")
	sb.SetWidth(80)

	if sb.FocusSection() != FocusInput {
		t.Fatalf("expected initial focus FocusInput")
	}

	// 1st Tab -> moves to FocusFromTime
	sb, _, _ = sb.Update(tea.KeyMsg{Type: tea.KeyTab})
	if sb.FocusSection() != FocusFromTime {
		t.Fatalf("expected focus FocusFromTime after 1st tab, got %v", sb.FocusSection())
	}

	// 2nd Tab -> moves to FocusToTime
	sb, _, _ = sb.Update(tea.KeyMsg{Type: tea.KeyTab})
	if sb.FocusSection() != FocusToTime {
		t.Fatalf("expected focus FocusToTime after 2nd tab, got %v", sb.FocusSection())
	}

	// Shift+Tab -> moves back to FocusFromTime
	sb, _, _ = sb.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	if sb.FocusSection() != FocusFromTime {
		t.Fatalf("expected focus FocusFromTime after shift+tab, got %v", sb.FocusSection())
	}

	// Shift+Tab -> moves back to FocusInput
	sb, _, _ = sb.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	if sb.FocusSection() != FocusInput {
		t.Fatalf("expected focus FocusInput after shift+tab, got %v", sb.FocusSection())
	}
}

func TestSearchBarViewHasFromAndTo(t *testing.T) {
	th := theme.DefaultTheme()
	sb := NewSearchBar(th, "-24h")
	sb.SetWidth(80)

	view := sb.View()
	if !strings.Contains(view, "From: -24h") {
		t.Errorf("expected view to contain 'From: -24h', got:\n%s", view)
	}
	if !strings.Contains(view, "To: now") {
		t.Errorf("expected view to contain 'To: now', got:\n%s", view)
	}

	// Changing time bounds reflects in view
	sb.SetTimeRange("-7d", "-1d")
	view2 := sb.View()
	if !strings.Contains(view2, "From: -7d") || !strings.Contains(view2, "To: -1d") {
		t.Errorf("expected updated From: -7d and To: -1d, got:\n%s", view2)
	}
}
