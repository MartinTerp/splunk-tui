package search

import (
	"fmt"
	"sort"
	"strings"

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
		case "pgup", "b":
			m.scroll -= 10
			if m.scroll < 0 {
				m.scroll = 0
			}
		case "pgdown", "f", "space":
			m.scroll += 10
		case "home", "g":
			m.scroll = 0
		case "end", "G":
			m.scroll = 999999
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

	rawBoxStyle := lipgloss.NewStyle().
		Width(boxWidth - 6).
		Foreground(m.theme.Text).
		Padding(0, 1)

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
			item := lipgloss.JoinHorizontal(lipgloss.Top, keyStyled, " ", valStyled)
			for _, l := range strings.Split(item, "\n") {
				lines = append(lines, l)
			}
		}

		// Raw section
		rawText := m.event.Raw()
		formattedRaw, isJSON := FormatRawJSON(rawText)

		var rawTitle string
		if isJSON {
			jsonBadge := lipgloss.NewStyle().
				Background(m.theme.Primary).
				Foreground(lipgloss.Color("#FFFFFF")).
				Bold(true).
				Padding(0, 1).
				Render("JSON")
			rawTitle = lipgloss.JoinHorizontal(lipgloss.Center, m.theme.Title.Render("Raw Event Content (_raw) "), jsonBadge)
			formattedRaw = HighlightJSON(formattedRaw, m.theme)
		} else {
			rawTitle = m.theme.Title.Render("Raw Event Content (_raw):")
			formattedRaw = HighlightLogText(formattedRaw, m.theme)
		}

		lines = append(lines, "", rawTitle)
		rawRendered := rawBoxStyle.Render(formattedRaw)
		for _, rl := range strings.Split(rawRendered, "\n") {
			lines = append(lines, rl)
		}
	} else if m.row != nil {
		var keys []string
		for k := range m.row {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		hasRaw := false
		var rawVal string
		for _, k := range keys {
			if k == "_raw" {
				hasRaw = true
				rawVal = m.row[k]
				continue
			}
			val := m.row[k]
			keyStyled := lipgloss.NewStyle().Width(18).Bold(true).Foreground(m.theme.Primary).Render(k)
			valStyled := lipgloss.NewStyle().Width(boxWidth - 24).Render(val)
			item := lipgloss.JoinHorizontal(lipgloss.Top, keyStyled, " ", valStyled)
			for _, l := range strings.Split(item, "\n") {
				lines = append(lines, l)
			}
		}

		if hasRaw {
			formattedRaw, isJSON := FormatRawJSON(rawVal)
			var rawTitle string
			if isJSON {
				jsonBadge := lipgloss.NewStyle().
					Background(m.theme.Primary).
					Foreground(lipgloss.Color("#FFFFFF")).
					Bold(true).
					Padding(0, 1).
					Render("JSON")
				rawTitle = lipgloss.JoinHorizontal(lipgloss.Center, m.theme.Title.Render("Raw Event Content (_raw) "), jsonBadge)
				formattedRaw = HighlightJSON(formattedRaw, m.theme)
			} else {
				rawTitle = m.theme.Title.Render("Raw Event Content (_raw):")
				formattedRaw = HighlightLogText(formattedRaw, m.theme)
			}

			lines = append(lines, "", rawTitle)
			rawRendered := rawBoxStyle.Render(formattedRaw)
			for _, rl := range strings.Split(rawRendered, "\n") {
				lines = append(lines, rl)
			}
		}
	}

	maxVisibleLines := boxHeight - 6
	if maxVisibleLines < 5 {
		maxVisibleLines = 5
	}

	maxScroll := len(lines) - maxVisibleLines
	if maxScroll < 0 {
		maxScroll = 0
	}
	if m.scroll > maxScroll {
		m.scroll = maxScroll
	}

	end := m.scroll + maxVisibleLines
	if end > len(lines) {
		end = len(lines)
	}
	visibleLines := lines[m.scroll:end]

	helpText := "↑/↓ scroll • esc or enter to close"
	if len(lines) > maxVisibleLines {
		helpText = fmt.Sprintf("↑/↓ scroll (%d-%d of %d) • esc or enter to close", m.scroll+1, end, len(lines))
	}
	help := lipgloss.NewStyle().Foreground(m.theme.SubtleText).Render(helpText)

	content := lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		"",
		lipgloss.JoinVertical(lipgloss.Left, visibleLines...),
		"",
		help,
	)

	card := m.theme.Card.Width(boxWidth).Height(boxHeight).MaxHeight(boxHeight).Render(content)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, card)
}
