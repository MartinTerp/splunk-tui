package search

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"splunk-tui/internal/config"
	"splunk-tui/internal/history"
	"splunk-tui/internal/ui/theme"
)

func TestHistoryModalFilterAndSelect(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "historymodal-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	filePath := filepath.Join(tempDir, "history_boot.txt")
	_ = history.Append(filePath, "index=_internal | head 10")
	_ = history.Append(filePath, "index=main error")
	_ = history.Append(filePath, "index=_internal | stats count by sourcetype")

	th := theme.DefaultTheme()
	modal := NewHistoryModal(th, filePath, "boot")
	modal.SetDimensions(100, 24)

	// Verify loaded in recent-first order
	if len(modal.allEntries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(modal.allEntries))
	}
	if modal.allEntries[0] != "index=_internal | stats count by sourcetype" {
		t.Errorf("expected newest at index 0, got %q", modal.allEntries[0])
	}

	// Verify view contains title and entries
	view := modal.View()
	if !strings.Contains(view, "Search History") || !strings.Contains(view, "boot") {
		t.Errorf("expected view to contain header and instance name, got:\n%s", view)
	}
	if !strings.Contains(view, "index=main error") {
		t.Errorf("expected view to contain queries, got:\n%s", view)
	}

	// Press Down to select second query ("index=main error")
	var closed bool
	var selected string
	modal, _, closed, selected = modal.Update(tea.KeyMsg{Type: tea.KeyDown})
	if closed || selected != "" {
		t.Fatalf("down arrow should not close modal")
	}
	if modal.cursor != 1 {
		t.Fatalf("expected cursor 1 after down arrow, got %d", modal.cursor)
	}

	// Press Enter to select
	modal, _, closed, selected = modal.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !closed {
		t.Fatalf("expected modal to close on enter")
	}
	if selected != "index=main error" {
		t.Errorf("expected selected query 'index=main error', got %q", selected)
	}
}

func TestHistoryModalLiveFiltering(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "historymodal-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	filePath := filepath.Join(tempDir, "history_boot.txt")
	_ = history.Append(filePath, "index=_internal | head 10")
	_ = history.Append(filePath, "index=main error")
	_ = history.Append(filePath, "index=_audit action=login")

	th := theme.DefaultTheme()
	modal := NewHistoryModal(th, filePath, "boot")
	modal.SetDimensions(100, 24)

	// Filter by "audit"
	modal, _, _, _ = modal.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a', 'u', 'd', 'i', 't'}})
	if len(modal.filtered) != 1 {
		t.Fatalf("expected 1 match for 'audit', got %d: %v", len(modal.filtered), modal.filtered)
	}
	if modal.filtered[0] != "index=_audit action=login" {
		t.Errorf("expected match 'index=_audit action=login', got %q", modal.filtered[0])
	}

	// Press Enter to select filtered query
	_, _, closed, selected := modal.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !closed || selected != "index=_audit action=login" {
		t.Errorf("expected to select filtered query, got %q (closed=%v)", selected, closed)
	}

	// Test Esc cancels without selection
	modal = NewHistoryModal(th, filePath, "boot")
	_, _, closedEsc, selectedEsc := modal.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if !closedEsc || selectedEsc != "" {
		t.Errorf("expected cancel on esc, got closed=%v, selected=%q", closedEsc, selectedEsc)
	}
}

func TestSearchModelCtrlHHistoryTransfer(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "search-history-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	th := theme.DefaultTheme()
	cfg := &config.Config{
		PageSize: 50,
	}
	cfgPath := filepath.Join(tempDir, "config.yaml")
	_ = cfg.Save()
	cfg, _ = config.Load(cfgPath)

	bm := config.Bookmark{Name: "test-instance"}
	m := New(th, cfg, bm, nil, nil)
	m.SetDimensions(100, 24)

	// Pre-populate history file for this instance
	hPath := m.historyPath()
	_ = history.Append(hPath, "index=_internal sourcetype=splunkd | head 25")

	// Trigger Ctrl+H while focused on search input
	m.focus = focusSearch
	m.searchBar.SetFocusSection(FocusInput)

	var cmd tea.Cmd
	m, cmd = m.Update(tea.KeyMsg{Type: tea.KeyCtrlH})
	_ = cmd

	if m.historyModal == nil {
		t.Fatalf("expected historyModal to be open after Ctrl+H")
	}

	// Modal view should be rendered
	view := m.View()
	if !strings.Contains(view, "Search History") || !strings.Contains(view, "test-instance") {
		t.Errorf("expected modal in View(), got:\n%s", view)
	}

	// Press Enter in modal to transfer query to SPL search bar
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.historyModal != nil {
		t.Fatalf("expected historyModal to close after Enter")
	}

	if m.searchBar.Query() != "index=_internal sourcetype=splunkd | head 25" {
		t.Errorf("expected query transferred to search bar, got %q", m.searchBar.Query())
	}
}
