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

// IncludeFieldMsg requests adding a field condition to the base search.
type IncludeFieldMsg struct {
	Field string
	Value string
}

// ExcludeFieldMsg requests adding an exclusion to the base search.
type ExcludeFieldMsg struct {
	Field string
	Value string
}

// ToggleColumnMsg toggles a field as an explicit column in the table.
type ToggleColumnMsg struct {
	Field string
}

type fieldItem struct {
	isField bool
	field   splunk.FieldSummary
	val     splunk.TopValue
}

// FieldsSidebarModel manages the list of extracted fields and their top values.
type FieldsSidebarModel struct {
	theme          theme.Theme
	items          []fieldItem
	expanded       map[string]bool
	selectedCols   map[string]bool
	cursor         int
	focused        bool
	width          int
	height         int
}

// NewFieldsSidebar creates a sidebar model.
func NewFieldsSidebar(th theme.Theme) FieldsSidebarModel {
	return FieldsSidebarModel{
		theme:        th,
		expanded:     make(map[string]bool),
		selectedCols: make(map[string]bool),
		width:        28,
	}
}

// Focus marks the sidebar as having keyboard focus.
func (m *FieldsSidebarModel) Focus() {
	m.focused = true
}

// Blur unsets keyboard focus.
func (m *FieldsSidebarModel) Blur() {
	m.focused = false
}

// IsFocused reports focus status.
func (m FieldsSidebarModel) IsFocused() bool {
	return m.focused
}

// SelectedColumns returns a list of fields toggled as table columns.
func (m FieldsSidebarModel) SelectedColumns() []string {
	var cols []string
	for c, active := range m.selectedCols {
		if active {
			cols = append(cols, c)
		}
	}
	sort.Strings(cols)
	return cols
}

// SetSummary updates the field stats, filtering out internal _* fields.
func (m *FieldsSidebarModel) SetSummary(summary *splunk.SummaryResponse) {
	if summary == nil {
		m.items = nil
		return
	}

	var nonInternal []splunk.FieldSummary
	for name, f := range summary.Fields {
		// Filter out internal fields starting with "_"
		if !strings.HasPrefix(name, "_") {
			nonInternal = append(nonInternal, f)
		}
	}

	// Sort fields by coverage descending, then by name
	sort.Slice(nonInternal, func(i, j int) bool {
		if nonInternal[i].Coverage == nonInternal[j].Coverage {
			return nonInternal[i].Name < nonInternal[j].Name
		}
		return nonInternal[i].Coverage > nonInternal[j].Coverage
	})

	m.rebuildItems(nonInternal)
}

func (m *FieldsSidebarModel) rebuildItems(fields []splunk.FieldSummary) {
	var items []fieldItem
	for _, f := range fields {
		items = append(items, fieldItem{isField: true, field: f})
		if m.expanded[f.Name] {
			for _, tv := range f.TopValues {
				items = append(items, fieldItem{isField: false, field: f, val: tv})
			}
		}
	}
	m.items = items
	if m.cursor >= len(m.items) && len(m.items) > 0 {
		m.cursor = len(m.items) - 1
	}
}

// SetDimensions updates dimensions.
func (m *FieldsSidebarModel) SetDimensions(w, h int) {
	m.width = w
	m.height = h
}

func (m FieldsSidebarModel) Update(msg tea.Msg) (FieldsSidebarModel, tea.Cmd) {
	if !m.focused || len(m.items) == 0 {
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
			return m, nil

		case "down", "j":
			if m.cursor < len(m.items)-1 {
				m.cursor++
			}
			return m, nil

		case "right", "enter":
			cur := m.items[m.cursor]
			if cur.isField {
				m.expanded[cur.field.Name] = !m.expanded[cur.field.Name]
				// Re-extract fields from current items
				var fields []splunk.FieldSummary
				seen := make(map[string]bool)
				for _, it := range m.items {
					if it.isField && !seen[it.field.Name] {
						fields = append(fields, it.field)
						seen[it.field.Name] = true
					}
				}
				m.rebuildItems(fields)
			}
			return m, nil

		case "+":
			cur := m.items[m.cursor]
			val := "*"
			if !cur.isField {
				val = cur.val.Value
			} else if len(cur.field.TopValues) > 0 {
				val = cur.field.TopValues[0].Value
			}
			return m, func() tea.Msg {
				return IncludeFieldMsg{Field: cur.field.Name, Value: val}
			}

		case "-":
			cur := m.items[m.cursor]
			val := "*"
			if !cur.isField {
				val = cur.val.Value
			} else if len(cur.field.TopValues) > 0 {
				val = cur.field.TopValues[0].Value
			}
			return m, func() tea.Msg {
				return ExcludeFieldMsg{Field: cur.field.Name, Value: val}
			}

		case "s":
			cur := m.items[m.cursor]
			fName := cur.field.Name
			m.selectedCols[fName] = !m.selectedCols[fName]
			return m, func() tea.Msg {
				return ToggleColumnMsg{Field: fName}
			}
		}
	}

	return m, nil
}

func (m FieldsSidebarModel) View() string {
	boxWidth := m.width - 4
	if boxWidth < 18 {
		boxWidth = 18
	}

	cardStyle := m.theme.Card
	if m.focused {
		cardStyle = m.theme.FocusedCard
	}

	header := m.theme.Title.Render("📋 Fields")
	helpLine := lipgloss.NewStyle().Foreground(m.theme.SubtleText).Render("+ inc • - exc • s col")

	availInner := m.height - 2
	if availInner < 3 {
		availInner = 3
	}

	if len(m.items) == 0 {
		empty := lipgloss.NewStyle().Foreground(m.theme.SubtleText).Italic(true).Render("No fields extracted")
		return cardStyle.Width(boxWidth).Height(availInner).MaxHeight(m.height).Render(lipgloss.JoinVertical(lipgloss.Left, header, "", empty))
	}

	maxVisible := availInner - 3
	if maxVisible <= 0 {
		maxVisible = 1
	}

	// Calculate scrolling window
	start := 0
	if m.cursor >= maxVisible {
		start = m.cursor - maxVisible + 1
	}
	end := start + maxVisible
	if end > len(m.items) {
		end = len(m.items)
	}

	var rows []string
	for i := start; i < end; i++ {
		it := m.items[i]
		isCur := i == m.cursor && m.focused

		if it.isField {
			expIcon := "▸ "
			if m.expanded[it.field.Name] {
				expIcon = "▾ "
			}

			colBadge := ""
			if m.selectedCols[it.field.Name] {
				colBadge = "✓"
			}

			covPct := fmt.Sprintf("%3.0f%%", it.field.Coverage*100)
			nameWidth := boxWidth - 10
			if nameWidth < 8 {
				nameWidth = 8
			}

			fName := truncate(it.field.Name, nameWidth)
			label := fmt.Sprintf("%s%-*s %s %s", expIcon, nameWidth, fName, covPct, colBadge)

			if isCur {
				rows = append(rows, m.theme.SidebarActive.Width(boxWidth-2).Render(label))
			} else {
				rows = append(rows, m.theme.SidebarItem.Width(boxWidth-2).Render(label))
			}
		} else {
			// Top value line
			valStr := truncate(it.val.Value, boxWidth-14)
			valLine := fmt.Sprintf("  • %-12s (%d)", valStr, it.val.Count)

			styled := lipgloss.NewStyle().Foreground(m.theme.SubtleText)
			if isCur {
				rows = append(rows, m.theme.SidebarActive.Width(boxWidth-2).Render(valLine))
			} else {
				rows = append(rows, styled.Width(boxWidth-2).Render(valLine))
			}
		}
	}

	content := lipgloss.JoinVertical(lipgloss.Left, header, helpLine, "", lipgloss.JoinVertical(lipgloss.Left, rows...))
	return cardStyle.Width(boxWidth).Height(availInner).MaxHeight(m.height).Render(content)
}
