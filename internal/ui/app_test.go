package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"splunk-tui/internal/config"
	"splunk-tui/internal/splunk"
	"splunk-tui/internal/ui/browser"
)

func TestAppMultiTabWorkspaces(t *testing.T) {
	cfg := &config.Config{}
	app := NewApp(cfg, nil, "")

	// Resize app window
	model, _ := app.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	app = model.(AppModel)

	// Simulate successful connection to an instance
	client, err := splunk.NewClient(splunk.ClientConfig{BaseURL: "https://splunk.corp.local:8089"})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	bm := config.Bookmark{Name: "prod-cluster", URL: "https://splunk.corp.local:8089"}
	info := &splunk.ServerInfo{ServerName: "sh01", Version: "9.2.0"}

	connMsg := browser.ConnectedMsg{
		Bookmark: bm,
		Client:   client,
		Info:     info,
	}

	model, _ = app.Update(connMsg)
	app = model.(AppModel)

	if app.screen != screenSearch {
		t.Fatalf("expected screenSearch, got %v", app.screen)
	}

	// Should have 1 tab initially
	if len(app.searchTabs) != 1 {
		t.Fatalf("expected 1 search tab, got %d", len(app.searchTabs))
	}
	if app.activeTab != 0 {
		t.Fatalf("expected activeTab 0, got %d", app.activeTab)
	}

	// Tab bar should render 1 tab and new tab hint
	tabBar := app.renderTabBar(100)
	if !strings.Contains(tabBar, "1: Search") {
		t.Fatalf("expected tab bar to render '1: Search', got: %s", tabBar)
	}
	if !strings.Contains(tabBar, "+ New (Ctrl+T)") {
		t.Fatalf("expected tab bar to show new tab hint, got: %s", tabBar)
	}

	// Press Ctrl+T to open a new tab
	keyCtrlT := tea.KeyMsg{Type: tea.KeyCtrlT}
	model, _ = app.Update(keyCtrlT)
	app = model.(AppModel)

	if len(app.searchTabs) != 2 {
		t.Fatalf("expected 2 search tabs after Ctrl+T, got %d", len(app.searchTabs))
	}
	if app.activeTab != 1 {
		t.Fatalf("expected activeTab to be 1 (new tab), got %d", app.activeTab)
	}

	tabBar2 := app.renderTabBar(100)
	if !strings.Contains(tabBar2, "2: Search") {
		t.Fatalf("expected tab bar to show '2: Search', got: %s", tabBar2)
	}

	// Unfocus the search input with Esc so '[' and ']' navigate tabs rather than entering subsearch SPL
	model, _ = app.Update(tea.KeyMsg{Type: tea.KeyEscape})
	app = model.(AppModel)

	// Switch to previous tab using '['
	keyPrev := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'['}}
	model, _ = app.Update(keyPrev)
	app = model.(AppModel)
	if app.activeTab != 0 {
		t.Fatalf("expected activeTab to be 0 after '[', got %d", app.activeTab)
	}

	// Switch to next tab using ']'
	keyNext := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}}
	model, _ = app.Update(keyNext)
	app = model.(AppModel)
	if app.activeTab != 1 {
		t.Fatalf("expected activeTab to be 1 after ']', got %d", app.activeTab)
	}

	// Direct switch using Alt+1
	keyAlt1 := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}, Alt: true}
	model, _ = app.Update(keyAlt1)
	app = model.(AppModel)
	if app.activeTab != 0 {
		t.Fatalf("expected activeTab to be 0 after Alt+1, got %d", app.activeTab)
	}

	// Switch back using Alt+2
	model, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}, Alt: true})
	app = model.(AppModel)
	if app.activeTab != 1 {
		t.Fatalf("expected activeTab to be 1 after Alt+2, got %d", app.activeTab)
	}

	// Focus the search input again to verify Ctrl+W works even while input is marked/focused
	app.searchTabs[1].FocusSearch()
	if !app.isInputFocused() {
		t.Fatalf("expected input to be focused before Ctrl+W test")
	}

	// Close tab 1 with Ctrl+W while search input is focused
	keyCtrlW := tea.KeyMsg{Type: tea.KeyCtrlW}
	model, _ = app.Update(keyCtrlW)
	app = model.(AppModel)
	if len(app.searchTabs) != 1 {
		t.Fatalf("expected 1 search tab after Ctrl+W while input focused, got %d", len(app.searchTabs))
	}
	if app.activeTab != 0 {
		t.Fatalf("expected activeTab to be 0, got %d", app.activeTab)
	}

	// Attempting to close the only remaining tab with Ctrl+W should keep it open
	model, _ = app.Update(keyCtrlW)
	app = model.(AppModel)
	if len(app.searchTabs) != 1 {
		t.Fatalf("expected remaining tab to stay open, got %d tabs", len(app.searchTabs))
	}

	// Render view should include tab bar and search dashboard
	view := app.View()
	if !strings.Contains(view, "1: Search") {
		t.Fatalf("expected View to contain tab bar header, got: %s", view)
	}
}
