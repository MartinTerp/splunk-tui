package search

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"splunk-tui/internal/splunk"
	"splunk-tui/internal/ui/theme"
)

func TestConfigExplorerModelNavigation(t *testing.T) {
	th := theme.DefaultTheme()
	cm := NewConfigExplorer(th, nil)
	cm.SetDimensions(120, 30)

	// Simulate loaded config files
	cm, _, _ = cm.Update(configFilesLoadedMsg{
		files: []string{"inputs", "props", "server", "transforms"},
	})

	if cm.loading {
		t.Fatalf("expected loading to be false after files loaded")
	}
	if len(cm.files) != 4 {
		t.Fatalf("expected 4 files, got %d", len(cm.files))
	}

	// Move cursor down
	cm, _, _ = cm.Update(tea.KeyMsg{Type: tea.KeyDown})
	if cm.fileCursor != 1 {
		t.Errorf("expected fileCursor to be 1, got %d", cm.fileCursor)
	}

	// Simulate loaded stanzas for "props"
	stanzas := []splunk.ConfigStanza{
		{
			Name: "syslog",
			Properties: map[string]string{
				"TIME_FORMAT": "%b %d %H:%M:%S",
				"SHOULD_LINEMERGE": "false",
			},
		},
		{
			Name: "json",
			Properties: map[string]string{
				"INDEXED_EXTRACTIONS": "json",
			},
		},
	}

	cm, _, _ = cm.Update(configStanzasLoadedMsg{
		confFile: "props",
		stanzas:  stanzas,
	})

	if cm.activePane != paneStanzas {
		t.Errorf("expected activePane to be paneStanzas after loading stanzas, got %v", cm.activePane)
	}

	// View should display props stanzas
	v := cm.View()
	if !strings.Contains(v, "syslog") || !strings.Contains(v, "json") {
		t.Errorf("expected view to contain stanzas, got:\n%s", v)
	}

	// Press right/enter to move to properties pane
	cm, _, _ = cm.Update(tea.KeyMsg{Type: tea.KeyRight})
	if cm.activePane != paneProperties {
		t.Errorf("expected activePane to be paneProperties after KeyRight, got %v", cm.activePane)
	}

	// Filter stanzas
	cm, _, _ = cm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	if !cm.filtering {
		t.Fatalf("expected filtering to be active after '/'")
	}
	cm, _, _ = cm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	cm, _, _ = cm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	filtered := cm.filteredStanzas()
	if len(filtered) != 1 || filtered[0].Name != "syslog" {
		t.Errorf("expected 1 filtered stanza 'syslog', got %d: %v", len(filtered), filtered)
	}

	// Esc exits filter
	cm, _, _ = cm.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cm.filtering {
		t.Errorf("expected filtering to be disabled after esc")
	}

	// Esc in properties moves back to stanzas
	cm, _, _ = cm.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cm.activePane != paneStanzas {
		t.Errorf("expected activePane to be paneStanzas after esc, got %v", cm.activePane)
	}

	// Esc in stanzas moves back to files
	cm, _, _ = cm.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cm.activePane != paneFiles {
		t.Errorf("expected activePane to be paneFiles after esc, got %v", cm.activePane)
	}

	// Esc in files closes the modal
	var closed bool
	cm, _, closed = cm.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if !closed {
		t.Errorf("expected closed=true on final esc")
	}
}

func TestConfigExplorerWindowing(t *testing.T) {
	th := theme.DefaultTheme()
	cm := NewConfigExplorer(th, nil)
	height := 24
	width := 100
	cm.SetDimensions(width, height)

	// Create 150 stanzas
	var stanzas []splunk.ConfigStanza
	for i := 0; i < 150; i++ {
		stanzas = append(stanzas, splunk.ConfigStanza{
			Name: strings.Repeat("stanza_", 2) + string(rune('a'+i%26)),
			Properties: map[string]string{
				"key1": "val1",
				"key2": "val2",
			},
		})
	}

	cm, _, _ = cm.Update(configStanzasLoadedMsg{
		confFile: "server",
		stanzas:  stanzas,
	})

	view := cm.View()
	lineCount := len(strings.Split(view, "\n"))
	if lineCount > height {
		t.Fatalf("expected view lines (%d) not to exceed terminal height (%d)", lineCount, height)
	}

	// Verify page down navigation works
	cm, _, _ = cm.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	if cm.stanzaCursor != 10 {
		t.Errorf("expected stanzaCursor to be 10 after PgDown, got %d", cm.stanzaCursor)
	}
}
