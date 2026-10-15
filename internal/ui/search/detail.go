package search

import (
	"sort"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"splunk-tui/internal/splunk"
	"splunk-tui/internal/ui/theme"
)

// DetailModel displays an inspector modal for a single event.
type DetailModel struct {
	theme  theme.Theme
	event  splunk.Event
	row    map[string]string // for transforming results
	width  int
	height int
	scroll int
}

// NewDetail creates an event detail modal.
func NewDetail(th theme.Theme, event splunk.Event, row map[string]string) DetailModel {
	return DetailModel{
		theme: th,
		event: event,
		row:   row,
	}
}

func (m DetailModel) Update(msg tea.Msg) (DetailModel, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "enter", "q":
			return m, nil, true // close
		case "up", "k":
			if m.scroll > 0 {
				m.scroll--
			}
		case "down", "j":
			m.scroll++
		}
	}
	return m, nil, false
}

func (m DetailModel) View() string {
	boxWidth := m.width - 8
	if boxWidth < 40 {
		boxWidth = 40
	}
	boxHeight := m.height - 6
	if boxHeight < 15 {
		boxHeight = 15
	}

	title := m.theme.Title.Render("🔍 Event Inspector")
	var lines []string

	if m.event != nil {
		// Collect and sort fields
		var keys []string
		for k := range m.event {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		// Show fields list
		for _, k := range keys {
			if k == "_raw" {
				continue
			}
			val := m.event.FieldValue(k)
			keyStyled := lipgloss.NewStyle().Width(18).Bold(true).Foreground(m.theme.Primary).Render(k)
			valStyled := lipgloss.NewStyle().Width(boxWidth - 24).Render(val)
			lines = append(lines, lipgloss.JoinHorizontal(lipgloss.Top, keyStyled, " ", valStyled))
		}

		// Raw section
		lines = append(lines, "", m.theme.Title.Render("Raw Event Content (_raw):"))
		rawText := m.event.Raw()
		rawBox := lipgloss.NewStyle().
			Width(boxWidth - 6).
			Foreground(m.theme.Text).
			Background(lipgloss.Color("#16191E")).
			Padding(1).
			Render(rawText)
		lines = append(lines, rawBox)
	} else if m.row != nil {
		var keys []string
		for k := range m.row {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		for _, k := range keys {
			val := m.row[k]
			keyStyled := lipgloss.NewStyle().Width(18).Bold(true).Foreground(m.theme.Primary).Render(k)
			valStyled := lipgloss.NewStyle().Width(boxWidth - 24).Render(val)
			lines = append(lines, lipgloss.JoinHorizontal(lipgloss.Top, keyStyled, " ", valStyled))
		}
	}

	help := lipgloss.NewStyle().Foreground(m.theme.SubtleText).Render("↑/↓ scroll • esc or enter to close")

	// Apply scrolling
	visibleLines := lines
	if m.scroll < len(lines) {
		visibleLines = lines[m.scroll:]
	}

	content := lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		"",
		lipgloss.JoinVertical(lipgloss.Left, visibleLines...),
		"",
		help,
	)

	card := m.theme.Card.Width(boxWidth).Height(boxHeight).Render(content)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, card)
}
