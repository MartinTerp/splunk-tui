package search

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"splunk-tui/internal/splunk"
	"splunk-tui/internal/ui/theme"
)

func TestTableHorizontalScroll(t *testing.T) {
	th := theme.DefaultTheme()
	tbl := NewTable(th, 20)
	tbl.SetDimensions(80, 20)
	tbl.Focus()

	longRaw := "2026-10-13 14:00:00 [ERROR] com.example.service.AuthService: Failed authentication for user=admin " +
		"from ip=192.168.1.100 endpoint=/api/v1/auth/login reason=invalid_credentials attempts=5 response_time_ms=124 payload={\"action\":\"login\"}"

	events := []splunk.Event{
		{
			"_time": "2026-10-13 14:00:00",
			"_raw":  longRaw,
		},
	}

	tbl.SetEvents("index=main", events, 1, 1, nil)

	if tbl.HScroll() != 0 {
		t.Fatalf("expected initial HScroll = 0, got %d", tbl.HScroll())
	}

	// Press Right arrow
	tbl, _ = tbl.Update(tea.KeyMsg{Type: tea.KeyRight})
	if tbl.HScroll() != 20 {
		t.Errorf("expected HScroll = 20 after right, got %d", tbl.HScroll())
	}

	// Press Right arrow again
	tbl, _ = tbl.Update(tea.KeyMsg{Type: tea.KeyRight})
	if tbl.HScroll() != 40 {
		t.Errorf("expected HScroll = 40 after second right, got %d", tbl.HScroll())
	}

	// Check View contains indicator in header
	viewOutput := tbl.View()
	if !strings.Contains(viewOutput, "_raw (▶ +40)") {
		t.Errorf("expected view to contain '_raw (▶ +40)', got:\n%s", viewOutput)
	}

	// Press Left arrow
	tbl, _ = tbl.Update(tea.KeyMsg{Type: tea.KeyLeft})
	if tbl.HScroll() != 20 {
		t.Errorf("expected HScroll = 20 after left, got %d", tbl.HScroll())
	}

	// Press Home / 0 to reset
	tbl, _ = tbl.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'0'}})
	if tbl.HScroll() != 0 {
		t.Errorf("expected HScroll = 0 after '0', got %d", tbl.HScroll())
	}

	// Scroll with vim 'l' key
	tbl, _ = tbl.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	if tbl.HScroll() != 20 {
		t.Errorf("expected HScroll = 20 after 'l', got %d", tbl.HScroll())
	}

	// Scroll back with vim 'h' key
	tbl, _ = tbl.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	if tbl.HScroll() != 0 {
		t.Errorf("expected HScroll = 0 after 'h', got %d", tbl.HScroll())
	}

	// Jump to end with $
	tbl, _ = tbl.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'$'}})
	if tbl.HScroll() <= 0 {
		t.Errorf("expected HScroll > 0 after '$', got %d", tbl.HScroll())
	}

	// New search resets scroll
	tbl.SetEvents("index=main", events, 1, 1, nil)
	if tbl.HScroll() != 0 {
		t.Errorf("expected HScroll to reset on SetEvents, got %d", tbl.HScroll())
	}
}

func TestTablePaginationKeys(t *testing.T) {
	th := theme.DefaultTheme()
	tbl := NewTable(th, 20)
	tbl.SetDimensions(80, 20)
	tbl.Focus()

	// 1. KeyPgDown should trigger NextPageMsg
	_, cmd := tbl.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	if cmd == nil {
		t.Fatalf("expected command for KeyPgDown, got nil")
	}
	msg := cmd()
	if _, ok := msg.(NextPageMsg); !ok {
		t.Errorf("expected NextPageMsg from KeyPgDown, got %T", msg)
	}

	// 2. 'n' should trigger NextPageMsg
	_, cmd = tbl.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	if cmd == nil {
		t.Fatalf("expected command for 'n', got nil")
	}
	msg = cmd()
	if _, ok := msg.(NextPageMsg); !ok {
		t.Errorf("expected NextPageMsg from 'n', got %T", msg)
	}

	// 3. KeyPgUp should trigger PrevPageMsg
	_, cmd = tbl.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	if cmd == nil {
		t.Fatalf("expected command for KeyPgUp, got nil")
	}
	msg = cmd()
	if _, ok := msg.(PrevPageMsg); !ok {
		t.Errorf("expected PrevPageMsg from KeyPgUp, got %T", msg)
	}

	// 4. 'p' should trigger PrevPageMsg
	_, cmd = tbl.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	if cmd == nil {
		t.Fatalf("expected command for 'p', got nil")
	}
	msg = cmd()
	if _, ok := msg.(PrevPageMsg); !ok {
		t.Errorf("expected PrevPageMsg from 'p', got %T", msg)
	}
}

func TestTableErrorMessage(t *testing.T) {
	th := theme.DefaultTheme()
	tbl := NewTable(th, 20)
	tbl.SetDimensions(80, 20)

	tbl.SetError("Error in 'stats' command: The argument 'coun' is invalid.")
	view := tbl.View()

	if !strings.Contains(view, "Search Error") {
		t.Errorf("expected view to contain 'Search Error', got:\n%s", view)
	}
	if !strings.Contains(view, "The argument 'coun' is invalid") {
		t.Errorf("expected view to contain error text, got:\n%s", view)
	}

	// Setting valid events clears the error
	tbl.SetEvents("index=main", []splunk.Event{{"_raw": "test"}}, 1, 1, nil)
	viewAfter := tbl.View()
	if strings.Contains(viewAfter, "Search Error") {
		t.Errorf("expected error to be cleared after SetEvents")
	}
}

func TestTableFullTimeWidthAndSpacing(t *testing.T) {
	th := theme.DefaultTheme()
	tbl := NewTable(th, 20)
	tbl.SetDimensions(120, 20)

	fullTimestamp := "2026-10-15T10:59:39.000+02:00"
	events := []splunk.Event{
		{
			"_time": fullTimestamp,
			"_raw":  "audit log message from host=app01",
		},
	}

	tbl.SetEvents("index=main", events, 1, 1, nil)
	view := tbl.View()

	// Verify the full 29-character timestamp appears completely without being truncated
	if !strings.Contains(view, fullTimestamp) {
		t.Errorf("expected view to contain full timestamp %q, got:\n%s", fullTimestamp, view)
	}

	// Verify spacing between columns is at least 4 spaces
	if !strings.Contains(view, "    ") {
		t.Errorf("expected view to contain column spacing with at least 4 spaces")
	}
}

func TestTableSingleLinePerEvent(t *testing.T) {
	th := theme.DefaultTheme()
	tbl := NewTable(th, 20)
	tbl.SetDimensions(100, 20)

	longRaw := "2026-10-15 11:30:00 [ERROR] " + strings.Repeat("Very long event log payload with multiple parameters and data. ", 10)
	events := []splunk.Event{
		{"_time": "2026-10-15 11:30:00.000", "_raw": longRaw},
		{"_time": "2026-10-15 11:31:00.000", "_raw": "Short event 2"},
		{"_time": "2026-10-15 11:32:00.000", "_raw": "Line with extra newline\r\nand more content " + longRaw},
	}

	tbl.SetEvents("index=main", events, 1, 1, nil)
	view := tbl.View()

	lines := strings.Split(view, "\n")
	if len(lines) != 20 {
		t.Fatalf("expected view to have exactly 20 lines for height 20, got %d", len(lines))
	}

	for i, l := range lines {
		w := lipgloss.Width(l)
		if w != 100 {
			t.Errorf("line %d width = %d, expected 100 (content wrapped or misaligned): %q", i, w, l)
		}
	}

	// Line 1: Header row with _time and _raw
	if !strings.Contains(lines[1], "_time") || !strings.Contains(lines[1], "_raw") {
		t.Errorf("line 1 should be header line, got: %s", lines[1])
	}

	// Line 2: Divider line
	if !strings.Contains(lines[2], "───") {
		t.Errorf("line 2 should be divider line, got: %s", lines[2])
	}

	// Line 3: First event (exactly one line)
	if !strings.Contains(lines[3], "2026-10-15 11:30:00.000") || !strings.Contains(lines[3], "Very long event") {
		t.Errorf("line 3 should contain first event on a single line, got: %s", lines[3])
	}

	// Line 4: Second event (exactly one line)
	if !strings.Contains(lines[4], "2026-10-15 11:31:00.000") || !strings.Contains(lines[4], "Short event 2") {
		t.Errorf("line 4 should contain second event on a single line, got: %s", lines[4])
	}

	// Line 5: Third event (newline sanitized, exactly one line)
	if !strings.Contains(lines[5], "2026-10-15 11:32:00.000") || !strings.Contains(lines[5], "with extra newline") {
		t.Errorf("line 5 should contain third event on a single line, got: %s", lines[5])
	}

	// Line 6 should be empty space inside the card (no line wrapping overflow from events)
	if strings.Contains(lines[6], "ERROR") || strings.Contains(lines[6], "Short") {
		t.Errorf("line 6 should be empty inside card padding, got: %s", lines[6])
	}

	// Test horizontal scroll (arrow right)
	tbl.hScroll = 15
	scrolledView := tbl.View()
	scrolledLines := strings.Split(scrolledView, "\n")
	if len(scrolledLines) != 20 {
		t.Fatalf("scrolled view expected 20 lines, got %d", len(scrolledLines))
	}
	if !strings.Contains(scrolledLines[1], "▶ +15") {
		t.Errorf("header should indicate horizontal scroll +15, got: %s", scrolledLines[1])
	}
	if !strings.Contains(scrolledLines[3], "…") {
		t.Errorf("scrolled event should start with ellipsis, got: %s", scrolledLines[3])
	}
}

