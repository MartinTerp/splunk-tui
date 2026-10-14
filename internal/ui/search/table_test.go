package search

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
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

