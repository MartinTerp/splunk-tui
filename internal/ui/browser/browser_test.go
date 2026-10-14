package browser

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"splunk-tui/internal/config"
	"splunk-tui/internal/ui/theme"
)

func TestBrowserFilterByNameAndURL(t *testing.T) {
	th := theme.DefaultTheme()
	cfg := &config.Config{
		Bookmarks: []config.Bookmark{
			{Name: "prod-us-east-1", URL: "https://splunk-east.corp:8089"},
			{Name: "prod-eu-west-1", URL: "https://splunk-eu.corp:8089"},
			{Name: "staging-cluster", URL: "https://staging.internal:8089"},
		},
	}

	b := New(th, cfg, nil)
	b.width = 100
	b.height = 30

	// Initially all 3 bookmarks present
	if len(b.filteredBookmarks()) != 3 {
		t.Fatalf("expected 3 bookmarks initially, got %d", len(b.filteredBookmarks()))
	}

	// Press '/' to start filtering
	b, _ = b.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	if !b.filtering {
		t.Fatalf("expected filtering to be active after '/'")
	}

	// Type "eu"
	b, _ = b.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	b, _ = b.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})

	filtered := b.filteredBookmarks()
	if len(filtered) != 1 || filtered[0].Name != "prod-eu-west-1" {
		t.Fatalf("expected 1 match 'prod-eu-west-1', got %d: %v", len(filtered), filtered)
	}

	// View should contain the filtered item and filter prompt
	view := b.View()
	if !strings.Contains(view, "prod-eu-west-1") {
		t.Errorf("expected view to contain 'prod-eu-west-1', got:\n%s", view)
	}
	if strings.Contains(view, "staging-cluster") {
		t.Errorf("expected view NOT to contain 'staging-cluster' when filtered, got:\n%s", view)
	}

	// Filter by URL: type "internal"
	b.filterInput.SetValue("internal")
	filteredURL := b.filteredBookmarks()
	if len(filteredURL) != 1 || filteredURL[0].Name != "staging-cluster" {
		t.Fatalf("expected 1 match for URL filter 'staging-cluster', got %d", len(filteredURL))
	}

	// Press 'esc' to clear filter
	b, _ = b.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if b.filtering {
		t.Fatalf("expected filtering to be disabled after esc")
	}
	if len(b.filteredBookmarks()) != 3 {
		t.Fatalf("expected all 3 bookmarks restored after clearing filter, got %d", len(b.filteredBookmarks()))
	}
}

func TestBrowserFooterDoesNotWrapOnNarrowTerminal(t *testing.T) {
	th := theme.DefaultTheme()
	cfg := &config.Config{
		Bookmarks: []config.Bookmark{},
	}

	widths := []int{60, 75, 85, 95, 120}
	for _, w := range widths {
		b := New(th, cfg, nil)
		b.width = w
		b.height = 24
		view := b.View()
		lines := strings.Split(view, "\n")
		// The bottom line should start with Ready or contain Ready, and should not have a second wrapped footer line
		bottomLine := lines[len(lines)-1]
		if strings.Contains(bottomLine, "quit") && !strings.Contains(bottomLine, "Ready") && len(lines) > 24 {
			t.Errorf("width %d: footer wrapped onto an extra line! Total lines: %d", w, len(lines))
		}
	}
}

func TestBrowserDividersAndRowsDoNotWrap(t *testing.T) {
	th := theme.DefaultTheme()
	cfg := &config.Config{
		Bookmarks: []config.Bookmark{
			{Name: "boot", URL: "https://192.168.4.104:8090", Username: "admin", VerifySSL: false},
			{Name: "test", URL: "https://192.168.4.104:8090", Username: "admin", VerifySSL: false},
		},
	}

	widths := []int{70, 80, 100, 120, 150}
	for _, w := range widths {
		b := New(th, cfg, nil)
		b.width = w
		b.height = 24
		view := b.View()

		// Verify no line starts with wrapped dashes "--" or "─"
		lines := strings.Split(view, "\n")
		for lineIdx, line := range lines {
			trimmed := strings.TrimSpace(line)
			// A wrapped divider artifact looks like a line with just 1-4 dashes
			if (strings.HasPrefix(trimmed, "--") || strings.HasPrefix(trimmed, "──")) && len(trimmed) <= 4 {
				t.Errorf("width %d, line %d: divider wrapped onto a new line: %q\nFull view:\n%s", w, lineIdx, line, view)
			}
		}

		// Verify bookmark names appear in view
		if !strings.Contains(view, "boot") || !strings.Contains(view, "test") {
			t.Errorf("width %d: expected bookmarks in view, got:\n%s", w, view)
		}
	}
}


