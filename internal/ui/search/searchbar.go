package search

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"splunk-tui/internal/ui/theme"
)

// FocusSection represents which interactive control in the SearchBar is currently focused.
type FocusSection int

const (
	FocusInput FocusSection = iota
	FocusFromTime
	FocusToTime
)

// SearchBarModel controls the SPL query input and both From and To time pickers.
type SearchBarModel struct {
	theme        theme.Theme
	input        textinput.Model
	timePicker   TimePickerModel
	focusSection FocusSection
	width        int
}

// NewSearchBar creates a search input bar with default earliest time.
func NewSearchBar(th theme.Theme, defaultEarliest string) SearchBarModel {
	ti := textinput.New()
	ti.Placeholder = "Enter SPL search (e.g. index=* | stats count by sourcetype)..."
	ti.Prompt = "▶ "
	ti.PromptStyle = lipgloss.NewStyle().Foreground(th.Primary).Bold(true)
	ti.Focus()

	tp := NewTimePicker(th, defaultEarliest)

	return SearchBarModel{
		theme:        th,
		input:        ti,
		timePicker:   tp,
		focusSection: FocusInput,
	}
}

// Focus focuses the SPL text input.
func (m *SearchBarModel) Focus() {
	m.focusSection = FocusInput
	m.input.Focus()
}

// Blur blurs the SPL text input and closes any active dropdown.
func (m *SearchBarModel) Blur() {
	m.input.Blur()
	m.timePicker.Close()
}

// FocusSection returns which section of the search bar is currently focused.
func (m SearchBarModel) FocusSection() FocusSection {
	return m.focusSection
}

// SetFocusSection updates the focused control inside the search bar.
func (m *SearchBarModel) SetFocusSection(fs FocusSection) {
	m.focusSection = fs
	if fs == FocusInput {
		m.input.Focus()
	} else {
		m.input.Blur()
	}
}

// IsFocused reports whether any search bar control or dropdown is active.
func (m SearchBarModel) IsFocused() bool {
	return m.input.Focused() || m.focusSection != FocusInput || m.timePicker.IsOpen()
}

// Query returns the current SPL query string.
func (m SearchBarModel) Query() string {
	return m.input.Value()
}

// SetQuery sets the SPL query string.
func (m *SearchBarModel) SetQuery(q string) {
	m.input.SetValue(q)
}

// TimeRange returns the currently selected time range bounds.
func (m SearchBarModel) TimeRange() TimeRange {
	return m.timePicker.Selected()
}

// Earliest returns the active earliest time string.
func (m SearchBarModel) Earliest() string {
	return m.timePicker.Earliest()
}

// Latest returns the active latest time string.
func (m SearchBarModel) Latest() string {
	return m.timePicker.Latest()
}

// SetTimeRange updates both earliest and latest boundaries.
func (m *SearchBarModel) SetTimeRange(earliest, latest string) {
	m.timePicker.SetTimeRange(earliest, latest)
}

// HasDropdown reports whether the time picker dropdown is open.
func (m SearchBarModel) HasDropdown() bool {
	return m.timePicker.IsOpen()
}

// ViewDropdown renders the open time picker dropdown.
func (m SearchBarModel) ViewDropdown() string {
	return m.timePicker.ViewDropdown()
}

// SetWidth updates layout dimensions.
func (m *SearchBarModel) SetWidth(w int) {
	m.width = w
	cardW := w - 4
	if cardW < 20 {
		cardW = 20
	}

	fromBtn := m.timePicker.ViewFromButton(m.focusSection == FocusFromTime)
	toBtn := m.timePicker.ViewToButton(m.focusSection == FocusToTime)
	btnsW := lipgloss.Width(fromBtn) + 1 + lipgloss.Width(toBtn)
	promptW := lipgloss.Width(m.input.Prompt)

	availInput := cardW - btnsW - promptW - 3
	if availInput < 3 {
		availInput = 3
	}
	m.input.Width = availInput
}

// Update handles keyboard navigation, tabbing between controls, and time picker dropdowns.
func (m SearchBarModel) Update(msg tea.Msg) (SearchBarModel, tea.Cmd, bool) {
	// If dropdown is open, time picker takes all input
	if m.timePicker.IsOpen() {
		var cmd tea.Cmd
		var changed bool
		m.timePicker, cmd, changed = m.timePicker.Update(msg)
		return m, cmd, changed
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "tab":
			switch m.focusSection {
			case FocusInput:
				m.focusSection = FocusFromTime
				m.input.Blur()
				return m, nil, false
			case FocusFromTime:
				m.focusSection = FocusToTime
				return m, nil, false
			case FocusToTime:
				m.focusSection = FocusInput
				m.input.Focus()
				return m, nil, false
			}

		case "shift+tab":
			switch m.focusSection {
			case FocusToTime:
				m.focusSection = FocusFromTime
				return m, nil, false
			case FocusFromTime:
				m.focusSection = FocusInput
				m.input.Focus()
				return m, nil, false
			case FocusInput:
				m.focusSection = FocusToTime
				m.input.Blur()
				return m, nil, false
			}

		case "ctrl+t", "ctrl+e":
			if m.focusSection == FocusToTime {
				m.timePicker.OpenLatest()
			} else {
				m.focusSection = FocusFromTime
				m.timePicker.OpenEarliest()
			}
			m.input.Blur()
			return m, nil, false

		case "enter":
			if m.focusSection == FocusFromTime {
				m.timePicker.OpenEarliest()
				return m, nil, false
			}
			if m.focusSection == FocusToTime {
				m.timePicker.OpenLatest()
				return m, nil, false
			}
			// FocusInput: submit search
			return m, nil, true

		case "esc":
			if m.focusSection != FocusInput {
				m.focusSection = FocusInput
				m.input.Focus()
				return m, nil, false
			}
		}
	}

	if m.focusSection == FocusInput {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd, false
	}

	return m, nil, false
}

// View renders the search bar with SPL input and both From and To buttons.
func (m SearchBarModel) View() string {
	cardW := m.width - 4
	if cardW < 20 {
		cardW = 20
	}

	fromBtn := m.timePicker.ViewFromButton(m.focusSection == FocusFromTime)
	toBtn := m.timePicker.ViewToButton(m.focusSection == FocusToTime)
	timeBtns := lipgloss.JoinHorizontal(lipgloss.Center, fromBtn, " ", toBtn)
	btnsW := lipgloss.Width(timeBtns)

	promptW := lipgloss.Width(m.input.Prompt)
	availInput := cardW - btnsW - promptW - 3
	if availInput < 3 {
		availInput = 3
	}
	m.input.Width = availInput

	inputView := m.input.View()
	remSpace := cardW - lipgloss.Width(inputView) - btnsW - 1
	if remSpace < 1 {
		remSpace = 1
	}
	bar := lipgloss.JoinHorizontal(lipgloss.Center, inputView, strings.Repeat(" ", remSpace), timeBtns)
	if lipgloss.Width(bar) > cardW {
		bar = truncate(bar, cardW)
	}

	cardStyle := m.theme.Card
	if m.input.Focused() {
		cardStyle = m.theme.FocusedCard
	}
	return cardStyle.Width(cardW).MaxHeight(3).Render(bar)
}
