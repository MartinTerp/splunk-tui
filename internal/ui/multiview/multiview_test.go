package multiview

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"splunk-tui/internal/config"
	"splunk-tui/internal/spl"
	"splunk-tui/internal/ui/browser"
	"splunk-tui/internal/ui/search"
	"splunk-tui/internal/ui/theme"
)

func TestMultiViewTabNavigation(t *testing.T) {
	th := theme.DefaultTheme()
	conns := []browser.InstanceConnection{
		{Bookmark: config.Bookmark{Name: "idx1", URL: "https://splunk1:8089"}},
		{Bookmark: config.Bookmark{Name: "idx2", URL: "https://splunk2:8089"}},
	}
	mv := New(th, nil, conns)
	mv.SetDimensions(100, 30)

	if mv.focus != focusSearchSection {
		t.Fatalf("expected initial focus on search section")
	}
	if mv.searchBar.FocusSection() != search.FocusInput {
		t.Fatalf("expected initial searchbar focus on input")
	}

	// 1st Tab: advances search bar focus to From time picker!
	mv, _ = mv.Update(tea.KeyMsg{Type: tea.KeyTab})
	if mv.focus != focusSearchSection {
		t.Fatalf("expected focus to stay on search section on 1st tab")
	}
	if mv.searchBar.FocusSection() != search.FocusFromTime {
		t.Fatalf("expected searchbar focus on FocusFromTime on 1st tab, got %v", mv.searchBar.FocusSection())
	}

	// 2nd Tab: advances search bar focus to To time picker!
	mv, _ = mv.Update(tea.KeyMsg{Type: tea.KeyTab})
	if mv.focus != focusSearchSection {
		t.Fatalf("expected focus to stay on search section on 2nd tab")
	}
	if mv.searchBar.FocusSection() != search.FocusToTime {
		t.Fatalf("expected searchbar focus on FocusToTime on 2nd tab, got %v", mv.searchBar.FocusSection())
	}

	// 3rd Tab: moves from To time picker to Grid section!
	mv, _ = mv.Update(tea.KeyMsg{Type: tea.KeyTab})
	if mv.focus != focusGridSection {
		t.Fatalf("expected focus to advance to focusGridSection on 3rd tab, got %v", mv.focus)
	}

	// Tab from Grid section returns to Search section!
	mv, _ = mv.Update(tea.KeyMsg{Type: tea.KeyTab})
	if mv.focus != focusSearchSection {
		t.Fatalf("expected focus to return to focusSearchSection, got %v", mv.focus)
	}
	if mv.searchBar.FocusSection() != search.FocusInput {
		t.Fatalf("expected searchbar focus on input when returning from grid")
	}
}

func TestMultiViewCustomToTimeInSearch(t *testing.T) {
	th := theme.DefaultTheme()
	conns := []browser.InstanceConnection{
		{Bookmark: config.Bookmark{Name: "idx1", URL: "https://splunk1:8089"}},
	}
	mv := New(th, nil, conns)
	mv.SetDimensions(100, 30)

	// Set earliest to -7d and TO time to -1d
	mv.searchBar.SetTimeRange("-7d", "-1d")

	// View should display both bounds in the search bar
	v := mv.View()
	if !strings.Contains(v, "From: -7d") || !strings.Contains(v, "To: -1d") {
		t.Errorf("expected view to contain From: -7d and To: -1d, got:\n%s", v)
	}
}

func TestMultiViewChartTypeSwitching(t *testing.T) {
	th := theme.DefaultTheme()
	conns := []browser.InstanceConnection{
		{Bookmark: config.Bookmark{Name: "idx1", URL: "https://splunk1:8089"}},
		{Bookmark: config.Bookmark{Name: "idx2", URL: "https://splunk2:8089"}},
	}
	mv := New(th, nil, conns)
	mv.SetDimensions(120, 30)

	mv, _ = mv.RunSearch("index=_internal | timechart count by host")
	if mv.focus != focusGridSection {
		t.Fatalf("expected focus to be in grid section after running search")
	}
	if mv.cards[0].ActiveType() != spl.ChartLine {
		t.Fatalf("expected initial chart type to be ChartLine, got %v", mv.cards[0].ActiveType())
	}

	// 1st 'v': switches to ChartColumnTime (Bar)
	mv, _ = mv.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	if mv.cards[0].ActiveType() != spl.ChartColumnTime {
		t.Errorf("expected 1st 'v' to switch to ChartColumnTime, got %v", mv.cards[0].ActiveType())
	}
	if mv.cards[1].ActiveType() != spl.ChartColumnTime {
		t.Errorf("expected card 2 to also switch to ChartColumnTime, got %v", mv.cards[1].ActiveType())
	}

	// 2nd 'v': switches to ChartSparklines
	mv, _ = mv.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	if mv.cards[0].ActiveType() != spl.ChartSparklines {
		t.Errorf("expected 2nd 'v' to switch to ChartSparklines, got %v", mv.cards[0].ActiveType())
	}

	// 3rd 'v': wraps back to ChartLine
	mv, _ = mv.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	if mv.cards[0].ActiveType() != spl.ChartLine {
		t.Errorf("expected 3rd 'v' to wrap back to ChartLine, got %v", mv.cards[0].ActiveType())
	}
}

func TestMultiViewFullscreenMode(t *testing.T) {
	th := theme.DefaultTheme()
	conns := []browser.InstanceConnection{
		{Bookmark: config.Bookmark{Name: "idx1", URL: "https://splunk1:8089"}},
		{Bookmark: config.Bookmark{Name: "idx2", URL: "https://splunk2:8089"}},
	}
	mv := New(th, nil, conns)
	mv.SetDimensions(120, 30)
	mv, _ = mv.RunSearch("index=_internal | timechart count by host")

	if mv.IsFullScreen() {
		t.Fatalf("expected initial fullscreen to be false")
	}

	// Enter fullscreen via 'enter'
	mv, _ = mv.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !mv.IsFullScreen() {
		t.Fatalf("expected 'enter' in grid to activate fullscreen")
	}

	v := mv.View()
	if !strings.Contains(v, "Fullscreen: idx1") {
		t.Errorf("expected view to contain 'Fullscreen: idx1', got:\n%s", v)
	}

	// Switch instance in fullscreen via 'right'
	mv, _ = mv.Update(tea.KeyMsg{Type: tea.KeyRight})
	if mv.activeCard != 1 {
		t.Errorf("expected activeCard to be 1 after 'right', got %d", mv.activeCard)
	}
	v2 := mv.View()
	if !strings.Contains(v2, "Fullscreen: idx2") {
		t.Errorf("expected view to contain 'Fullscreen: idx2', got:\n%s", v2)
	}

	// Exit fullscreen via 'esc'
	mv, _ = mv.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if mv.IsFullScreen() {
		t.Fatalf("expected 'esc' to exit fullscreen")
	}

	// Enter fullscreen via 'f'
	mv, _ = mv.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	if !mv.IsFullScreen() {
		t.Fatalf("expected 'f' to activate fullscreen")
	}

	// Exit fullscreen via 'f'
	mv, _ = mv.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	if mv.IsFullScreen() {
		t.Fatalf("expected 'f' while in fullscreen to exit fullscreen")
	}
}
