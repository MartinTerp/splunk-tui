package search

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"splunk-tui/internal/spl"
	"splunk-tui/internal/splunk"
	"splunk-tui/internal/ui/theme"
)

// OpenDetailMsg requests opening the inspector for the selected row.
type OpenDetailMsg struct {
	Event splunk.Event
	Row   map[string]string
}

// NextPageMsg requests loading the next page of results.
type NextPageMsg struct{}

// PrevPageMsg requests loading the previous page of results.
type PrevPageMsg struct{}

// TableModel renders paginated events or transforming results.
type TableModel struct {
	theme         theme.Theme
	events        []splunk.Event
	results       []map[string]string
	resultFields  []string
	query         string
	selectedCols  []string
	isTransforming bool
	isTableCommand bool
	tableCols     []string
	cursor        int
	focused       bool
	page          int
	pageSize      int
	totalCount    int64
	hScroll       int
	colOffset     int
	width         int
	height        int
	errMsg        string
}

// NewTable creates a table model.
func NewTable(th theme.Theme, pageSize int) TableModel {
	if pageSize <= 0 {
		pageSize = 50
	}
	return TableModel{
		theme:    th,
		pageSize: pageSize,
		page:     1,
	}
}

// Focus marks the table as focused.
func (m *TableModel) Focus() {
	m.focused = true
}

// Blur unsets focus.
func (m *TableModel) Blur() {
	m.focused = false
}

// IsFocused reports whether table is focused.
func (m TableModel) IsFocused() bool {
	return m.focused
}

// SetEvents updates the table with events from an events search.
func (m *TableModel) SetEvents(query string, events []splunk.Event, total int64, page int, selectedCols []string) {
	m.errMsg = ""
	m.query = query
	m.events = events
	m.results = nil
	m.totalCount = total
	m.page = page
	m.selectedCols = selectedCols
	m.isTransforming = false
	m.isTableCommand = spl.HasTableCommand(query)
	if m.isTableCommand {
		m.tableCols = spl.ExtractTableColumns(query)
	}
	m.hScroll = 0
	m.colOffset = 0
	if m.cursor >= len(m.events) && len(m.events) > 0 {
		m.cursor = len(m.events) - 1
	}
}

// SetResults updates the table with records from a transforming search.
func (m *TableModel) SetResults(query string, res *splunk.ResultsResponse, total int64, page int) {
	m.errMsg = ""
	m.query = query
	m.events = nil
	m.totalCount = total
	m.page = page
	m.isTransforming = true
	m.isTableCommand = spl.HasTableCommand(query)
	if m.isTableCommand {
		m.tableCols = spl.ExtractTableColumns(query)
	}
	m.hScroll = 0
	m.colOffset = 0

	if res != nil {
		m.results = res.Results
		m.resultFields = res.Fields
	} else {
		m.results = nil
		m.resultFields = nil
	}

	if m.cursor >= len(m.results) && len(m.results) > 0 {
		m.cursor = len(m.results) - 1
	}
}

// SetError sets the error message to display in the table area.
func (m *TableModel) SetError(errStr string) {
	m.errMsg = errStr
	m.events = nil
	m.results = nil
	m.totalCount = 0
	m.cursor = 0
}

// ClearError clears any displayed error.
func (m *TableModel) ClearError() {
	m.errMsg = ""
}

// IsTransforming reports whether the table is displaying transforming or tabular results.
func (m TableModel) IsTransforming() bool {
	return m.isTransforming
}

// SetDimensions updates dimensions.
func (m *TableModel) SetDimensions(w, h int) {
	m.width = w
	m.height = h
}

// HScroll returns the current horizontal character scroll offset.
func (m TableModel) HScroll() int {
	return m.hScroll
}

// ColOffset returns the current column offset.
func (m TableModel) ColOffset() int {
	return m.colOffset
}

func (m TableModel) maxContentLength() int {
	maxLen := 0
	if m.isTransforming {
		for _, r := range m.results {
			for _, v := range r {
				if l := len([]rune(v)); l > maxLen {
					maxLen = l
				}
			}
		}
	} else {
		for _, ev := range m.events {
			if l := len([]rune(ev.Raw())); l > maxLen {
				maxLen = l
			}
		}
	}
	return maxLen
}

func (m TableModel) maxScroll(rawWidth int) int {
	maxLen := m.maxContentLength()
	if rawWidth <= 0 {
		rawWidth = 40
	}
	max := maxLen - rawWidth + 10
	if max < 0 {
		max = 0
	}
	return max
}

func (m TableModel) Update(msg tea.Msg) (TableModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if isNextPageKey(msg) || msg.String() == "n" || msg.String() == "N" {
			m.hScroll = 0
			m.colOffset = 0
			return m, func() tea.Msg { return NextPageMsg{} }
		}
		if isPrevPageKey(msg) || msg.String() == "p" || msg.String() == "P" {
			m.hScroll = 0
			m.colOffset = 0
			return m, func() tea.Msg { return PrevPageMsg{} }
		}

		if !m.focused {
			return m, nil
		}

		rowCount := len(m.events)
		if m.isTransforming {
			rowCount = len(m.results)
		}

		switch msg.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
			return m, nil

		case "down", "j":
			if m.cursor < rowCount-1 {
				m.cursor++
			}
			return m, nil

		case "right", "l":
			if m.isTransforming && len(m.resultFields) > 0 {
				totalCols := len(m.resultFields)
				if m.isTableCommand && len(m.tableCols) > 0 {
					totalCols = len(m.tableCols)
				}
				visCols := (m.width - 4) / 14
				if visCols < 2 {
					visCols = 2
				}
				if totalCols > visCols && m.colOffset < totalCols-visCols {
					m.colOffset++
					return m, nil
				}
			}

			maxScroll := m.maxScroll(m.width - 24)
			if m.hScroll < maxScroll {
				m.hScroll += 20
				if m.hScroll > maxScroll {
					m.hScroll = maxScroll
				}
			}
			return m, nil

		case "shift+right":
			if m.isTransforming && len(m.resultFields) > 0 {
				totalCols := len(m.resultFields)
				if m.isTableCommand && len(m.tableCols) > 0 {
					totalCols = len(m.tableCols)
				}
				visCols := (m.width - 4) / 14
				if visCols < 2 {
					visCols = 2
				}
				if totalCols > visCols && m.colOffset < totalCols-visCols {
					m.colOffset += 3
					if m.colOffset > totalCols-visCols {
						m.colOffset = totalCols - visCols
					}
					return m, nil
				}
			}

			maxScroll := m.maxScroll(m.width - 24)
			if m.hScroll < maxScroll {
				m.hScroll += 60
				if m.hScroll > maxScroll {
					m.hScroll = maxScroll
				}
			}
			return m, nil

		case "left", "h":
			if m.colOffset > 0 {
				m.colOffset--
				return m, nil
			}
			if m.hScroll > 0 {
				m.hScroll -= 20
				if m.hScroll < 0 {
					m.hScroll = 0
				}
			}
			return m, nil

		case "shift+left":
			if m.colOffset > 0 {
				m.colOffset -= 3
				if m.colOffset < 0 {
					m.colOffset = 0
				}
				return m, nil
			}
			if m.hScroll > 0 {
				m.hScroll -= 60
				if m.hScroll < 0 {
					m.hScroll = 0
				}
			}
			return m, nil

		case "0", "home":
			m.hScroll = 0
			m.colOffset = 0
			return m, nil

		case "end", "$":
			if m.isTransforming && len(m.resultFields) > 0 {
				totalCols := len(m.resultFields)
				if m.isTableCommand && len(m.tableCols) > 0 {
					totalCols = len(m.tableCols)
				}
				visCols := (m.width - 4) / 14
				if visCols < 2 {
					visCols = 2
				}
				if totalCols > visCols {
					m.colOffset = totalCols - visCols
				}
			}
			m.hScroll = m.maxScroll(m.width - 24)
			return m, nil

		case "enter":
			if m.isTransforming && m.cursor < len(m.results) {
				return m, func() tea.Msg {
					return OpenDetailMsg{Row: m.results[m.cursor]}
				}
			}
			if !m.isTransforming && m.cursor < len(m.events) {
				return m, func() tea.Msg {
					return OpenDetailMsg{Event: m.events[m.cursor]}
				}
			}
		}
	}

	return m, nil
}

func (m TableModel) View() string {
	boxWidth := m.width - 4
	if boxWidth < 30 {
		boxWidth = 30
	}

	cardStyle := m.theme.Card
	if m.focused {
		cardStyle = m.theme.FocusedCard
	}

	rowCount := len(m.events)
	if m.isTransforming {
		rowCount = len(m.results)
	}

	availInner := m.height - 2
	if availInner < 2 {
		availInner = 2
	}

	if m.errMsg != "" {
		errHeader := lipgloss.NewStyle().Foreground(m.theme.Error).Bold(true).Render("✖ Search Error:")
		errBody := lipgloss.NewStyle().Foreground(m.theme.Error).Render(m.errMsg)
		errHelp := lipgloss.NewStyle().Foreground(m.theme.SubtleText).Render("Please check your SPL syntax, command names, and arguments.")
		content := lipgloss.JoinVertical(lipgloss.Left, errHeader, "", errBody, "", errHelp)
		return cardStyle.Width(boxWidth).Height(availInner).MaxHeight(m.height).Render(content)
	}

	if rowCount == 0 {
		emptyMsg := lipgloss.NewStyle().Foreground(m.theme.SubtleText).Italic(true).Render("No results to display.")
		return cardStyle.Width(boxWidth).Height(availInner).MaxHeight(m.height).Render(emptyMsg)
	}

	var allHeaders []string
	if m.isTableCommand && len(m.tableCols) > 0 {
		allHeaders = m.tableCols
	} else if m.isTransforming {
		allHeaders = m.resultFields
		if len(allHeaders) == 0 && len(m.results) > 0 {
			seen := make(map[string]bool)
			for _, r := range m.results {
				for k := range r {
					if !seen[k] {
						allHeaders = append(allHeaders, k)
						seen[k] = true
					}
				}
			}
		}
	} else {
		// Normal events: _time, selected fields, _raw
		allHeaders = []string{"_time"}
		allHeaders = append(allHeaders, m.selectedCols...)
		allHeaders = append(allHeaders, "_raw")
	}

	headers := allHeaders
	if m.isTransforming && m.colOffset > 0 && m.colOffset < len(allHeaders) {
		headers = allHeaders[m.colOffset:]
	}

	var colWidths []int
	if m.isTransforming || m.isTableCommand {
		colWidths = calculateColumnWidths(headers, boxWidth)
	} else {
		colWidths = make([]int, len(headers))
		colWidths[0] = 20 // _time

		remaining := boxWidth - 24
		for i := 1; i < len(headers)-1; i++ {
			colWidths[i] = 16
			remaining -= 18
		}
		if remaining < 20 {
			remaining = 20
		}
		colWidths[len(headers)-1] = remaining // _raw gets the rest
	}

	// Render header row
	var headerCells []string
	for i, h := range headers {
		w := colWidths[i]
		headerText := h
		if h == "_raw" && m.hScroll > 0 {
			headerText = fmt.Sprintf("_raw (▶ +%d)", m.hScroll)
		} else if m.isTransforming {
			if i == 0 && m.colOffset > 0 {
				headerText = "◀ " + h
			} else if i == len(headers)-1 && m.hScroll > 0 {
				headerText = fmt.Sprintf("%s (▶ +%d)", h, m.hScroll)
			}
		}
		headerCells = append(headerCells, m.theme.TableHeader.Width(w).Render(headerText))
	}
	headerLine := strings.Join(headerCells, " ")
	if lipgloss.Width(headerLine) > boxWidth {
		headerLine = truncate(headerLine, boxWidth)
	}

	// Render rows
	maxVisibleRows := availInner - 2
	if maxVisibleRows <= 0 {
		maxVisibleRows = 1
	}

	start := 0
	if m.cursor >= maxVisibleRows {
		start = m.cursor - maxVisibleRows + 1
	}
	end := start + maxVisibleRows
	if end > rowCount {
		end = rowCount
	}

	var rowLines []string
	for r := start; r < end; r++ {
		isCur := r == m.cursor && m.focused
		var cells []string

		for i, h := range headers {
			w := colWidths[i]
			var val string

			if m.isTransforming {
				val = m.results[r][h]
			} else {
				if h == "_raw" {
					val = m.events[r].Raw()
				} else {
					val = m.events[r].FieldValue(h)
				}
			}

			cleanVal := strings.ReplaceAll(val, "\n", " ")
			// If scrolled horizontally, slice text starting from hScroll
			if (h == "_raw" || (m.isTransforming && len([]rune(cleanVal)) > w)) && m.hScroll > 0 {
				runes := []rune(cleanVal)
				if m.hScroll < len(runes) {
					cleanVal = "…" + string(runes[m.hScroll:])
				} else {
					cleanVal = ""
				}
			}

			truncated := truncate(cleanVal, w)
			style := m.theme.TableRow
			if isCur {
				style = m.theme.TableSelected
			} else if r%2 == 1 {
				style = m.theme.TableRowAlt
			}

			// Syntax highlight _raw events or highlight severity in _time
			if !isCur && !m.isTransforming {
				if h == "_raw" {
					truncated = HighlightLogText(truncated, m.theme)
				} else if h == "_time" {
					lvl := DetectLogLevel(m.events[r].Raw())
					switch lvl {
					case LogLevelFatal, LogLevelError:
						truncated = lipgloss.NewStyle().Foreground(m.theme.Error).Render("✖ " + truncated)
					case LogLevelWarn:
						truncated = lipgloss.NewStyle().Foreground(m.theme.Warning).Render("⚠ " + truncated)
					case LogLevelInfo:
						truncated = lipgloss.NewStyle().Foreground(m.theme.Success).Render("● " + truncated)
					}
				}
			}

			cells = append(cells, style.Width(w).Render(truncated))
		}

		rowStr := strings.Join(cells, " ")
		if lipgloss.Width(rowStr) > boxWidth {
			rowStr = truncate(rowStr, boxWidth)
		}
		rowLines = append(rowLines, rowStr)
	}

	dividerLine := lipgloss.NewStyle().Foreground(m.theme.SurfaceBorder).Render(strings.Repeat("─", boxWidth))
	content := lipgloss.JoinVertical(lipgloss.Left, headerLine, dividerLine, lipgloss.JoinVertical(lipgloss.Left, rowLines...))
	return cardStyle.Width(boxWidth).Height(availInner).MaxHeight(m.height).Render(content)
}

func calculateColumnWidths(headers []string, totalWidth int) []int {
	if len(headers) == 0 {
		return nil
	}
	avg := (totalWidth - (len(headers) * 2)) / len(headers)
	if avg < 10 {
		avg = 10
	}
	widths := make([]int, len(headers))
	for i := range widths {
		widths[i] = avg
	}
	return widths
}

func isNextPageKey(msg tea.KeyMsg) bool {
	if msg.Type == tea.KeyPgDown {
		return true
	}
	s := strings.ToLower(msg.String())
	return s == "pgdown" || s == "pagedown" || s == "pgdn" || s == "page_down"
}

func isPrevPageKey(msg tea.KeyMsg) bool {
	if msg.Type == tea.KeyPgUp {
		return true
	}
	s := strings.ToLower(msg.String())
	return s == "pgup" || s == "pageup" || s == "page_up"
}
