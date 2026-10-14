package search

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"splunk-tui/internal/spl"
	"splunk-tui/internal/splunk"
	"splunk-tui/internal/ui/theme"
)

var blockLevels = []rune{' ', ' ', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

// ChartHeightOption represents the visual height mode for the chart pane.
type ChartHeightOption int

const (
	ChartHeightSmall  ChartHeightOption = iota // Default (8 rows)
	ChartHeightMedium                         // Medium (14 rows)
	ChartHeightLarge                          // Large (20 rows)
	ChartHeightXLarge                         // X-Large (28 rows)
	ChartHeightFull                           // Full screen height
)

func (h ChartHeightOption) Label() string {
	switch h {
	case ChartHeightSmall:
		return "Small"
	case ChartHeightMedium:
		return "Medium"
	case ChartHeightLarge:
		return "Large"
	case ChartHeightXLarge:
		return "X-Large"
	case ChartHeightFull:
		return "Full"
	default:
		return "Small"
	}
}

func (h ChartHeightOption) TotalRows() int {
	switch h {
	case ChartHeightSmall:
		return 8
	case ChartHeightMedium:
		return 14
	case ChartHeightLarge:
		return 20
	case ChartHeightXLarge:
		return 28
	case ChartHeightFull:
		return 40
	default:
		return 8
	}
}

// ChartPaneModel renders switchable graphs in the top section of the search view.
type ChartPaneModel struct {
	theme          theme.Theme
	detectedType   spl.ChartType   // Inferred from SPL query
	activeType     spl.ChartType   // Current visualization (user-switchable)
	availableTypes []spl.ChartType // List of compatible chart formats
	heightOption   ChartHeightOption
	timeline       []splunk.TimelineBucket
	results        *splunk.ResultsResponse
	userForced     *bool
	activeQuery    string
	activeSpan     string
	width          int
	height         int
	errMsg         string
}

// NewChartPane creates a new chart pane.
func NewChartPane(th theme.Theme) ChartPaneModel {
	return ChartPaneModel{
		theme:        th,
		detectedType: spl.ChartNone,
		activeType:   spl.ChartNone,
		heightOption: ChartHeightSmall,
		height:       ChartHeightSmall.TotalRows(),
	}
}

// HeightOption returns the current visual height mode.
func (m ChartPaneModel) HeightOption() ChartHeightOption {
	return m.heightOption
}

// SetHeightOption sets the visual height mode and updates height rows.
func (m *ChartPaneModel) SetHeightOption(opt ChartHeightOption) {
	m.heightOption = opt
	m.height = opt.TotalRows()
}

// CycleHeight cycles between Small -> Medium -> Large -> X-Large -> Small.
func (m *ChartPaneModel) CycleHeight() {
	switch m.heightOption {
	case ChartHeightSmall:
		m.heightOption = ChartHeightMedium
	case ChartHeightMedium:
		m.heightOption = ChartHeightLarge
	case ChartHeightLarge:
		m.heightOption = ChartHeightXLarge
	case ChartHeightXLarge:
		m.heightOption = ChartHeightFull
	case ChartHeightFull:
		m.heightOption = ChartHeightSmall
	default:
		m.heightOption = ChartHeightSmall
	}
	m.height = m.heightOption.TotalRows()
}

// SetQuery updates the SPL query string and extracts timechart span if present.
func (m *ChartPaneModel) SetQuery(q string) {
	m.activeQuery = q
	m.activeSpan = spl.ExtractTimechartSpan(q)
}

// SetSpan explicitly sets the span duration label.
func (m *ChartPaneModel) SetSpan(s string) {
	m.activeSpan = s
}

// ActiveSpan returns the current timechart span string.
func (m ChartPaneModel) ActiveSpan() string {
	return m.activeSpan
}

// SetData updates the chart with new search results and timeline.
func (m *ChartPaneModel) SetData(chartType spl.ChartType, timeline []splunk.TimelineBucket, results *splunk.ResultsResponse) {
	m.errMsg = ""
	if m.detectedType != chartType {
		m.detectedType = chartType
		m.availableTypes = spl.CompatibleChartTypes(chartType)
		if len(m.availableTypes) > 0 {
			m.activeType = m.availableTypes[0]
		} else {
			m.activeType = chartType
		}
		m.userForced = nil // Reset manual override on new query type
	}
	m.timeline = timeline
	m.results = results

	if m.activeSpan == "" && results != nil && len(results.Results) > 0 {
		if spanStr, ok := results.Results[0]["_span"]; ok && spanStr != "" {
			if sec, err := strconv.ParseInt(spanStr, 10, 64); err == nil {
				m.activeSpan = spl.FormatSpanSeconds(sec)
			}
		}
	}
}

// SetError sets an error message on the chart pane.
func (m *ChartPaneModel) SetError(errStr string) {
	m.errMsg = errStr
	m.results = nil
	m.timeline = nil
}

// ClearError clears any chart pane error.
func (m *ChartPaneModel) ClearError() {
	m.errMsg = ""
}

// CycleChartType switches to the next available visualization.
func (m *ChartPaneModel) CycleChartType() {
	if len(m.availableTypes) <= 1 {
		return
	}
	for i, t := range m.availableTypes {
		if t == m.activeType {
			m.activeType = m.availableTypes[(i+1)%len(m.availableTypes)]
			return
		}
	}
	m.activeType = m.availableTypes[0]
}

// SetActiveType sets an explicit visualization type if compatible.
func (m *ChartPaneModel) SetActiveType(t spl.ChartType) {
	for _, avail := range m.availableTypes {
		if avail == t {
			m.activeType = t
			return
		}
	}
}

// ActiveType returns the currently selected visualization.
func (m ChartPaneModel) ActiveType() spl.ChartType {
	return m.activeType
}

// IsVisible reports whether the chart pane is currently shown.
func (m ChartPaneModel) IsVisible() bool {
	if m.userForced != nil {
		return *m.userForced
	}
	return m.detectedType != spl.ChartNone && m.detectedType != spl.ChartEvents
}

// ToggleVisible toggles manual visibility override (ctrl+g).
func (m *ChartPaneModel) ToggleVisible() {
	curr := m.IsVisible()
	forced := !curr
	m.userForced = &forced
}

// ToggleCollapse is an alias for ToggleVisible for backward compatibility.
func (m *ChartPaneModel) ToggleCollapse() {
	m.ToggleVisible()
}

// IsCollapsed reports whether the chart pane is hidden.
func (m ChartPaneModel) IsCollapsed() bool {
	return !m.IsVisible()
}

// SetDimensions updates the allocated width and height.
func (m *ChartPaneModel) SetDimensions(w, h int) {
	m.width = w
	m.height = h
}

func (m ChartPaneModel) View() string {
	if !m.IsVisible() {
		return ""
	}

	cardW := m.width - 4
	if cardW < 20 {
		cardW = 20
	}
	contentW := cardW - 2
	if contentW < 18 {
		contentW = 18
	}

	chartH := m.height
	if chartH <= 0 {
		chartH = 8
	}
	availInner := chartH - 2
	if availInner < 1 {
		availInner = 1
	}

	var content string
	if m.errMsg != "" {
		errIcon := lipgloss.NewStyle().Foreground(m.theme.Error).Bold(true).Render("✖ Search Error:")
		errBody := lipgloss.NewStyle().Foreground(m.theme.Error).Render(m.errMsg)
		errHelp := lipgloss.NewStyle().Foreground(m.theme.SubtleText).Render("Please check your SPL syntax and arguments.")
		content = lipgloss.JoinVertical(lipgloss.Left, errIcon, "", errBody, "", errHelp)
	} else {
		switch m.activeType {
		case spl.ChartLine:
			content = m.renderLineChart(contentW)
		case spl.ChartSparklines:
			content = m.renderSparklinesChart(contentW)
		case spl.ChartColumnTime:
			content = m.renderTimechartColumns(contentW)
		case spl.ChartBar:
			content = m.renderBarChart(contentW)
		case spl.ChartPie:
			content = m.renderPieChart(contentW)
		case spl.ChartColumn:
			content = m.renderColumnChart(contentW)
		case spl.ChartEvents:
			content = m.renderEventCountChart(contentW)
		default:
			content = lipgloss.NewStyle().Foreground(m.theme.SubtleText).Render("No chart data available for this search.")
		}
	}

	// Render header with switchable pills and height indicator
	header := m.renderHeader(contentW)

	return m.theme.Card.Width(cardW).Height(availInner).MaxHeight(chartH).Render(lipgloss.JoinVertical(lipgloss.Left, header, "", content))
}

func (m ChartPaneModel) renderHeader(width int) string {
	titleText := "📈 " + m.activeType.Label()
	if m.activeSpan != "" && (m.detectedType == spl.ChartLine || m.detectedType == spl.ChartSparklines || m.detectedType == spl.ChartColumnTime) {
		titleText += fmt.Sprintf(" (span: %s)", m.activeSpan)
	}
	title := m.theme.Title.Render(titleText)

	// Render visualization pills if multiple options exist
	var pills []string
	if len(m.availableTypes) > 1 {
		for _, t := range m.availableTypes {
			lbl := t.Label()
			if t == m.activeType {
				pills = append(pills, m.theme.ActiveButton.Render("● "+lbl))
			} else {
				pills = append(pills, m.theme.Button.Render("○ "+lbl))
			}
		}
	}

	pillsStr := strings.Join(pills, " ")
	heightPill := m.theme.ActiveButton.Render("⇕ " + m.heightOption.Label())

	var leftSide string
	if len(pills) > 0 {
		leftSide = lipgloss.JoinHorizontal(lipgloss.Center, title, "  ", pillsStr, "  ", heightPill)
	} else {
		leftSide = lipgloss.JoinHorizontal(lipgloss.Center, title, "  ", heightPill)
	}

	leftSideW := lipgloss.Width(leftSide)
	availHelpW := width - leftSideW - 1

	var helpText string
	if availHelpW >= 38 {
		helpText = "(v: switch • h: height • ctrl+g: hide)"
	} else if availHelpW >= 24 {
		helpText = "(v: switch • h: height)"
	} else if availHelpW >= 11 {
		helpText = "(h: height)"
	} else if availHelpW >= 5 {
		helpText = "(h)"
	}

	var header string
	if helpText != "" {
		helpStr := lipgloss.NewStyle().Foreground(m.theme.SubtleText).Render(helpText)
		header = lipgloss.JoinHorizontal(lipgloss.Center, leftSide, " ", helpStr)
	} else {
		header = leftSide
	}

	if lipgloss.Width(header) > width {
		header = truncate(header, width)
	}
	return header
}

// ---------------------------------------------------------------------
// 1. 2D Cartesian Line Chart (Braille continuous curves over time)
// ---------------------------------------------------------------------

func (m ChartPaneModel) renderLineChart(width int) string {
	seriesCols, vals := m.extractTimeSeries()
	if len(seriesCols) == 0 || len(vals) == 0 {
		return m.renderEventCountChart(width)
	}

	numPoints := len(vals[0])
	if numPoints == 0 {
		return lipgloss.NewStyle().Foreground(m.theme.SubtleText).Render("Waiting for time series data...")
	}

	numSeries := len(seriesCols)

	var maxVal float64
	for s := 0; s < numSeries; s++ {
		for _, v := range vals[s] {
			if v > maxVal {
				maxVal = v
			}
		}
	}
	if maxVal <= 0 {
		maxVal = 1
	}

	legendH := 0
	if numSeries > 1 {
		legendH = 1
	}

	gridW := width - 14
	if gridW < 10 {
		gridW = 10
	}
	gridH := m.height - 6 - legendH
	if gridH < 2 {
		gridH = 2
	}

	// Virtual pixel resolution: 2 horizontal dots per char, 4 vertical dots per char
	pixW := gridW * 2
	pixH := gridH * 4

	canvas := make([][]rune, gridH)
	cellColor := make([][]lipgloss.Color, gridH)
	palette := m.theme.ChartPalette
	if len(palette) == 0 {
		palette = []lipgloss.Color{m.theme.Primary, m.theme.Highlight, m.theme.Warning, m.theme.Secondary, m.theme.Error}
	}

	for r := range canvas {
		canvas[r] = make([]rune, gridW)
		cellColor[r] = make([]lipgloss.Color, gridW)
		for c := range canvas[r] {
			canvas[r][c] = 0x2800 // Blank braille pattern
			cellColor[r][c] = palette[0]
		}
	}

	// Draw each series on Braille canvas
	for s := 0; s < numSeries; s++ {
		color := palette[s%len(palette)]
		seriesPoints := len(vals[s])
		if seriesPoints == 0 {
			continue
		}
		sampleRatio := float64(seriesPoints-1) / float64(pixW-1)
		if seriesPoints == 1 {
			sampleRatio = 0
		}

		var prevX, prevY int
		for px := 0; px < pixW; px++ {
			ptIdx := int(float64(px) * sampleRatio)
			if ptIdx >= seriesPoints {
				ptIdx = seriesPoints - 1
			}
			v := vals[s][ptIdx]
			py := int(math.Round((v / maxVal) * float64(pixH-1)))
			if py >= pixH {
				py = pixH - 1
			}
			if py < 0 {
				py = 0
			}
			// Invert Y so 0 is at bottom
			invertedY := (pixH - 1) - py

			if px > 0 {
				drawBrailleLineColor(canvas, cellColor, prevX, prevY, px, invertedY, color)
			} else {
				setBrailleDotColor(canvas, cellColor, px, invertedY, color)
			}
			prevX, prevY = px, invertedY
		}
	}

	var lines []string

	// Render plot rows with Y-axis tick values
	for r := 0; r < gridH; r++ {
		rowMaxVal := maxVal * float64(gridH-r) / float64(gridH)
		axisLabel := fmt.Sprintf("%6s ┤ ", formatMetric(rowMaxVal))
		axisStyled := lipgloss.NewStyle().Foreground(m.theme.SubtleText).Render(axisLabel)

		var rowChars strings.Builder
		var curColor lipgloss.Color
		var curBuf strings.Builder

		flush := func() {
			if curBuf.Len() > 0 {
				if curColor != "" {
					rowChars.WriteString(lipgloss.NewStyle().Foreground(curColor).Render(curBuf.String()))
				} else {
					rowChars.WriteString(curBuf.String())
				}
				curBuf.Reset()
			}
		}

		for c := 0; c < gridW; c++ {
			rn := canvas[r][c]
			if rn == 0x2800 {
				flush()
				curColor = ""
				curBuf.WriteString(" ")
			} else {
				col := cellColor[r][c]
				if col != curColor {
					flush()
					curColor = col
				}
				curBuf.WriteRune(rn)
			}
		}
		flush()

		lines = append(lines, axisStyled+rowChars.String())
	}

	// Bottom axis line
	bottomAxis := fmt.Sprintf("%6s ┴%s", "0", strings.Repeat("─", gridW))
	lines = append(lines, lipgloss.NewStyle().Foreground(m.theme.SubtleText).Render(bottomAxis))

	// Time boundaries
	t1, t2 := m.getTimeBounds()
	timePadding := gridW - len(t1) - len(t2)
	if timePadding < 2 {
		timePadding = 2
	}
	timeAxis := fmt.Sprintf("%8s%s%s%s", "", t1, strings.Repeat(" ", timePadding), t2)
	lines = append(lines, lipgloss.NewStyle().Foreground(m.theme.SubtleText).Render(timeAxis))

	// Legend for multiple series
	if numSeries > 1 {
		var legendItems []string
		for s := 0; s < numSeries; s++ {
			col := palette[s%len(palette)]
			swatch := lipgloss.NewStyle().Foreground(col).Render("■")
			name := lipgloss.NewStyle().Foreground(m.theme.Text).Render(truncate(seriesCols[s], 14))
			legendItems = append(legendItems, swatch+" "+name)
		}
		legendStr := fmt.Sprintf("%8s%s", "", strings.Join(legendItems, "   "))
		lines = append(lines, truncate(legendStr, width))
	}

	return strings.Join(lines, "\n")
}

func drawBrailleLineColor(canvas [][]rune, cellColor [][]lipgloss.Color, x0, y0, x1, y1 int, color lipgloss.Color) {
	dx := math.Abs(float64(x1 - x0))
	dy := math.Abs(float64(y1 - y0))
	sx := 1
	if x0 > x1 {
		sx = -1
	}
	sy := 1
	if y0 > y1 {
		sy = -1
	}
	err := dx - dy

	for {
		setBrailleDotColor(canvas, cellColor, x0, y0, color)
		if x0 == x1 && y0 == y1 {
			break
		}
		e2 := 2 * err
		if e2 > -dy {
			err -= dy
			x0 += sx
		}
		if e2 < dx {
			err += dx
			y0 += sy
		}
	}
}

func setBrailleDotColor(canvas [][]rune, cellColor [][]lipgloss.Color, x, y int, color lipgloss.Color) {
	charCol := x / 2
	charRow := y / 4
	if charRow < 0 || charRow >= len(canvas) || charCol < 0 || charCol >= len(canvas[0]) {
		return
	}

	subX := x % 2
	subY := y % 4

	// Braille dot offset mapping
	var mask rune
	switch {
	case subX == 0 && subY == 0:
		mask = 0x01
	case subX == 0 && subY == 1:
		mask = 0x02
	case subX == 0 && subY == 2:
		mask = 0x04
	case subX == 0 && subY == 3:
		mask = 0x40
	case subX == 1 && subY == 0:
		mask = 0x08
	case subX == 1 && subY == 1:
		mask = 0x10
	case subX == 1 && subY == 2:
		mask = 0x20
	case subX == 1 && subY == 3:
		mask = 0x80
	}

	canvas[charRow][charCol] |= mask
	cellColor[charRow][charCol] = color
}

// ---------------------------------------------------------------------
// 2. Pie / Donut Chart (Categorical distribution)
// ---------------------------------------------------------------------

func (m ChartPaneModel) renderPieChart(width int) string {
	items := m.extractCategoricalData()
	if len(items) == 0 {
		return lipgloss.NewStyle().Foreground(m.theme.SubtleText).Render("No categorical data to plot.")
	}

	// Calculate total and percentage
	var total float64
	for _, it := range items {
		total += it.val
	}
	if total <= 0 {
		return lipgloss.NewStyle().Foreground(m.theme.SubtleText).Render("All numeric values are zero.")
	}

	// Group into top 4 + other
	type sliceItem struct {
		name string
		val  float64
		pct  float64
	}
	var slices []sliceItem
	maxSlices := 4
	for i, it := range items {
		if i < maxSlices {
			slices = append(slices, sliceItem{name: it.name, val: it.val, pct: (it.val / total) * 100})
		} else {
			if len(slices) == maxSlices {
				slices = append(slices, sliceItem{name: "Other", val: it.val, pct: (it.val / total) * 100})
			} else {
				slices[maxSlices].val += it.val
				slices[maxSlices].pct = (slices[maxSlices].val / total) * 100
			}
		}
	}

	palette := m.theme.ChartPalette
	if len(palette) == 0 {
		palette = []lipgloss.Color{m.theme.Primary, m.theme.Highlight, m.theme.Warning, m.theme.Secondary, m.theme.Error}
	}

	// Calculate slice angle boundaries in radians [0, 2pi]
	angleBounds := make([]float64, len(slices))
	cumAngle := 0.0
	for i, sl := range slices {
		cumAngle += (sl.pct / 100.0) * 2 * math.Pi
		angleBounds[i] = cumAngle
	}

	// Render a 12x5 character circular donut glyph
	var glyphRows []string
	rows := 5
	cols := 12
	cx := 5.5
	cy := 2.0
	aspect := 2.0 // Terminal characters are ~2x taller than wide

	for y := 0; y < rows; y++ {
		var row strings.Builder
		for x := 0; x < cols; x++ {
			dx := float64(x) - cx
			dy := (float64(y) - cy) * aspect
			r := math.Sqrt(dx*dx + dy*dy)

			if r >= 1.2 && r <= 4.8 {
				theta := math.Atan2(dy, dx)
				if theta < 0 {
					theta += 2 * math.Pi
				}

				// Find which slice angle theta falls into
				sliceIdx := 0
				for idx, b := range angleBounds {
					if theta <= b {
						sliceIdx = idx
						break
					}
				}
				color := palette[sliceIdx%len(palette)]
				row.WriteString(lipgloss.NewStyle().Foreground(color).Render("█"))
			} else {
				row.WriteString(" ")
			}
		}
		glyphRows = append(glyphRows, row.String())
	}
	glyphStr := strings.Join(glyphRows, "\n")

	// Render right-side legend table
	var legendRows []string
	labelWidth := width - 36
	if labelWidth < 10 {
		labelWidth = 10
	}

	for i, sl := range slices {
		color := palette[i%len(palette)]
		swatch := lipgloss.NewStyle().Foreground(color).Render("■")
		name := truncate(sl.name, labelWidth)
		pctStr := fmt.Sprintf("%5.1f%%", sl.pct)
		valStr := fmt.Sprintf("(%s)", formatMetric(sl.val))

		lblStyled := lipgloss.NewStyle().Foreground(m.theme.Text).Width(labelWidth).Render(name)
		pctStyled := lipgloss.NewStyle().Foreground(color).Bold(true).Render(pctStr)
		valStyled := lipgloss.NewStyle().Foreground(m.theme.SubtleText).Render(valStr)

		legendRows = append(legendRows, fmt.Sprintf("%s %s %s %s", swatch, lblStyled, pctStyled, valStyled))
	}

	legendStr := strings.Join(legendRows, "\n")
	return lipgloss.JoinHorizontal(lipgloss.Center, glyphStr, "   ", legendStr)
}

// ---------------------------------------------------------------------
// 3. Vertical Column Chart (Categorical distribution)
// ---------------------------------------------------------------------

func (m ChartPaneModel) renderColumnChart(width int) string {
	items := m.extractCategoricalData()
	if len(items) == 0 {
		return lipgloss.NewStyle().Foreground(m.theme.SubtleText).Render("No categorical data to plot.")
	}

	// Display categories that fit within available width
	displayCount := len(items)
	maxFit := (width - 16) / 4
	if maxFit > 0 && displayCount > maxFit {
		displayCount = maxFit
	}

	var maxVal float64
	for i := 0; i < displayCount; i++ {
		if items[i].val > maxVal {
			maxVal = items[i].val
		}
	}
	if maxVal <= 0 {
		maxVal = 1
	}

	plotH := m.height - 5
	if plotH < 3 {
		plotH = 3
	}
	colWidth := (width - 16) / displayCount
	if colWidth < 4 {
		colWidth = 4
	}
	if colWidth > 12 {
		colWidth = 12
	}

	palette := m.theme.ChartPalette
	var lines []string

	for row := plotH; row >= 1; row-- {
		rowMin := float64(row-1) / float64(plotH) * maxVal
		rowMax := float64(row) / float64(plotH) * maxVal

		axisLabel := fmt.Sprintf("%6s ┤ ", formatMetric(rowMax))
		axisStyled := lipgloss.NewStyle().Foreground(m.theme.SubtleText).Render(axisLabel)

		var barCols []string
		for i := 0; i < displayCount; i++ {
			v := items[i].val
			color := m.theme.Primary
			if len(palette) > 0 {
				color = palette[i%len(palette)]
			}

			var ch rune = ' '
			if v >= rowMax {
				ch = '█'
			} else if v > rowMin {
				fraction := (v - rowMin) / (rowMax - rowMin)
				idx := int(fraction * float64(len(blockLevels)-1))
				if idx >= len(blockLevels) {
					idx = len(blockLevels) - 1
				}
				ch = blockLevels[idx]
			}
			barCell := strings.Repeat(string(ch), colWidth-1) + " "
			barCols = append(barCols, lipgloss.NewStyle().Foreground(color).Render(barCell))
		}
		lines = append(lines, axisStyled+strings.Join(barCols, ""))
	}

	// Bottom axis line
	totalBarW := displayCount * colWidth
	bottomAxis := fmt.Sprintf("%6s ┴%s", "0", strings.Repeat("─", totalBarW))
	lines = append(lines, lipgloss.NewStyle().Foreground(m.theme.SubtleText).Render(bottomAxis))

	// Labels under columns
	var labelCols []string
	for i := 0; i < displayCount; i++ {
		lbl := truncate(items[i].name, colWidth-1)
		labelCols = append(labelCols, lipgloss.NewStyle().Foreground(m.theme.SubtleText).Width(colWidth).Render(lbl))
	}
	lines = append(lines, fmt.Sprintf("%8s%s", "", strings.Join(labelCols, "")))

	return strings.Join(lines, "\n")
}

// ---------------------------------------------------------------------
// 4. Horizontal Bar Chart (Categorical distribution)
// ---------------------------------------------------------------------

func (m ChartPaneModel) renderBarChart(width int) string {
	items := m.extractCategoricalData()
	if len(items) == 0 {
		return lipgloss.NewStyle().Foreground(m.theme.SubtleText).Render("No chartable transforming results.")
	}

	var maxVal float64
	displayLimit := m.height - 4
	if displayLimit < 4 {
		displayLimit = 4
	}
	if len(items) < displayLimit {
		displayLimit = len(items)
	}
	for i := 0; i < displayLimit; i++ {
		if items[i].val > maxVal {
			maxVal = items[i].val
		}
	}
	if maxVal <= 0 {
		maxVal = 1
	}

	barSpace := width - 34
	if barSpace < 10 {
		barSpace = 10
	}

	palette := m.theme.ChartPalette
	var rows []string
	for i := 0; i < displayLimit; i++ {
		it := items[i]
		ratio := it.val / maxVal
		barLen := int(math.Round(ratio * float64(barSpace)))
		if barLen < 1 && it.val > 0 {
			barLen = 1
		}

		color := m.theme.Highlight
		if len(palette) > 0 {
			color = palette[i%len(palette)]
		}

		nameLabel := lipgloss.NewStyle().Width(14).Render(truncate(it.name, 13))
		bar := lipgloss.NewStyle().Foreground(color).Render(strings.Repeat("█", barLen))
		valLabel := lipgloss.NewStyle().Foreground(m.theme.SubtleText).Render(fmt.Sprintf(" %s", formatMetric(it.val)))

		rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Center, nameLabel, " ", bar, valLabel))
	}

	return strings.Join(rows, "\n")
}

// ---------------------------------------------------------------------
// 5. Sparklines Multi-Series Chart
// ---------------------------------------------------------------------

func (m ChartPaneModel) renderSparklinesChart(width int) string {
	seriesCols, vals := m.extractTimeSeries()
	if len(seriesCols) == 0 || len(vals) == 0 {
		return m.renderEventCountChart(width)
	}

	maxSeries := m.height - 5
	if maxSeries < 3 {
		maxSeries = 3
	}
	if len(seriesCols) < maxSeries {
		maxSeries = len(seriesCols)
	}

	sparkWidth := width - 26
	if sparkWidth < 10 {
		sparkWidth = 10
	}

	palette := m.theme.ChartPalette
	var rows []string

	for s := 0; s < maxSeries; s++ {
		colName := seriesCols[s]
		seriesVals := vals[s]

		var maxVal float64
		for _, v := range seriesVals {
			if v > maxVal {
				maxVal = v
			}
		}

		var spark strings.Builder
		if maxVal > 0 && len(seriesVals) > 0 {
			sampleRatio := float64(len(seriesVals)) / float64(sparkWidth)
			for c := 0; c < sparkWidth; c++ {
				idx := int(float64(c) * sampleRatio)
				if idx >= len(seriesVals) {
					idx = len(seriesVals) - 1
				}
				ratio := seriesVals[idx] / maxVal
				blockIdx := int(ratio * float64(len(blockLevels)-1))
				if blockIdx >= len(blockLevels) {
					blockIdx = len(blockLevels) - 1
				}
				spark.WriteRune(blockLevels[blockIdx])
			}
		}

		color := m.theme.Primary
		if len(palette) > 0 {
			color = palette[s%len(palette)]
		}

		lbl := lipgloss.NewStyle().Width(12).Bold(true).Render(truncate(colName, 11))
		spk := lipgloss.NewStyle().Foreground(color).Render(spark.String())
		maxLbl := lipgloss.NewStyle().Foreground(m.theme.SubtleText).Render(fmt.Sprintf(" max:%s", formatMetric(maxVal)))
		rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Center, lbl, " ", spk, maxLbl))
	}

	return strings.Join(rows, "\n")
}

// ---------------------------------------------------------------------
// 6. Event Count Histogram over Timeline
// ---------------------------------------------------------------------

func (m ChartPaneModel) renderEventCountChart(width int) string {
	if len(m.timeline) == 0 {
		return lipgloss.NewStyle().Foreground(m.theme.SubtleText).Render("Waiting for timeline buckets...")
	}

	numCols := width - 12
	if numCols <= 0 {
		numCols = 20
	}

	colCounts := make([]int64, numCols)
	var maxCount int64

	bucketRatio := float64(len(m.timeline)) / float64(numCols)
	for col := 0; col < numCols; col++ {
		startIdx := int(float64(col) * bucketRatio)
		endIdx := int(float64(col+1) * bucketRatio)
		if endIdx > len(m.timeline) {
			endIdx = len(m.timeline)
		}
		if startIdx >= len(m.timeline) {
			startIdx = len(m.timeline) - 1
		}
		if endIdx <= startIdx {
			endIdx = startIdx + 1
		}

		var sum int64
		for i := startIdx; i < endIdx; i++ {
			sum += m.timeline[i].Count
		}
		colCounts[col] = sum
		if sum > maxCount {
			maxCount = sum
		}
	}

	if maxCount == 0 {
		return lipgloss.NewStyle().Foreground(m.theme.SubtleText).Render("No matching events in the selected time range.")
	}

	plotLines := m.height - 6
	if plotLines < 2 {
		plotLines = 2
	}
	var lines []string

	for row := plotLines; row >= 1; row-- {
		rowMin := float64(row-1) / float64(plotLines) * float64(maxCount)
		rowMax := float64(row) / float64(plotLines) * float64(maxCount)

		var sb strings.Builder
		for _, cnt := range colCounts {
			if float64(cnt) < rowMin {
				sb.WriteRune(' ')
			} else if float64(cnt) >= rowMax {
				sb.WriteRune('█')
			} else {
				fraction := (float64(cnt) - rowMin) / (rowMax - rowMin)
				idx := int(fraction * float64(len(blockLevels)-1))
				if idx >= len(blockLevels) {
					idx = len(blockLevels) - 1
				}
				sb.WriteRune(blockLevels[idx])
			}
		}

		axisLabel := fmt.Sprintf("%6s ┤ ", formatMetric(rowMax))
		axisStyled := lipgloss.NewStyle().Foreground(m.theme.SubtleText).Render(axisLabel)
		barStyled := lipgloss.NewStyle().Foreground(m.theme.Primary).Render(sb.String())
		lines = append(lines, axisStyled+barStyled)
	}

	// Bottom axis line
	bottomAxis := fmt.Sprintf("%6s ┴%s", "0", strings.Repeat("─", numCols))
	lines = append(lines, lipgloss.NewStyle().Foreground(m.theme.SubtleText).Render(bottomAxis))

	// Time boundaries
	t1, t2 := m.getTimeBounds()
	timePadding := numCols - len(t1) - len(t2)
	if timePadding < 2 {
		timePadding = 2
	}
	timeAxis := fmt.Sprintf("%8s%s%s%s", "", t1, strings.Repeat(" ", timePadding), t2)
	lines = append(lines, lipgloss.NewStyle().Foreground(m.theme.SubtleText).Render(timeAxis))

	return strings.Join(lines, "\n")
}

func (m ChartPaneModel) renderTimechartColumns(width int) string {
	seriesCols, vals := m.extractTimeSeries()
	if len(seriesCols) == 0 || len(vals) == 0 || len(vals[0]) == 0 {
		return m.renderEventCountChart(width)
	}

	numCols := width - 12
	if numCols <= 0 {
		numCols = 20
	}

	numSeries := len(seriesCols)
	numBins := len(vals[0])

	colSeriesVals := make([][]float64, numCols)
	for col := 0; col < numCols; col++ {
		colSeriesVals[col] = make([]float64, numSeries)
	}
	colTotals := make([]float64, numCols)

	ratio := float64(numBins) / float64(numCols)
	for col := 0; col < numCols; col++ {
		startIdx := int(float64(col) * ratio)
		endIdx := int(float64(col+1) * ratio)
		if endIdx > numBins {
			endIdx = numBins
		}
		if startIdx >= numBins {
			startIdx = numBins - 1
		}
		if endIdx <= startIdx {
			endIdx = startIdx + 1
		}

		var colTot float64
		for s := 0; s < numSeries; s++ {
			var sSum float64
			for i := startIdx; i < endIdx; i++ {
				sSum += vals[s][i]
			}
			colSeriesVals[col][s] = sSum
			colTot += sSum
		}
		colTotals[col] = colTot
	}

	var maxVal float64
	for _, tot := range colTotals {
		if tot > maxVal {
			maxVal = tot
		}
	}
	if maxVal <= 0 {
		maxVal = 1
	}

	legendH := 0
	if numSeries > 1 {
		legendH = 1
	}

	plotLines := m.height - 6 - legendH
	if plotLines < 2 {
		plotLines = 2
	}

	palette := m.theme.ChartPalette
	if len(palette) == 0 {
		palette = []lipgloss.Color{m.theme.Primary, m.theme.Highlight, m.theme.Warning, m.theme.Secondary, m.theme.Error}
	}

	colCumVals := make([][]float64, numCols)
	for col := 0; col < numCols; col++ {
		cum := make([]float64, numSeries+1)
		cum[0] = 0
		for s := 0; s < numSeries; s++ {
			cum[s+1] = cum[s] + colSeriesVals[col][s]
		}
		colCumVals[col] = cum
	}

	var lines []string
	for row := plotLines; row >= 1; row-- {
		rowMin := float64(row-1) / float64(plotLines) * maxVal
		rowMax := float64(row) / float64(plotLines) * maxVal

		axisLabel := fmt.Sprintf("%6s ┤ ", formatMetric(rowMax))
		axisStyled := lipgloss.NewStyle().Foreground(m.theme.SubtleText).Render(axisLabel)

		var rowChars strings.Builder
		var curColor lipgloss.Color
		var curBuf strings.Builder

		flush := func() {
			if curBuf.Len() > 0 {
				if curColor != "" {
					rowChars.WriteString(lipgloss.NewStyle().Foreground(curColor).Render(curBuf.String()))
				} else {
					rowChars.WriteString(curBuf.String())
				}
				curBuf.Reset()
			}
		}

		for col := 0; col < numCols; col++ {
			tot := colTotals[col]
			if tot < rowMin || tot == 0 {
				if curColor != "" {
					flush()
					curColor = ""
				}
				curBuf.WriteRune(' ')
				continue
			}

			cum := colCumVals[col]
			if tot >= rowMax {
				// Midpoint of row belongs to which stacked series
				rowMid := (rowMin + rowMax) / 2.0
				seriesIdx := 0
				for s := 0; s < numSeries; s++ {
					if rowMid >= cum[s] && rowMid < cum[s+1] {
						seriesIdx = s
						break
					}
					if s == numSeries-1 && rowMid >= cum[s] {
						seriesIdx = s
					}
				}
				colColor := palette[seriesIdx%len(palette)]
				if colColor != curColor {
					flush()
					curColor = colColor
				}
				curBuf.WriteRune('█')
			} else {
				// Topmost partial block of the stack
				fraction := (tot - rowMin) / (rowMax - rowMin)
				idx := int(fraction * float64(len(blockLevels)-1))
				if idx >= len(blockLevels) {
					idx = len(blockLevels) - 1
				}
				if idx < 1 {
					idx = 1
				}
				topSeriesIdx := 0
				for s := numSeries - 1; s >= 0; s-- {
					if colSeriesVals[col][s] > 0 {
						topSeriesIdx = s
						break
					}
				}
				colColor := palette[topSeriesIdx%len(palette)]
				if colColor != curColor {
					flush()
					curColor = colColor
				}
				curBuf.WriteRune(blockLevels[idx])
			}
		}
		flush()

		lines = append(lines, axisStyled+rowChars.String())
	}

	// Bottom axis line
	bottomAxis := fmt.Sprintf("%6s ┴%s", "0", strings.Repeat("─", numCols))
	lines = append(lines, lipgloss.NewStyle().Foreground(m.theme.SubtleText).Render(bottomAxis))

	// Time boundaries
	t1, t2 := m.getTimeBounds()
	timePadding := numCols - len(t1) - len(t2)
	if timePadding < 2 {
		timePadding = 2
	}
	timeAxis := fmt.Sprintf("%8s%s%s%s", "", t1, strings.Repeat(" ", timePadding), t2)
	lines = append(lines, lipgloss.NewStyle().Foreground(m.theme.SubtleText).Render(timeAxis))

	// Legend for multiple series
	if numSeries > 1 {
		var legendItems []string
		for s := 0; s < numSeries; s++ {
			col := palette[s%len(palette)]
			swatch := lipgloss.NewStyle().Foreground(col).Render("■")
			name := lipgloss.NewStyle().Foreground(m.theme.Text).Render(truncate(seriesCols[s], 14))
			legendItems = append(legendItems, swatch+" "+name)
		}
		legendStr := fmt.Sprintf("%8s%s", "", strings.Join(legendItems, "   "))
		lines = append(lines, truncate(legendStr, width))
	}

	return strings.Join(lines, "\n")
}

// ---------------------------------------------------------------------
// Data Extraction Helpers
// ---------------------------------------------------------------------

type categoricalItem struct {
	name string
	val  float64
}

func (m ChartPaneModel) extractCategoricalData() []categoricalItem {
	if m.results == nil || len(m.results.Results) == 0 || len(m.results.Fields) < 2 {
		return nil
	}

	catField := m.results.Fields[0]
	valField := m.results.Fields[1]

	// If field 0 looks like an aggregation metric and field 1 is the category, swap them
	if (strings.HasPrefix(catField, "count") || strings.HasPrefix(catField, "sum") || strings.HasPrefix(catField, "avg")) &&
		!strings.HasPrefix(valField, "count") && !strings.HasPrefix(valField, "sum") && !strings.HasPrefix(valField, "avg") {
		catField, valField = valField, catField
	}

	var items []categoricalItem
	for _, row := range m.results.Results {
		cat := row[catField]
		valStr := row[valField]
		val, err := strconv.ParseFloat(valStr, 64)
		if err == nil {
			items = append(items, categoricalItem{name: cat, val: val})
		}
	}

	// Separate existing "other" if present
	var nonOther []categoricalItem
	var existingOtherVal float64
	hasOther := false
	for _, it := range items {
		if strings.EqualFold(it.name, "other") {
			existingOtherVal += it.val
			hasOther = true
		} else {
			nonOther = append(nonOther, it)
		}
	}

	// Sort non-other descending by value
	sort.SliceStable(nonOther, func(i, j int) bool {
		return nonOther[i].val > nonOther[j].val
	})

	limit, useOther := spl.EffectiveLimit(m.activeQuery)
	if limit > 0 && len(nonOther) > limit {
		topItems := append([]categoricalItem(nil), nonOther[:limit]...)
		if useOther {
			otherVal := existingOtherVal
			for _, rem := range nonOther[limit:] {
				otherVal += rem.val
			}
			topItems = append(topItems, categoricalItem{name: "other", val: otherVal})
		}
		return topItems
	}

	if hasOther {
		return append(nonOther, categoricalItem{name: "other", val: existingOtherVal})
	}
	return nonOther
}

func (m ChartPaneModel) extractTimeSeries() ([]string, [][]float64) {
	if m.results == nil || len(m.results.Results) == 0 {
		return nil, nil
	}

	var seriesCols []string
	for _, f := range m.results.Fields {
		if !strings.HasPrefix(f, "_") {
			seriesCols = append(seriesCols, f)
		}
	}
	if len(seriesCols) == 0 {
		return nil, nil
	}

	numRows := len(m.results.Results)

	type colData struct {
		name  string
		vals  []float64
		total float64
	}

	var candidates []colData
	var existingOtherVals []float64
	hasExistingOther := false

	for _, col := range seriesCols {
		vals := make([]float64, numRows)
		var tot float64
		for i, r := range m.results.Results {
			v, err := strconv.ParseFloat(r[col], 64)
			if err != nil {
				v = 0
			}
			vals[i] = v
			tot += v
		}

		if strings.EqualFold(col, "other") {
			hasExistingOther = true
			if len(existingOtherVals) == 0 {
				existingOtherVals = vals
			} else {
				for i := 0; i < numRows; i++ {
					existingOtherVals[i] += vals[i]
				}
			}
		} else {
			candidates = append(candidates, colData{
				name:  col,
				vals:  vals,
				total: tot,
			})
		}
	}

	// Sort candidates descending by total volume
	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].total > candidates[j].total
	})

	limit, useOther := spl.EffectiveLimit(m.activeQuery)
	if limit > 0 && len(candidates) > limit {
		var finalCols []string
		var finalVals [][]float64

		for i := 0; i < limit; i++ {
			finalCols = append(finalCols, candidates[i].name)
			finalVals = append(finalVals, candidates[i].vals)
		}

		if useOther {
			otherVals := make([]float64, numRows)
			if hasExistingOther {
				copy(otherVals, existingOtherVals)
			}
			for i := limit; i < len(candidates); i++ {
				for row := 0; row < numRows; row++ {
					otherVals[row] += candidates[i].vals[row]
				}
			}
			finalCols = append(finalCols, "other")
			finalVals = append(finalVals, otherVals)
		}

		return finalCols, finalVals
	}

	// When limit == 0 or len(candidates) <= limit: return all candidates as-is
	var finalCols []string
	var finalVals [][]float64
	for _, c := range candidates {
		finalCols = append(finalCols, c.name)
		finalVals = append(finalVals, c.vals)
	}
	if hasExistingOther {
		finalCols = append(finalCols, "other")
		finalVals = append(finalVals, existingOtherVals)
	}
	return finalCols, finalVals
}

func (m ChartPaneModel) getTimeBounds() (string, string) {
	if (m.detectedType == spl.ChartLine || m.detectedType == spl.ChartSparklines || m.detectedType == spl.ChartColumnTime) &&
		m.results != nil && len(m.results.Results) > 0 {
		r0 := m.results.Results[0]["_time"]
		rn := m.results.Results[len(m.results.Results)-1]["_time"]
		if r0 != "" && rn != "" {
			return formatTimeString(r0), formatTimeString(rn)
		}
	}
	if len(m.timeline) > 0 {
		t1 := m.timeline[0].Time.Format("01/02 15:04")
		t2 := m.timeline[len(m.timeline)-1].Time.Format("01/02 15:04")
		return t1, t2
	}
	if m.results != nil && len(m.results.Results) > 0 {
		r0 := m.results.Results[0]["_time"]
		rn := m.results.Results[len(m.results.Results)-1]["_time"]
		if r0 != "" && rn != "" {
			return formatTimeString(r0), formatTimeString(rn)
		}
	}
	return "earliest", "latest"
}

func formatTimeString(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	formats := []string{
		time.RFC3339,
		time.RFC3339Nano,
		"2006-01-02T15:04:05.000-07:00",
		"2006-01-02T15:04:05.000-0700",
		"2006-01-02T15:04:05-07:00",
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, raw); err == nil {
			return t.Format("01/02 15:04")
		}
	}
	if len(raw) >= 16 && strings.Contains(raw, "T") {
		parts := strings.SplitN(raw, "T", 2)
		dateParts := strings.Split(parts[0], "-")
		timeParts := parts[1]
		if len(dateParts) == 3 && len(timeParts) >= 5 {
			return dateParts[1] + "/" + dateParts[2] + " " + timeParts[:5]
		}
	}
	if len(raw) > 11 {
		return raw[:11]
	}
	return raw
}

func formatMetric(v float64) string {
	if v >= 1_000_000 {
		return fmt.Sprintf("%.1fM", v/1_000_000)
	}
	if v >= 1_000 {
		return fmt.Sprintf("%.1fk", v/1_000)
	}
	if v == float64(int64(v)) {
		return fmt.Sprintf("%d", int64(v))
	}
	return fmt.Sprintf("%.1f", v)
}
