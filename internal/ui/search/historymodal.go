package search

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"splunk-tui/internal/history"
	"splunk-tui/internal/ui/theme"
)

// HistoryModalModel displays an interactive searchable modal for past SPL queries.
type HistoryModalModel struct {
	theme        theme.Theme
	instanceName string
	filePath     string
	allEntries   []string
	filtered     []string
	cursor       int
	filterInput  textinput.Model
	width        int
	height       int
}

// NewHistoryModal creates a new SPL search history modal for the given instance.
func NewHistoryModal(th theme.Theme, filePath, instanceName string) HistoryModalModel {
	entries, _ := history.LoadRecentFirst(filePath)

	ti := textinput.New()
	ti.Prompt = "🔍 "
	ti.PromptStyle = lipgloss.NewStyle().Foreground(th.Primary).Bold(true)
	ti.Placeholder = "Type to filter history (or ↑/↓ to navigate, Enter to select)..."
	ti.Focus()

	return HistoryModalModel{
		theme:        th,
		instanceName: instanceName,
		filePath:     filePath,
		allEntries:   entries,
		filtered:     entries,
		cursor:       0,
		filterInput:  ti,
		width:        80,
		height:       24,
	}
}

// SetDimensions updates the allocated window dimensions for centering.
func (m *HistoryModalModel) SetDimensions(w, h int) {
	m.width = w
	m.height = h
}

// Update processes keyboard navigation, real-time filtering, and query selection.
// It returns (model, cmd, closed, selectedQuery).
func (m HistoryModalModel) Update(msg tea.Msg) (HistoryModalModel, tea.Cmd, bool, string) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			return m, nil, true, ""

		case "enter":
			if len(m.filtered) > 0 && m.cursor >= 0 && m.cursor < len(m.filtered) {
				return m, nil, true, m.filtered[m.cursor]
			}
			if len(m.filtered) == 0 && strings.TrimSpace(m.filterInput.Value()) != "" {
				return m, nil, true, strings.TrimSpace(m.filterInput.Value())
			}
			return m, nil, true, ""

		case "up", "ctrl+p":
			if m.cursor > 0 {
				m.cursor--
			}
			return m, nil, false, ""

		case "down", "ctrl+n":
			if m.cursor < len(m.filtered)-1 {
				m.cursor++
			}
			return m, nil, false, ""

		case "pgup":
			m.cursor -= 5
			if m.cursor < 0 {
				m.cursor = 0
			}
			return m, nil, false, ""

		case "pgdown":
			m.cursor += 5
			if m.cursor >= len(m.filtered) {
				m.cursor = len(m.filtered) - 1
			}
			if m.cursor < 0 {
				m.cursor = 0
			}
			return m, nil, false, ""
		}
	}

	// Route text typing to filterInput
	var cmd tea.Cmd
	oldVal := m.filterInput.Value()
	m.filterInput, cmd = m.filterInput.Update(msg)

	if m.filterInput.Value() != oldVal {
		m.applyFilter()
	}

	return m, cmd, false, ""
}

func (m *HistoryModalModel) applyFilter() {
	query := strings.TrimSpace(strings.ToLower(m.filterInput.Value()))
	if query == "" {
		m.filtered = m.allEntries
	} else {
		var matches []string
		for _, item := range m.allEntries {
			if strings.Contains(strings.ToLower(item), query) {
				matches = append(matches, item)
			}
		}
		m.filtered = matches
	}

	if m.cursor >= len(m.filtered) {
		m.cursor = 0
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

// View renders the centered history modal card.
func (m HistoryModalModel) View() string {
	w := m.width
	if w <= 0 {
		w = 80
	}
	h := m.height
	if h <= 0 {
		h = 24
	}

	boxW := w - 8
	if boxW > 110 {
		boxW = 110
	}
	if boxW < 45 {
		boxW = 45
	}

	boxH := h - 4
	if boxH > 22 {
		boxH = 22
	}
	if boxH < 10 {
		boxH = 10
	}

	contentW := boxW - 4
	if contentW < 20 {
		contentW = 20
	}

	// 1. Title bar
	instName := m.instanceName
	if instName == "" {
		instName = "current instance"
	}
	titleStr := fmt.Sprintf("📜 Search History  •  %s", instName)
	title := m.theme.Title.Bold(true).Render(titleStr)

	// 2. Filter input line
	m.filterInput.Width = contentW - 4
	filterLine := m.filterInput.View()

	// 3. Dividers
	divider := lipgloss.NewStyle().Foreground(m.theme.SurfaceBorder).Render(strings.Repeat("─", contentW))

	// 4. History item rows
	listH := boxH - 8
	if listH < 2 {
		listH = 2
	}

	var itemRows []string
	if len(m.allEntries) == 0 {
		emptyMsg := lipgloss.NewStyle().Foreground(m.theme.SubtleText).Italic(true).
			Render(fmt.Sprintf("  No search history recorded yet for instance %q.", instName))
		itemRows = append(itemRows, emptyMsg)
	} else if len(m.filtered) == 0 {
		noMatchMsg := lipgloss.NewStyle().Foreground(m.theme.SubtleText).Italic(true).
			Render(fmt.Sprintf("  No history matching %q.", m.filterInput.Value()))
		itemRows = append(itemRows, noMatchMsg)
	} else {
		start := 0
		if m.cursor >= listH {
			start = m.cursor - listH + 1
		}
		end := start + listH
		if end > len(m.filtered) {
			end = len(m.filtered)
		}

		for i := start; i < end; i++ {
			isCur := i == m.cursor
			item := m.filtered[i]
			textMaxW := contentW - 4
			if textMaxW < 10 {
				textMaxW = 10
			}
			truncated := truncate(item, textMaxW)

			if isCur {
				cursorStyled := lipgloss.NewStyle().Foreground(m.theme.Primary).Bold(true).Render("▶ ")
				textStyled := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Bold(true).Render(truncated)
				itemRows = append(itemRows, cursorStyled+textStyled)
			} else {
				textStyled := lipgloss.NewStyle().Foreground(m.theme.Text).Render(truncated)
				itemRows = append(itemRows, "  "+textStyled)
			}
		}
	}

	// Pad empty space to keep modal height fixed
	for len(itemRows) < listH {
		itemRows = append(itemRows, "")
	}

	listBlock := strings.Join(itemRows, "\n")

	// 5. Footer status & shortcuts
	footerLeft := lipgloss.NewStyle().Foreground(m.theme.SubtleText).Render("Enter: select query  •  ↑/↓: navigate  •  Esc: cancel")
	var footerRight string
	if len(m.filtered) > 0 {
		footerRight = lipgloss.NewStyle().Foreground(m.theme.Highlight).
			Render(fmt.Sprintf("[%d/%d]", m.cursor+1, len(m.filtered)))
	} else if len(m.allEntries) > 0 {
		footerRight = lipgloss.NewStyle().Foreground(m.theme.SubtleText).
			Render("[0 matches]")
	}
	footerGap := contentW - lipgloss.Width(footerLeft) - lipgloss.Width(footerRight)
	if footerGap < 1 {
		footerGap = 1
	}
	footerLine := footerLeft + strings.Repeat(" ", footerGap) + footerRight

	// Assemble card body
	body := lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		divider,
		filterLine,
		divider,
		listBlock,
		divider,
		footerLine,
	)

	card := m.theme.Card.Width(boxW).Height(boxH).Render(body)
	return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, card)
}
