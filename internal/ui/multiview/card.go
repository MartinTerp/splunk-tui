package multiview

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"splunk-tui/internal/config"
	"splunk-tui/internal/spl"
	"splunk-tui/internal/splunk"
	"splunk-tui/internal/ui/browser"
	"splunk-tui/internal/ui/theme"
)

// CardPollResultMsg conveys polling updates for a specific card.
type CardPollResultMsg struct {
	CardIndex int
	SID       string
	Status    *splunk.JobStatus
	Timeline  []splunk.TimelineBucket
	Results   *splunk.ResultsResponse
	Err       error
}

type cardPollTickMsg struct {
	cardIndex int
	sid       string
}

// InstanceCardModel manages a single Splunk instance's execution and visualization.
type InstanceCardModel struct {
	index          int
	bookmark       config.Bookmark
	client         *splunk.Client
	serverInfo     *splunk.ServerInfo
	theme          theme.Theme
	spinner        spinner.Model
	isSearching    bool
	currentSID     string
	activeQuery    string
	activeSpan     string
	jobStatus      *splunk.JobStatus
	timeline       []splunk.TimelineBucket
	results        *splunk.ResultsResponse
	err            error
	cmdType        spl.MultiViewCommandType
	activeType     spl.ChartType
	availableTypes []spl.ChartType
	tableCols      []string
	focused        bool
	width          int
	height         int
}

// NewInstanceCard creates a card model for an instance.
func NewInstanceCard(idx int, conn browser.InstanceConnection, th theme.Theme) InstanceCardModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(th.Primary).Bold(true)

	card := InstanceCardModel{
		index:      idx,
		bookmark:   conn.Bookmark,
		client:     conn.Client,
		serverInfo: conn.Info,
		theme:      th,
		spinner:    s,
		err:        conn.Err,
		height:     10,
	}

	return card
}

// Index returns the card index.
func (c InstanceCardModel) Index() int {
	return c.index
}

// InstanceName returns the bookmark display name.
func (c InstanceCardModel) InstanceName() string {
	return c.bookmark.Name
}

// SetDimensions sets allocated width and height.
func (c *InstanceCardModel) SetDimensions(w, h int) {
	c.width = w
	c.height = h
}

// SetFocus sets focused state.
func (c *InstanceCardModel) SetFocus(f bool) {
	c.focused = f
}

// CycleChartType cycles available visualizations.
func (c *InstanceCardModel) CycleChartType() {
	if len(c.availableTypes) <= 1 {
		return
	}
	for i, t := range c.availableTypes {
		if t == c.activeType {
			c.activeType = c.availableTypes[(i+1)%len(c.availableTypes)]
			return
		}
	}
	c.activeType = c.availableTypes[0]
}

// SetActiveType sets an explicit visualization type.
func (c *InstanceCardModel) SetActiveType(t spl.ChartType) {
	for _, avail := range c.availableTypes {
		if avail == t {
			c.activeType = t
			return
		}
	}
}

// ActiveType returns the currently active visualization type.
func (c InstanceCardModel) ActiveType() spl.ChartType {
	return c.activeType
}

// AvailableTypes returns the list of compatible visualization types.
func (c InstanceCardModel) AvailableTypes() []spl.ChartType {
	return c.availableTypes
}

// RunSearch initiates an async search job on this instance.
func (c *InstanceCardModel) RunSearch(query string, earliest, latest string) tea.Cmd {
	// Cancel running job if present
	if c.currentSID != "" && c.client != nil {
		oldSID := c.currentSID
		go func(cli *splunk.Client, sid string) {
			_ = cli.CancelJob(context.Background(), sid)
		}(c.client, oldSID)
	}

	c.activeQuery = query
	c.activeSpan = spl.ExtractTimechartSpan(query)
	c.results = nil
	c.timeline = nil
	c.jobStatus = nil
	c.err = nil

	// Detect command type
	_, cmdType, baseChart, _ := spl.IsMultiViewSupported(query)
	c.cmdType = cmdType
	c.activeType = baseChart
	c.availableTypes = spl.CompatibleChartTypes(baseChart)
	if cmdType == spl.MultiViewTable {
		c.tableCols = spl.ExtractTableColumns(query)
	} else {
		c.tableCols = nil
	}

	if c.client == nil || c.err != nil {
		c.isSearching = false
		return nil
	}

	c.isSearching = true
	cardIdx := c.index
	cli := c.client

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()

		sid, err := cli.CreateJob(ctx, splunk.CreateJobOptions{
			Search:        query,
			EarliestTime:  earliest,
			LatestTime:    latest,
			StatusBuckets: 150,
			Verbose:       true,
		})
		if err != nil {
			return CardPollResultMsg{CardIndex: cardIdx, Err: err}
		}

		return cardPollTickMsg{cardIndex: cardIdx, sid: sid}
	}
}

// Cancel cancels the active search job on this instance.
func (c *InstanceCardModel) Cancel() {
	if c.isSearching && c.currentSID != "" && c.client != nil {
		sid := c.currentSID
		c.isSearching = false
		go func(cli *splunk.Client, s string) {
			_ = cli.CancelJob(context.Background(), s)
		}(c.client, sid)
	}
}

// PollStatus queries job progress and retrieves results when ready.
func (c *InstanceCardModel) PollStatus(sid string) tea.Cmd {
	if c.client == nil {
		return nil
	}
	c.currentSID = sid
	cli := c.client
	cardIdx := c.index
	q := c.activeQuery

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		status, err := cli.GetJobStatus(ctx, sid)
		if err != nil {
			return CardPollResultMsg{CardIndex: cardIdx, SID: sid, Err: err}
		}

		res := CardPollResultMsg{
			CardIndex: cardIdx,
			SID:       sid,
			Status:    status,
		}

		// Fetch timeline if available
		if tl, err := cli.GetTimeline(ctx, sid); err == nil && len(tl) > 0 {
			res.Timeline = tl
		}

		// Fetch transforming results or table rows
		if status.ResultCount > 0 || status.IsDone {
			count := 50
			if resTable, err := cli.GetResults(ctx, sid, 0, count); err == nil && resTable != nil {
				res.Results = resTable
			}
		}

		_ = q
		return res
	}
}

// Update handles tick and polling messages for this card.
func (c InstanceCardModel) Update(msg tea.Msg) (InstanceCardModel, tea.Cmd) {
	switch msg := msg.(type) {
	case cardPollTickMsg:
		if msg.cardIndex == c.index && c.isSearching {
			c.currentSID = msg.sid
			return c, c.PollStatus(msg.sid)
		}

	case CardPollResultMsg:
		if msg.CardIndex != c.index {
			return c, nil
		}

		if msg.Err != nil {
			c.isSearching = false
			c.err = msg.Err
			return c, nil
		}

		if msg.Status != nil {
			c.jobStatus = msg.Status
			c.timeline = msg.Timeline
			if errMsg := msg.Status.ErrorMessage(); errMsg != "" || msg.Status.IsFailed {
				c.isSearching = false
				if errMsg == "" {
					errMsg = "Search job failed"
				}
				c.err = fmt.Errorf("%s", errMsg)
				return c, nil
			}
			if msg.Results != nil {
				c.results = msg.Results
				if c.activeSpan == "" && len(msg.Results.Results) > 0 {
					if spanStr, ok := msg.Results.Results[0]["_span"]; ok && spanStr != "" {
						if sec, err := strconv.ParseInt(spanStr, 10, 64); err == nil {
							c.activeSpan = spl.FormatSpanSeconds(sec)
						}
					}
				}
			}

			if !msg.Status.IsDone && !msg.Status.IsFailed {
				// Continue polling after 500ms
				cardIdx := c.index
				sid := msg.SID
				return c, tea.Tick(500*time.Millisecond, func(time.Time) tea.Msg {
					return cardPollTickMsg{cardIndex: cardIdx, sid: sid}
				})
			}

			c.isSearching = false
		}
	}

	return c, nil
}

// View renders the card interface.
func (c InstanceCardModel) View() string {
	boxW := c.width - 4
	if boxW < 24 {
		boxW = 24
	}
	boxH := c.height
	if boxH < 8 {
		boxH = 8
	}
	availInner := boxH - 2
	if availInner < 4 {
		availInner = 4
	}

	cardStyle := c.theme.Card
	if c.focused {
		cardStyle = c.theme.FocusedCard
	}

	// Inner content width inside Card borders (2 cols) and padding (2 cols)
	contentW := boxW - 2
	if contentW < 20 {
		contentW = 20
	}

	// 1. Single header line: Instance Name + Status Line (URL removed, status moved to first line)
	titleText := fmt.Sprintf("⚡ %s", c.bookmark.Name)
	if c.activeSpan != "" && c.cmdType == spl.MultiViewTimechart {
		titleText += fmt.Sprintf(" (span: %s)", c.activeSpan)
	}
	title := lipgloss.NewStyle().Foreground(c.theme.Primary).Bold(true).Render(titleText)
	titleW := lipgloss.Width(title)
	statusW := contentW - titleW - 2
	if statusW < 10 {
		statusW = 10
	}
	status := c.renderStatusLine(statusW)
	header := lipgloss.JoinHorizontal(lipgloss.Center, title, "  ", status)
	if lipgloss.Width(header) > contentW {
		header = truncate(header, contentW)
	}

	// 2. Visualization Content (takes full remaining inner height)
	contentH := availInner - 1
	if contentH < 2 {
		contentH = 2
	}
	content := c.renderContent(contentW, contentH)

	body := lipgloss.JoinVertical(lipgloss.Left, header, content)
	return cardStyle.Width(boxW).Height(availInner).MaxHeight(boxH).Render(body)
}

func (c InstanceCardModel) renderStatusLine(width int) string {
	if c.err != nil {
		errStr := "✖ FAILED"
		if width >= 15 {
			errStr = "✖ Search Failed"
		}
		return lipgloss.NewStyle().Foreground(c.theme.Error).Bold(true).Render(errStr)
	}

	// Build visualization pills if multiple options exist (like main splunk view)
	var pillsStr string
	if len(c.availableTypes) > 1 {
		var pills []string
		for _, t := range c.availableTypes {
			lbl := t.Label()
			if t == c.activeType {
				pills = append(pills, c.theme.ActiveButton.Render("● "+lbl))
			} else {
				pills = append(pills, c.theme.Button.Render("○ "+lbl))
			}
		}
		pillsStr = strings.Join(pills, " ")
	}
	pillsW := lipgloss.Width(pillsStr)

	if c.isSearching {
		pct := 0
		if c.jobStatus != nil {
			pct = int(math.Round(c.jobStatus.DoneProgress * 100))
		}
		spinStr := fmt.Sprintf("%s Running %d%%", c.spinner.View(), pct)
		progBar := renderProgressBar(pct, 8)
		line := fmt.Sprintf("%s %s", spinStr, progBar)
		styledLine := lipgloss.NewStyle().Foreground(c.theme.Highlight).Render(line)
		if pillsStr != "" && width >= lipgloss.Width(styledLine)+pillsW+2 {
			return lipgloss.JoinHorizontal(lipgloss.Center, styledLine, "  ", pillsStr)
		}
		return truncate(styledLine, width)
	}

	if c.jobStatus != nil && c.jobStatus.IsDone {
		var statText string
		if c.jobStatus.ResultCount > 0 && c.jobStatus.ResultCount != c.jobStatus.EventCount {
			if width >= 50 {
				statText = fmt.Sprintf("✔ DONE • %d results (%d events) • %.2fs", c.jobStatus.ResultCount, c.jobStatus.EventCount, c.jobStatus.RunDuration)
			} else if width >= 30 {
				statText = fmt.Sprintf("✔ DONE • %d results • %.2fs", c.jobStatus.ResultCount, c.jobStatus.RunDuration)
			} else {
				statText = "✔ DONE"
			}
		} else {
			if width >= 40 {
				statText = fmt.Sprintf("✔ DONE • %d events • %.2fs", c.jobStatus.EventCount, c.jobStatus.RunDuration)
			} else if width >= 25 {
				statText = fmt.Sprintf("✔ DONE • %d events", c.jobStatus.EventCount)
			} else {
				statText = "✔ DONE"
			}
		}

		statStyled := lipgloss.NewStyle().Foreground(c.theme.Success).Render(statText)

		if pillsStr != "" {
			if width >= pillsW+lipgloss.Width(statStyled)+2 {
				if width >= pillsW+lipgloss.Width(statStyled)+14 {
					helpStr := lipgloss.NewStyle().Foreground(c.theme.SubtleText).Render("(v: switch)")
					return lipgloss.JoinHorizontal(lipgloss.Center, statStyled, "  ", pillsStr, " ", helpStr)
				}
				return lipgloss.JoinHorizontal(lipgloss.Center, statStyled, "  ", pillsStr)
			} else if width >= pillsW+8 {
				shortDone := lipgloss.NewStyle().Foreground(c.theme.Success).Render("✔ DONE")
				return lipgloss.JoinHorizontal(lipgloss.Center, shortDone, "  ", pillsStr)
			} else if width >= pillsW {
				return pillsStr
			} else {
				badge := c.theme.ActiveButton.Render("● " + c.activeType.Label())
				if width >= lipgloss.Width(badge)+3 {
					return lipgloss.JoinHorizontal(lipgloss.Center, lipgloss.NewStyle().Foreground(c.theme.Success).Render("✔"), " ", badge)
				}
				return badge
			}
		}

		return truncate(statStyled, width)
	}

	if pillsStr != "" && width >= pillsW+10 {
		return lipgloss.JoinHorizontal(lipgloss.Center, lipgloss.NewStyle().Foreground(c.theme.SubtleText).Render("Ready"), "  ", pillsStr)
	}
	return lipgloss.NewStyle().Foreground(c.theme.SubtleText).Render("Ready for search")
}

func (c InstanceCardModel) renderContent(width, height int) string {
	if c.err != nil {
		errTitle := lipgloss.NewStyle().Foreground(c.theme.Error).Bold(true).Render("✖ Search Error:")
		errMsg := lipgloss.NewStyle().Foreground(c.theme.Error).Width(width).Render(c.err.Error())
		errHint := lipgloss.NewStyle().Foreground(c.theme.SubtleText).Render("Check syntax, arguments, or connection.")
		return lipgloss.JoinVertical(lipgloss.Left, "", errTitle, errMsg, "", errHint)
	}

	if c.isSearching && (c.results == nil || len(c.results.Results) == 0) {
		spinText := fmt.Sprintf("\n  %s Waiting for results from %s...", c.spinner.View(), c.bookmark.Name)
		return lipgloss.NewStyle().Foreground(c.theme.SubtleText).Render(spinText)
	}

	if c.cmdType == spl.MultiViewTable {
		return c.renderMiniTable(width, height)
	}

	switch c.activeType {
	case spl.ChartLine:
		return c.renderLineChart(width, height)
	case spl.ChartSparklines:
		return c.renderSparklines(width, height)
	case spl.ChartBar:
		return c.renderBarChart(width, height)
	case spl.ChartPie:
		return c.renderPieChart(width, height)
	case spl.ChartColumn:
		return c.renderColumnChart(width, height)
	case spl.ChartColumnTime:
		if c.cmdType == spl.MultiViewTimechart {
			return c.renderTimechartColumns(width, height)
		}
		return c.renderColumnChart(width, height)
	default:
		if c.results != nil && len(c.results.Results) > 0 {
			return c.renderMiniTable(width, height)
		}
		return lipgloss.NewStyle().Foreground(c.theme.SubtleText).Render("No chart data available for this query.")
	}
}

// ---------------------------------------------------------------------
// Chart Renderers (Optimized for compact 2-per-row cards)
// ---------------------------------------------------------------------

func (c InstanceCardModel) renderLineChart(width, height int) string {
	seriesCols, allVals := c.extractTimeSeries()

	chartW := width - 7
	if chartW < 12 {
		chartW = 12
	}

	t1, t2 := c.getTimeBounds()
	hasTimeAxis := t1 != "" && t2 != ""

	palette := c.theme.ChartPalette
	if len(palette) == 0 {
		palette = []lipgloss.Color{c.theme.Primary, c.theme.Highlight, c.theme.Warning, c.theme.Secondary, c.theme.Error}
	}

	if len(seriesCols) > 1 {
		numSeries := len(seriesCols)
		if numSeries > 6 {
			numSeries = 6
		}

		allSeriesPts := make([][]float64, numSeries)
		var globalMin, globalMax float64
		first := true
		for s := 0; s < numSeries; s++ {
			pts := resample(allVals[s], chartW)
			allSeriesPts[s] = pts
			sMin, sMax := minMax(pts)
			if first {
				globalMin, globalMax = sMin, sMax
				first = false
			} else {
				if sMin < globalMin {
					globalMin = sMin
				}
				if sMax > globalMax {
					globalMax = sMax
				}
			}
		}
		if globalMin == globalMax {
			globalMax = globalMin + 1
		}

		legendH := 1
		rows := height - 1 - legendH
		if hasTimeAxis {
			rows = height - 2 - legendH
		}
		if rows < 2 {
			rows = 2
		}

		var lines []string
		for r := 0; r < rows; r++ {
			yHigh := globalMax - (float64(r) / float64(rows) * (globalMax - globalMin))
			yLow := globalMax - (float64(r+1) / float64(rows) * (globalMax - globalMin))

			var label string
			if r == 0 {
				label = formatShortMetric(globalMax)
			} else if r == rows-1 {
				label = formatShortMetric(globalMin)
			}
			yAxis := fmt.Sprintf("%5s┤", label)
			axisStyled := lipgloss.NewStyle().Foreground(c.theme.SubtleText).Render(yAxis)

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

			for col := 0; col < chartW; col++ {
				matched := false
				for s := 0; s < numSeries; s++ {
					v := allSeriesPts[s][col]
					if v >= yLow && v <= yHigh {
						sColor := palette[s%len(palette)]
						if sColor != curColor {
							flush()
							curColor = sColor
						}
						curBuf.WriteString("╭")
						matched = true
						break
					}
				}
				if !matched {
					if curColor != "" {
						flush()
						curColor = ""
					}
					curBuf.WriteString(" ")
				}
			}
			flush()

			lines = append(lines, axisStyled+rowChars.String())
		}

		bottomAxis := "     ┼" + strings.Repeat("─", chartW)
		lines = append(lines, lipgloss.NewStyle().Foreground(c.theme.SurfaceBorder).Render(bottomAxis))

		if hasTimeAxis {
			pad := chartW - len(t1) - len(t2)
			if pad < 2 {
				pad = 2
			}
			timeAxis := fmt.Sprintf("      %s%s%s", t1, strings.Repeat(" ", pad), t2)
			lines = append(lines, lipgloss.NewStyle().Foreground(c.theme.SubtleText).Render(timeAxis))
		}

		var legendItems []string
		for s := 0; s < numSeries; s++ {
			col := palette[s%len(palette)]
			swatch := lipgloss.NewStyle().Foreground(col).Render("■")
			name := lipgloss.NewStyle().Foreground(c.theme.Text).Render(truncate(seriesCols[s], 10))
			legendItems = append(legendItems, swatch+" "+name)
		}
		legendStr := fmt.Sprintf("      %s", strings.Join(legendItems, "  "))
		lines = append(lines, truncate(legendStr, width))

		return strings.Join(lines, "\n")
	}

	var series []float64
	if c.results != nil && len(c.results.Results) > 0 {
		valField := findMetricField(c.results)
		for _, r := range c.results.Results {
			if v, err := strconv.ParseFloat(r[valField], 64); err == nil {
				series = append(series, v)
			}
		}
	} else if len(c.timeline) > 0 {
		for _, b := range c.timeline {
			series = append(series, float64(b.Count))
		}
	}

	if len(series) == 0 {
		return lipgloss.NewStyle().Foreground(c.theme.SubtleText).Render("No numeric series data to plot.")
	}

	rows := height - 1
	if hasTimeAxis {
		rows = height - 2
	}
	if rows < 2 {
		rows = 2
	}

	points := resample(series, chartW)
	minVal, maxVal := minMax(points)
	if minVal == maxVal {
		maxVal = minVal + 1
	}

	// Render Braille curve lines
	var lines []string
	for r := 0; r < rows; r++ {
		yHigh := maxVal - (float64(r) / float64(rows) * (maxVal - minVal))
		yLow := maxVal - (float64(r+1) / float64(rows) * (maxVal - minVal))

		var label string
		if r == 0 {
			label = formatShortMetric(maxVal)
		} else if r == rows-1 {
			label = formatShortMetric(minVal)
		}
		yAxis := fmt.Sprintf("%5s┤", label)

		var curve strings.Builder
		for _, v := range points {
			if v >= yLow && v <= yHigh {
				curve.WriteString("╭")
			} else if v > yHigh {
				curve.WriteString("│")
			} else {
				curve.WriteString(" ")
			}
		}

		lineStr := lipgloss.NewStyle().Foreground(c.theme.SubtleText).Render(yAxis) +
			lipgloss.NewStyle().Foreground(c.theme.Primary).Render(curve.String())
		lines = append(lines, lineStr)
	}

	// Bottom axis line
	bottomAxis := "     ┼" + strings.Repeat("─", chartW)
	lines = append(lines, lipgloss.NewStyle().Foreground(c.theme.SurfaceBorder).Render(bottomAxis))

	// Time boundaries
	if hasTimeAxis {
		pad := chartW - len(t1) - len(t2)
		if pad < 2 {
			pad = 2
		}
		timeAxis := fmt.Sprintf("      %s%s%s", t1, strings.Repeat(" ", pad), t2)
		lines = append(lines, lipgloss.NewStyle().Foreground(c.theme.SubtleText).Render(timeAxis))
	}

	return strings.Join(lines, "\n")
}

func (c InstanceCardModel) renderSparklines(width, height int) string {
	blocks := []rune{' ', '▂', '▃', '▄', '▅', '▆', '▇', '█'}
	var series []float64

	if c.results != nil && len(c.results.Results) > 0 {
		valField := findMetricField(c.results)
		for _, r := range c.results.Results {
			if v, err := strconv.ParseFloat(r[valField], 64); err == nil {
				series = append(series, v)
			}
		}
	} else if len(c.timeline) > 0 {
		for _, b := range c.timeline {
			series = append(series, float64(b.Count))
		}
	}

	if len(series) == 0 {
		return lipgloss.NewStyle().Foreground(c.theme.SubtleText).Render("No time series data.")
	}

	chartW := width - 2
	if chartW < 10 {
		chartW = 10
	}
	pts := resample(series, chartW)
	minVal, maxVal := minMax(pts)
	rng := maxVal - minVal
	if rng <= 0 {
		rng = 1
	}

	var sb strings.Builder
	for _, v := range pts {
		idx := int((v - minVal) / rng * float64(len(blocks)-1))
		if idx < 0 {
			idx = 0
		}
		if idx >= len(blocks) {
			idx = len(blocks) - 1
		}
		sb.WriteRune(blocks[idx])
	}

	sparkLine := lipgloss.NewStyle().Foreground(c.theme.Primary).Bold(true).Render(sb.String())
	stats := fmt.Sprintf("min: %s  max: %s", formatShortMetric(minVal), formatShortMetric(maxVal))
	statsStyled := lipgloss.NewStyle().Foreground(c.theme.SubtleText).Render(stats)

	t1, t2 := c.getTimeBounds()
	hasTimeAxis := t1 != "" && t2 != ""

	var lines []string
	topPad := (height - 2) / 2
	if hasTimeAxis {
		topPad = (height - 3) / 2
	}
	if topPad < 0 {
		topPad = 0
	}
	for i := 0; i < topPad; i++ {
		lines = append(lines, "")
	}
	lines = append(lines, sparkLine, statsStyled)
	if hasTimeAxis {
		pad := chartW - len(t1) - len(t2)
		if pad < 2 {
			pad = 2
		}
		timeAxis := fmt.Sprintf("%s%s%s", t1, strings.Repeat(" ", pad), t2)
		lines = append(lines, lipgloss.NewStyle().Foreground(c.theme.SubtleText).Render(timeAxis))
	}

	return strings.Join(lines, "\n")
}

func (c InstanceCardModel) renderTimechartColumns(width, height int) string {
	blocks := []rune{' ', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

	// Check if multi-series timechart results exist
	seriesCols, allVals := c.extractTimeSeries()

	chartW := width - 7
	if chartW < 10 {
		chartW = 10
	}

	palette := c.theme.ChartPalette
	if len(palette) == 0 {
		palette = []lipgloss.Color{c.theme.Primary, c.theme.Highlight, c.theme.Warning, c.theme.Secondary, c.theme.Error}
	}

	t1, t2 := c.getTimeBounds()
	hasTimeAxis := t1 != "" && t2 != ""

	// If multi-series (e.g. timechart count BY host/sourcetype)
	if len(seriesCols) > 1 {
		numSeries := len(seriesCols)
		if numSeries > 6 {
			numSeries = 6
		}
		numBins := len(allVals[0])

		colSeriesVals := make([][]float64, chartW)
		for col := 0; col < chartW; col++ {
			colSeriesVals[col] = make([]float64, numSeries)
		}
		colTotals := make([]float64, chartW)

		ratio := float64(numBins) / float64(chartW)
		for col := 0; col < chartW; col++ {
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
					sSum += allVals[s][i]
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

		legendH := 1
		plotRows := height - 1 - legendH
		if hasTimeAxis {
			plotRows = height - 2 - legendH
		}
		if plotRows < 2 {
			plotRows = 2
		}

		colCumVals := make([][]float64, chartW)
		for col := 0; col < chartW; col++ {
			cum := make([]float64, numSeries+1)
			cum[0] = 0
			for s := 0; s < numSeries; s++ {
				cum[s+1] = cum[s] + colSeriesVals[col][s]
			}
			colCumVals[col] = cum
		}

		var lines []string
		for row := plotRows; row >= 1; row-- {
			rowMin := float64(row-1) / float64(plotRows) * maxVal
			rowMax := float64(row) / float64(plotRows) * maxVal

			var label string
			if row == plotRows {
				label = formatShortMetric(maxVal)
			} else if row == 1 {
				label = formatShortMetric(0)
			}
			axisLabel := fmt.Sprintf("%5s┤", label)
			axisStyled := lipgloss.NewStyle().Foreground(c.theme.SubtleText).Render(axisLabel)

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

			for col := 0; col < chartW; col++ {
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
					fraction := (tot - rowMin) / (rowMax - rowMin)
					idx := int(fraction * float64(len(blocks)-1))
					if idx >= len(blocks) {
						idx = len(blocks) - 1
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
					curBuf.WriteRune(blocks[idx])
				}
			}
			flush()

			lines = append(lines, axisStyled+rowChars.String())
		}

		bottomAxis := "     ┼" + strings.Repeat("─", chartW)
		lines = append(lines, lipgloss.NewStyle().Foreground(c.theme.SurfaceBorder).Render(bottomAxis))

		if hasTimeAxis {
			pad := chartW - len(t1) - len(t2)
			if pad < 2 {
				pad = 2
			}
			timeAxis := fmt.Sprintf("      %s%s%s", t1, strings.Repeat(" ", pad), t2)
			lines = append(lines, lipgloss.NewStyle().Foreground(c.theme.SubtleText).Render(timeAxis))
		}

		var legendItems []string
		for s := 0; s < numSeries; s++ {
			col := palette[s%len(palette)]
			swatch := lipgloss.NewStyle().Foreground(col).Render("■")
			name := lipgloss.NewStyle().Foreground(c.theme.Text).Render(truncate(seriesCols[s], 10))
			legendItems = append(legendItems, swatch+" "+name)
		}
		legendStr := fmt.Sprintf("      %s", strings.Join(legendItems, "  "))
		lines = append(lines, truncate(legendStr, width))

		return strings.Join(lines, "\n")
	}

	// Single-series fallback
	var series []float64
	if c.results != nil && len(c.results.Results) > 0 {
		valField := findMetricField(c.results)
		for _, r := range c.results.Results {
			if v, err := strconv.ParseFloat(r[valField], 64); err == nil {
				series = append(series, v)
			}
		}
	} else if len(c.timeline) > 0 {
		for _, b := range c.timeline {
			series = append(series, float64(b.Count))
		}
	}

	if len(series) == 0 {
		return lipgloss.NewStyle().Foreground(c.theme.SubtleText).Render("No time series data.")
	}

	pts := resample(series, chartW)
	minVal, maxVal := minMax(pts)
	if minVal == maxVal {
		maxVal = minVal + 1
	}

	plotRows := height - 1
	if hasTimeAxis {
		plotRows = height - 2
	}
	if plotRows < 2 {
		plotRows = 2
	}

	var lines []string
	for row := plotRows; row >= 1; row-- {
		rowMin := float64(row-1) / float64(plotRows) * maxVal
		rowMax := float64(row) / float64(plotRows) * maxVal

		var sb strings.Builder
		for _, v := range pts {
			if v < rowMin {
				sb.WriteRune(' ')
			} else if v >= rowMax {
				sb.WriteRune('█')
			} else {
				fraction := (v - rowMin) / (rowMax - rowMin)
				idx := int(fraction * float64(len(blocks)-1))
				if idx >= len(blocks) {
					idx = len(blocks) - 1
				}
				sb.WriteRune(blocks[idx])
			}
		}

		var label string
		if row == plotRows {
			label = formatShortMetric(maxVal)
		} else if row == 1 {
			label = formatShortMetric(0)
		}
		axisLabel := fmt.Sprintf("%5s┤", label)
		axisStyled := lipgloss.NewStyle().Foreground(c.theme.SubtleText).Render(axisLabel)
		colStyled := lipgloss.NewStyle().Foreground(c.theme.Primary).Render(sb.String())
		lines = append(lines, axisStyled+colStyled)
	}

	bottomAxis := "     ┼" + strings.Repeat("─", chartW)
	lines = append(lines, lipgloss.NewStyle().Foreground(c.theme.SurfaceBorder).Render(bottomAxis))

	if hasTimeAxis {
		pad := chartW - len(t1) - len(t2)
		if pad < 2 {
			pad = 2
		}
		timeAxis := fmt.Sprintf("      %s%s%s", t1, strings.Repeat(" ", pad), t2)
		lines = append(lines, lipgloss.NewStyle().Foreground(c.theme.SubtleText).Render(timeAxis))
	}

	return strings.Join(lines, "\n")
}

func (c InstanceCardModel) renderBarChart(width, height int) string {
	items := c.extractCategoricalData()
	if len(items) == 0 {
		return lipgloss.NewStyle().Foreground(c.theme.SubtleText).Render("No categorical items to display.")
	}

	displayCount := height
	if displayCount > len(items) {
		displayCount = len(items)
	}
	if displayCount < 1 {
		displayCount = 1
	}

	var maxVal float64
	for _, it := range items {
		if it.val > maxVal {
			maxVal = it.val
		}
	}
	if maxVal <= 0 {
		maxVal = 1
	}

	labelW := 12
	barMaxW := width - labelW - 14
	if barMaxW < 8 {
		barMaxW = 8
	}

	palette := c.theme.ChartPalette
	if len(palette) == 0 {
		palette = []lipgloss.Color{c.theme.Primary, c.theme.Highlight}
	}

	var lines []string
	for i := 0; i < displayCount; i++ {
		it := items[i]
		color := palette[i%len(palette)]

		lbl := truncate(it.name, labelW)
		lblPadded := lbl + strings.Repeat(" ", labelW-lipgloss.Width(lbl))

		barLen := int(math.Round((it.val / maxVal) * float64(barMaxW)))
		if barLen < 1 && it.val > 0 {
			barLen = 1
		}
		barStr := strings.Repeat("█", barLen)
		barStyled := lipgloss.NewStyle().Foreground(color).Render(barStr)

		valStr := fmt.Sprintf(" %s", formatShortMetric(it.val))
		valStyled := lipgloss.NewStyle().Foreground(c.theme.SubtleText).Render(valStr)

		lines = append(lines, fmt.Sprintf("%s %s%s", lblPadded, barStyled, valStyled))
	}

	return strings.Join(lines, "\n")
}

func (c InstanceCardModel) renderPieChart(width, height int) string {
	items := c.extractCategoricalData()
	if len(items) == 0 {
		return lipgloss.NewStyle().Foreground(c.theme.SubtleText).Render("No category data for pie chart.")
	}

	var total float64
	for _, it := range items {
		total += it.val
	}
	if total <= 0 {
		total = 1
	}

	palette := c.theme.ChartPalette
	if len(palette) == 0 {
		palette = []lipgloss.Color{c.theme.Primary, c.theme.Highlight}
	}

	// Compact glyph on left
	glyphLines := []string{
		" ╭──╮ ",
		" │◔ │ ",
		" ╰──╯ ",
	}
	glyphStr := lipgloss.NewStyle().Foreground(c.theme.Primary).Render(strings.Join(glyphLines, "\n"))

	// Legend rows on right
	var legendLines []string
	displayCount := height
	if displayCount > len(items) {
		displayCount = len(items)
	}
	if displayCount < 1 {
		displayCount = 1
	}

	for i := 0; i < displayCount; i++ {
		it := items[i]
		color := palette[i%len(palette)]
		pct := (it.val / total) * 100
		swatch := lipgloss.NewStyle().Foreground(color).Render("■")
		name := truncate(it.name, 12)
		pctStr := fmt.Sprintf("%4.1f%%", pct)
		row := fmt.Sprintf("%s %-12s %s", swatch, name, lipgloss.NewStyle().Foreground(color).Bold(true).Render(pctStr))
		legendLines = append(legendLines, row)
	}

	return lipgloss.JoinHorizontal(lipgloss.Center, glyphStr, "  ", strings.Join(legendLines, "\n"))
}

func (c InstanceCardModel) renderColumnChart(width, height int) string {
	items := c.extractCategoricalData()
	if len(items) == 0 {
		return lipgloss.NewStyle().Foreground(c.theme.SubtleText).Render("No categorical data.")
	}

	displayCount := (width - 4) / 7
	if displayCount < 2 {
		displayCount = 2
	}
	if displayCount > len(items) {
		displayCount = len(items)
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

	blocks := []rune{' ', '▂', '▄', '▆', '█'}
	colH := height - 1
	if colH < 2 {
		colH = 2
	}

	var grid [][]string
	for r := 0; r < colH; r++ {
		grid = append(grid, make([]string, displayCount))
	}

	palette := c.theme.ChartPalette
	if len(palette) == 0 {
		palette = []lipgloss.Color{c.theme.Primary}
	}

	for cIdx := 0; cIdx < displayCount; cIdx++ {
		val := items[cIdx].val
		color := palette[cIdx%len(palette)]
		frac := val / maxVal
		fullBlocks := int(frac * float64(colH))

		for r := 0; r < colH; r++ {
			rowFromBottom := (colH - 1) - r
			if rowFromBottom < fullBlocks {
				grid[r][cIdx] = lipgloss.NewStyle().Foreground(color).Render("████")
			} else if rowFromBottom == fullBlocks {
				remFrac := (frac * float64(colH)) - float64(fullBlocks)
				subIdx := int(remFrac * float64(len(blocks)-1))
				grid[r][cIdx] = lipgloss.NewStyle().Foreground(color).Render(string(blocks[subIdx]) + string(blocks[subIdx]) + string(blocks[subIdx]) + string(blocks[subIdx]))
			} else {
				grid[r][cIdx] = "    "
			}
		}
	}

	var outLines []string
	for _, row := range grid {
		outLines = append(outLines, strings.Join(row, " "))
	}

	// Bottom labels
	var labels []string
	for i := 0; i < displayCount; i++ {
		lbl := truncate(items[i].name, 4)
		labels = append(labels, fmt.Sprintf("%-4s", lbl))
	}
	outLines = append(outLines, lipgloss.NewStyle().Foreground(c.theme.SubtleText).Render(strings.Join(labels, " ")))

	return strings.Join(outLines, "\n")
}

func (c InstanceCardModel) renderMiniTable(width, height int) string {
	if c.results == nil || len(c.results.Results) == 0 {
		return lipgloss.NewStyle().Foreground(c.theme.SubtleText).Render("No table rows returned.")
	}

	headers := c.results.Fields
	if len(c.tableCols) > 0 {
		headers = c.tableCols
	}
	if len(headers) == 0 && len(c.results.Results) > 0 {
		for k := range c.results.Results[0] {
			headers = append(headers, k)
		}
	}

	if len(headers) == 0 {
		return lipgloss.NewStyle().Foreground(c.theme.SubtleText).Render("No columns in table.")
	}

	// Fit columns to fill width
	maxCols := width / 14
	if maxCols < 1 {
		maxCols = 1
	}
	if maxCols > 6 {
		maxCols = 6
	}
	if len(headers) < maxCols {
		maxCols = len(headers)
	}
	visHeaders := headers[:maxCols]
	colW := (width - (maxCols - 1)) / maxCols
	if colW < 8 {
		colW = 8
	}

	var headerCells []string
	for _, h := range visHeaders {
		headerCells = append(headerCells, c.theme.TableHeader.Width(colW).Render(truncate(h, colW)))
	}
	headerRow := strings.Join(headerCells, " ")

	var rows []string
	rows = append(rows, headerRow)

	displayRows := height - 1
	if displayRows > len(c.results.Results) {
		displayRows = len(c.results.Results)
	}
	if displayRows < 1 {
		displayRows = 1
	}

	for r := 0; r < displayRows; r++ {
		rowMap := c.results.Results[r]
		var cells []string
		for _, h := range visHeaders {
			val := rowMap[h]
			cellStr := truncate(val, colW)
			cells = append(cells, c.theme.TableRow.Width(colW).Render(cellStr))
		}
		rows = append(rows, strings.Join(cells, " "))
	}

	return strings.Join(rows, "\n")
}

// ---------------------------------------------------------------------
// Data Extraction Helpers
// ---------------------------------------------------------------------

type catItem struct {
	name string
	val  float64
}

func (c InstanceCardModel) extractCategoricalData() []catItem {
	if c.results == nil || len(c.results.Results) == 0 {
		return nil
	}

	// Look for category label field and metric value field
	fields := c.results.Fields
	catField := ""
	valField := ""

	for _, f := range fields {
		if strings.HasPrefix(f, "_") {
			continue
		}
		if valField == "" && (strings.HasPrefix(f, "count") || strings.HasPrefix(f, "sum") || strings.HasPrefix(f, "avg")) {
			valField = f
		} else if catField == "" {
			catField = f
		}
	}

	if catField == "" && len(fields) > 0 {
		catField = fields[0]
	}
	if valField == "" && len(fields) > 1 {
		valField = fields[1]
	}

	var items []catItem
	for _, r := range c.results.Results {
		name := r[catField]
		if name == "" {
			name = "unknown"
		}
		val, _ := strconv.ParseFloat(r[valField], 64)
		items = append(items, catItem{name: name, val: val})
	}

	// Separate existing "other" if present
	var nonOther []catItem
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

	sort.SliceStable(nonOther, func(i, j int) bool {
		return nonOther[i].val > nonOther[j].val
	})

	limit, useOther := spl.EffectiveLimit(c.activeQuery)
	if limit > 0 && len(nonOther) > limit {
		topItems := append([]catItem(nil), nonOther[:limit]...)
		if useOther {
			otherVal := existingOtherVal
			for _, rem := range nonOther[limit:] {
				otherVal += rem.val
			}
			topItems = append(topItems, catItem{name: "other", val: otherVal})
		}
		return topItems
	}

	if hasOther {
		return append(nonOther, catItem{name: "other", val: existingOtherVal})
	}
	return nonOther
}

func (c InstanceCardModel) extractTimeSeries() ([]string, [][]float64) {
	if c.results == nil || len(c.results.Results) == 0 {
		return nil, nil
	}

	var seriesCols []string
	for _, f := range c.results.Fields {
		if !strings.HasPrefix(f, "_") {
			seriesCols = append(seriesCols, f)
		}
	}
	if len(seriesCols) == 0 {
		return nil, nil
	}

	numRows := len(c.results.Results)

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
		for i, r := range c.results.Results {
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

	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].total > candidates[j].total
	})

	limit, useOther := spl.EffectiveLimit(c.activeQuery)
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

	var finalCols []string
	var finalVals [][]float64
	for _, cand := range candidates {
		finalCols = append(finalCols, cand.name)
		finalVals = append(finalVals, cand.vals)
	}
	if hasExistingOther {
		finalCols = append(finalCols, "other")
		finalVals = append(finalVals, existingOtherVals)
	}
	return finalCols, finalVals
}

func findMetricField(res *splunk.ResultsResponse) string {
	if res == nil || len(res.Fields) == 0 {
		return "count"
	}
	for _, f := range res.Fields {
		if !strings.HasPrefix(f, "_") && f != "host" && f != "sourcetype" {
			return f
		}
	}
	for _, f := range res.Fields {
		if !strings.HasPrefix(f, "_") {
			return f
		}
	}
	return res.Fields[len(res.Fields)-1]
}

func resample(series []float64, targetLen int) []float64 {
	if len(series) == 0 || targetLen <= 0 {
		return nil
	}
	if len(series) == targetLen {
		return series
	}
	res := make([]float64, targetLen)
	if len(series) == 1 {
		for i := 0; i < targetLen; i++ {
			res[i] = series[0]
		}
		return res
	}
	step := float64(len(series)-1) / float64(targetLen-1)
	for i := 0; i < targetLen; i++ {
		srcIdx := int(math.Round(float64(i) * step))
		if srcIdx >= len(series) {
			srcIdx = len(series) - 1
		}
		res[i] = series[srcIdx]
	}
	return res
}

func minMax(vals []float64) (float64, float64) {
	if len(vals) == 0 {
		return 0, 0
	}
	min := vals[0]
	max := vals[0]
	for _, v := range vals {
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}
	return min, max
}

func formatShortMetric(v float64) string {
	if math.Abs(v) >= 1_000_000 {
		return fmt.Sprintf("%.1fM", v/1_000_000)
	}
	if math.Abs(v) >= 1_000 {
		return fmt.Sprintf("%.1fk", v/1_000)
	}
	return fmt.Sprintf("%.0f", v)
}

func renderProgressBar(pct, width int) string {
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	filled := int(math.Round(float64(pct) / 100.0 * float64(width)))
	if filled > width {
		filled = width
	}
	empty := width - filled
	return "[" + strings.Repeat("█", filled) + strings.Repeat("░", empty) + "]"
}

func (c InstanceCardModel) getTimeBounds() (string, string) {
	if c.cmdType == spl.MultiViewTimechart && c.results != nil && len(c.results.Results) > 0 {
		r0 := c.results.Results[0]["_time"]
		rn := c.results.Results[len(c.results.Results)-1]["_time"]
		if r0 != "" && rn != "" {
			return formatTimeString(r0), formatTimeString(rn)
		}
	}
	if len(c.timeline) > 0 {
		t1 := c.timeline[0].Time.Format("01/02 15:04")
		t2 := c.timeline[len(c.timeline)-1].Time.Format("01/02 15:04")
		return t1, t2
	}
	if c.results != nil && len(c.results.Results) > 0 {
		r0 := c.results.Results[0]["_time"]
		rn := c.results.Results[len(c.results.Results)-1]["_time"]
		if r0 != "" && rn != "" {
			return formatTimeString(r0), formatTimeString(rn)
		}
	}
	return "", ""
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
