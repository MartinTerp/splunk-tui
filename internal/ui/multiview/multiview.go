package multiview

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"splunk-tui/internal/config"
	"splunk-tui/internal/history"
	"splunk-tui/internal/spl"
	"splunk-tui/internal/ui/browser"
	"splunk-tui/internal/ui/search"
	"splunk-tui/internal/ui/theme"
)

type focusSection int

const (
	focusSearchSection focusSection = iota
	focusGridSection
)

// MultiViewModel coordinates running a single SPL transforming search across multiple Splunk instances.
type MultiViewModel struct {
	theme         theme.Theme
	cfg           *config.Config
	searchBar     search.SearchBarModel
	cards         []InstanceCardModel
	activeCard    int
	isFullScreen  bool
	focus         focusSection
	grid          GridRenderer
	validationErr string
	historyModal  *search.HistoryModalModel
	width         int
	height        int
}

// New creates a MultiViewModel from established instance connections.
func New(th theme.Theme, cfg *config.Config, connections []browser.InstanceConnection) MultiViewModel {
	defaultTime := "-24h"
	if cfg != nil && cfg.DefaultTimeRange != "" {
		defaultTime = cfg.DefaultTimeRange
	}

	sb := search.NewSearchBar(th, defaultTime)

	var cards []InstanceCardModel
	for i, conn := range connections {
		card := NewInstanceCard(i, conn, th)
		cards = append(cards, card)
	}

	return MultiViewModel{
		theme:      th,
		cfg:        cfg,
		searchBar:  sb,
		cards:      cards,
		activeCard: 0,
		focus:      focusSearchSection,
		width:      80,
		height:     24,
	}
}

// Init initializes the multi view.
func (m MultiViewModel) Init() tea.Cmd {
	return nil
}

// SetDimensions updates dimensions and propagates them to child elements.
func (m *MultiViewModel) SetDimensions(w, h int) {
	m.width = w
	m.height = h
	m.searchBar.SetWidth(w)
}

// SearchBarIsFocused reports whether the search query input is active.
func (m MultiViewModel) SearchBarIsFocused() bool {
	return (m.focus == focusSearchSection && m.searchBar.IsFocused()) || m.historyModal != nil
}

func (m MultiViewModel) historyPath() string {
	dir := ""
	if m.cfg != nil {
		dir = m.cfg.Dir()
	}
	return history.FilePath(dir, "multiview")
}

// RunSearch dispatches the query across all instances if supported.
func (m MultiViewModel) RunSearch(query string) (MultiViewModel, tea.Cmd) {
	trimmed := strings.TrimSpace(query)
	if trimmed == "" {
		return m, nil
	}

	// Validate query against Multi View restrictions
	supported, _, _, reason := spl.IsMultiViewSupported(trimmed)
	if !supported {
		m.validationErr = reason
		return m, nil
	}

	m.validationErr = ""
	timeRange := m.searchBar.TimeRange()
	earliest := timeRange.Earliest
	latest := timeRange.Latest
	if qE, qL := spl.ExtractTimeBounds(trimmed); qE != "" || qL != "" {
		if qE != "" {
			earliest = qE
		}
		if qL != "" {
			latest = qL
		}
	}

	var cmds []tea.Cmd
	for i := range m.cards {
		cmd := m.cards[i].RunSearch(trimmed, earliest, latest)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}

	// Persist query to history file for multiview
	if hPath := m.historyPath(); hPath != "" {
		_ = history.Append(hPath, trimmed)
	}

	m.searchBar.Blur()
	m.isFullScreen = false
	m.focus = focusGridSection
	return m, tea.Batch(cmds...)
}

// IsFullScreen reports whether an instance card is currently maximized.
func (m MultiViewModel) IsFullScreen() bool {
	return m.isFullScreen
}

// SetFullScreen explicitly sets fullscreen mode.
func (m *MultiViewModel) SetFullScreen(fs bool) {
	m.isFullScreen = fs
}

// ToggleFullScreen toggles fullscreen mode on the active instance card.
func (m *MultiViewModel) ToggleFullScreen() {
	if len(m.cards) > 0 {
		m.isFullScreen = !m.isFullScreen
	}
}

// CycleChartType cycles the visualization type across all cards in sync.
func (m *MultiViewModel) CycleChartType() {
	if len(m.cards) == 0 {
		return
	}
	idx := m.activeCard
	if idx < 0 || idx >= len(m.cards) {
		idx = 0
	}
	m.cards[idx].CycleChartType()
	newType := m.cards[idx].ActiveType()
	for i := range m.cards {
		if i != idx {
			m.cards[i].SetActiveType(newType)
		}
	}
}

// CancelAll stops active search jobs across all instances.
func (m *MultiViewModel) CancelAll() {
	for i := range m.cards {
		m.cards[i].Cancel()
	}
}

// Update handles keyboard navigation, search bar input, and card polling results.
func (m MultiViewModel) Update(msg tea.Msg) (MultiViewModel, tea.Cmd) {
	// If history modal is open, delegate (except WindowSizeMsg which resizes both)
	if m.historyModal != nil {
		if _, isResize := msg.(tea.WindowSizeMsg); !isResize {
			var cmd tea.Cmd
			var closed bool
			var selectedQuery string
			*m.historyModal, cmd, closed, selectedQuery = m.historyModal.Update(msg)
			if closed {
				m.historyModal = nil
				if selectedQuery != "" {
					m.searchBar.SetQuery(selectedQuery)
					m.searchBar.Focus()
				}
			}
			return m, cmd
		}
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.SetDimensions(msg.Width, msg.Height)
		if m.historyModal != nil {
			m.historyModal.SetDimensions(msg.Width, msg.Height)
		}
		return m, nil

	case cardPollTickMsg:
		if msg.cardIndex >= 0 && msg.cardIndex < len(m.cards) {
			var cmd tea.Cmd
			m.cards[msg.cardIndex], cmd = m.cards[msg.cardIndex].Update(msg)
			return m, cmd
		}

	case CardPollResultMsg:
		if msg.CardIndex >= 0 && msg.CardIndex < len(m.cards) {
			var cmd tea.Cmd
			m.cards[msg.CardIndex], cmd = m.cards[msg.CardIndex].Update(msg)
			return m, cmd
		}

	case tea.KeyMsg:
		// Global shortcut: toggle search focus
		switch msg.String() {
		case "ctrl+f", "/":
			if m.focus != focusSearchSection {
				m.isFullScreen = false
				m.focus = focusSearchSection
				m.searchBar.Focus()
				return m, nil
			}

		case "ctrl+h":
			if m.focus == focusSearchSection && m.searchBar.FocusSection() == search.FocusInput {
				modal := search.NewHistoryModal(m.theme, m.historyPath(), "Multi View")
				modal.SetDimensions(m.width, m.height)
				m.historyModal = &modal
				return m, nil
			}

		case "v", "V":
			if m.focus != focusSearchSection && !m.searchBar.IsFocused() {
				m.CycleChartType()
				return m, nil
			}

		case "ctrl+x", "esc":
			if m.searchBar.HasDropdown() {
				break
			}
			if m.searchBar.IsFocused() {
				m.searchBar.Blur()
				m.focus = focusGridSection
				return m, nil
			}
			if m.isFullScreen {
				m.isFullScreen = false
				return m, nil
			}
			m.CancelAll()
			return m, nil

		case "tab":
			if m.focus == focusSearchSection {
				if m.searchBar.HasDropdown() {
					break
				}
				if m.searchBar.FocusSection() == search.FocusToTime {
					m.searchBar.Blur()
					m.focus = focusGridSection
					return m, nil
				}
				break
			} else {
				m.isFullScreen = false
				m.focus = focusSearchSection
				m.searchBar.Focus()
				return m, nil
			}

		case "shift+tab":
			if m.focus == focusSearchSection {
				if m.searchBar.HasDropdown() {
					break
				}
				if m.searchBar.FocusSection() == search.FocusInput {
					m.searchBar.Blur()
					m.focus = focusGridSection
					return m, nil
				}
				break
			} else {
				m.isFullScreen = false
				m.focus = focusSearchSection
				m.searchBar.SetFocusSection(search.FocusToTime)
				return m, nil
			}
		}
	}

	// Route based on focus
	if m.focus == focusSearchSection {
		var cmd tea.Cmd
		var submitted bool
		m.searchBar, cmd, submitted = m.searchBar.Update(msg)
		if submitted {
			return m.RunSearch(m.searchBar.Query())
		}
		return m, cmd
	}

	// Grid / Fullscreen focus: navigate cards, toggle fullscreen, cycle chart types
	switch msg := msg.(type) {
	case tea.KeyMsg:
		numCards := len(m.cards)
		if numCards == 0 {
			return m, nil
		}

		if m.isFullScreen {
			switch msg.String() {
			case "esc", "enter", "f", "F":
				m.isFullScreen = false
				return m, nil

			case "left", "h", "p":
				if m.activeCard > 0 {
					m.activeCard--
				} else {
					m.activeCard = numCards - 1
				}
				return m, nil

			case "right", "l", "n":
				if m.activeCard < numCards-1 {
					m.activeCard++
				} else {
					m.activeCard = 0
				}
				return m, nil

			case "v", "V":
				m.CycleChartType()
				return m, nil

			case "/", "ctrl+f":
				m.isFullScreen = false
				m.focus = focusSearchSection
				m.searchBar.Focus()
				return m, nil
			}
			return m, nil
		}

		switch msg.String() {
		case "enter", "f", "F":
			if m.activeCard >= 0 && m.activeCard < numCards {
				m.isFullScreen = true
				return m, nil
			}

		case "v", "V":
			m.CycleChartType()
			return m, nil

		case "up", "k":
			if m.activeCard >= 2 {
				m.activeCard -= 2
			}
			return m, nil

		case "down", "j":
			if m.activeCard+2 < numCards {
				m.activeCard += 2
			} else if m.activeCard%2 == 0 && m.activeCard+1 < numCards {
				m.activeCard++
			}
			return m, nil

		case "left", "h":
			if m.activeCard > 0 {
				m.activeCard--
			}
			return m, nil

		case "right", "l":
			if m.activeCard < numCards-1 {
				m.activeCard++
			}
			return m, nil
		}
	}

	return m, nil
}

// View renders the Multi View interface.
func (m MultiViewModel) View() string {
	if m.historyModal != nil {
		return m.historyModal.View()
	}

	w := m.width
	if w <= 0 {
		w = 80
	}
	h := m.height
	if h <= 0 {
		h = 24
	}

	innerW := w - 2
	if innerW < 20 {
		innerW = 20
	}

	// 1. Top Header Bar
	var headerTitle string
	if m.isFullScreen && m.activeCard >= 0 && m.activeCard < len(m.cards) {
		headerTitle = fmt.Sprintf("⚡ Splunk TUI  │  Multi View ⛶ Fullscreen: %s (%d/%d)", m.cards[m.activeCard].InstanceName(), m.activeCard+1, len(m.cards))
	} else {
		headerTitle = fmt.Sprintf("⚡ Splunk TUI  │  Multi View (%d instances)", len(m.cards))
	}
	var names []string
	for _, c := range m.cards {
		names = append(names, c.InstanceName())
	}
	clusterSummary := truncate(strings.Join(names, ", "), innerW/2)
	space := innerW - lipgloss.Width(headerTitle) - lipgloss.Width(clusterSummary)
	if space < 2 {
		space = 2
	}
	headerText := headerTitle + strings.Repeat(" ", space) + clusterSummary
	header := m.theme.Header.Width(innerW).MaxHeight(1).Render(headerText)

	// 2. Search Bar
	searchView := m.searchBar.View()
	if m.searchBar.HasDropdown() {
		dropdown := m.searchBar.ViewDropdown()
		searchView = lipgloss.JoinVertical(lipgloss.Left, searchView, dropdown)
	}

	// 3. Validation Banner (if query is invalid for Multi View)
	var warnBanner string
	warnH := 0
	if m.validationErr != "" {
		warnText := "⚠ " + m.validationErr + "\n" +
			"Example transforming queries:\n" +
			"  • index=_internal | timechart count by sourcetype\n" +
			"  • index=_internal | stats count, avg(cpu_usage) by host\n" +
			"  • index=_internal | chart count over component by log_level\n" +
			"  • index=_internal | table _time host sourcetype message"
		warnBanner = m.theme.Card.
			BorderForeground(m.theme.Warning).
			Width(w - 4).
			Render(lipgloss.NewStyle().Foreground(m.theme.Warning).Render(warnText))
		warnH = 7
	}

	// 4. Status Bar (bottom)
	statusBar := m.renderStatusBar(innerW)

	// 5. Card Grid or Fullscreen Card
	// Fixed height: Header (1) + SearchBar (3) + StatusBar (1) = 5
	fixedH := 5 + warnH
	availGridH := h - fixedH
	if availGridH < 8 {
		availGridH = 8
	}

	var gridView string
	if m.isFullScreen && m.activeCard >= 0 && m.activeCard < len(m.cards) {
		m.cards[m.activeCard].SetDimensions(w-4, availGridH)
		m.cards[m.activeCard].SetFocus(true)
		gridView = m.cards[m.activeCard].View()
	} else {
		activeIdx := -1
		if m.focus == focusGridSection {
			activeIdx = m.activeCard
		}
		gridView = m.grid.RenderGrid(m.cards, activeIdx, w, availGridH)
	}

	var views []string
	views = append(views, header, searchView)
	if warnBanner != "" {
		views = append(views, warnBanner)
	}
	views = append(views, gridView, statusBar)

	full := lipgloss.JoinVertical(lipgloss.Left, views...)
	return lipgloss.NewStyle().MaxHeight(h).Render(full)
}

func (m MultiViewModel) renderStatusBar(width int) string {
	if m.isFullScreen && m.activeCard >= 0 && m.activeCard < len(m.cards) {
		activeName := m.cards[m.activeCard].InstanceName()
		leftText := fmt.Sprintf("⛶ Fullscreen: %s (%d/%d)", activeName, m.activeCard+1, len(m.cards))
		leftStyled := lipgloss.NewStyle().Foreground(m.theme.Primary).Bold(true).Render(leftText)

		rightText := "esc/f/enter: exit fullscreen • v: chart • ←/→: instance • /: search • q: quit"
		rightStyled := lipgloss.NewStyle().Foreground(m.theme.SubtleText).Render(rightText)

		gap := width - lipgloss.Width(leftStyled) - lipgloss.Width(rightStyled)
		if gap < 1 {
			gap = 1
		}
		line := leftStyled + strings.Repeat(" ", gap) + rightStyled
		return m.theme.StatusBar.Width(width).MaxHeight(1).Render(line)
	}

	runningCount := 0
	doneCount := 0
	errCount := 0

	for _, c := range m.cards {
		if c.err != nil {
			errCount++
		} else if c.isSearching {
			runningCount++
		} else if c.jobStatus != nil && c.jobStatus.IsDone {
			doneCount++
		}
	}

	var leftText string
	if runningCount > 0 {
		leftText = fmt.Sprintf("● Running: %d/%d instances  │  ✔ %d done", runningCount, len(m.cards), doneCount)
	} else if errCount > 0 && doneCount > 0 {
		leftText = fmt.Sprintf("✔ %d done  │  ✖ %d failed", doneCount, errCount)
	} else if doneCount == len(m.cards) && len(m.cards) > 0 {
		leftText = fmt.Sprintf("✔ All %d instances complete", doneCount)
	} else {
		leftText = "Ready to search across all instances"
	}

	leftStyled := lipgloss.NewStyle().Foreground(m.theme.Highlight).Bold(true).Render(leftText)

	rightText := "? help • / search • v chart • enter/f fullscreen • ctrl+o switch • q quit"
	if runningCount > 0 {
		rightText = "esc: cancel search • ? help • q quit"
	}
	rightStyled := lipgloss.NewStyle().Foreground(m.theme.SubtleText).Render(rightText)

	gap := width - lipgloss.Width(leftStyled) - lipgloss.Width(rightStyled)
	if gap < 1 {
		gap = 1
	}

	line := leftStyled + strings.Repeat(" ", gap) + rightStyled
	return m.theme.StatusBar.Width(width).MaxHeight(1).Render(line)
}
