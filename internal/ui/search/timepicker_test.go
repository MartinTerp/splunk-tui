package search

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"splunk-tui/internal/ui/theme"
)

func TestTimePickerEarliestAndLatest(t *testing.T) {
	th := theme.DefaultTheme()
	tp := NewTimePicker(th, "-4h")

	if tp.Earliest() != "-4h" {
		t.Fatalf("expected earliest '-4h', got %q", tp.Earliest())
	}
	if tp.Latest() != "now" {
		t.Fatalf("expected latest 'now', got %q", tp.Latest())
	}

	// Button rendering
	fromBtn := tp.ViewFromButton(false)
	if !strings.Contains(fromBtn, "From: -4h") {
		t.Errorf("expected ViewFromButton to contain 'From: -4h', got %q", fromBtn)
	}

	toBtn := tp.ViewToButton(false)
	if !strings.Contains(toBtn, "To: now") {
		t.Errorf("expected ViewToButton to contain 'To: now', got %q", toBtn)
	}

	// Change bounds
	tp.SetEarliest("-7d")
	tp.SetLatest("-1d")
	if tp.Earliest() != "-7d" || tp.Latest() != "-1d" {
		t.Errorf("expected -7d to -1d, got %s to %s", tp.Earliest(), tp.Latest())
	}

	sel := tp.Selected()
	if sel.Earliest != "-7d" || sel.Latest != "-1d" {
		t.Errorf("expected Selected() to return -7d and -1d, got %s and %s", sel.Earliest, sel.Latest)
	}
}

func TestTimePickerLatestDropdown(t *testing.T) {
	th := theme.DefaultTheme()
	tp := NewTimePicker(th, "-24h")

	// Open Latest picker
	tp.OpenLatest()
	if !tp.IsOpen() {
		t.Fatalf("expected picker to be open")
	}
	if tp.Target() != TargetLatest {
		t.Fatalf("expected target TargetLatest")
	}

	dropdown := tp.ViewDropdown()
	if !strings.Contains(dropdown, "Select Latest Time (To)") {
		t.Errorf("expected dropdown header for Latest, got:\n%s", dropdown)
	}

	// Navigate down to first preset (Now)
	tp, _, _ = tp.Update(tea.KeyMsg{Type: tea.KeyDown})
	if tp.selectedIndex != 0 {
		t.Fatalf("expected preset index 0, got %d", tp.selectedIndex)
	}

	// Navigate down to second preset (@d)
	tp, _, _ = tp.Update(tea.KeyMsg{Type: tea.KeyDown})
	if tp.selectedIndex != 1 {
		t.Fatalf("expected preset index 1, got %d", tp.selectedIndex)
	}

	// Select @d with enter
	tp, _, chosen := tp.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !chosen {
		t.Fatalf("expected chosen to be true")
	}
	if tp.IsOpen() {
		t.Fatalf("expected picker to close")
	}
	if tp.Latest() != "@d" {
		t.Errorf("expected latest '@d', got %q", tp.Latest())
	}
}

func TestTimePickerCustomInput(t *testing.T) {
	th := theme.DefaultTheme()
	tp := NewTimePicker(th, "-24h")

	// Open Latest and type custom time
	tp.OpenLatest()
	// Set custom value directly on input
	tp.customInput.SetValue("2026-10-12 18:00:00")
	// Press Enter to apply
	tp, _, chosen := tp.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !chosen {
		t.Fatalf("expected chosen to be true")
	}
	if tp.Latest() != "2026-10-12 18:00:00" {
		t.Errorf("expected custom latest, got %q", tp.Latest())
	}
}
