package search

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"splunk-tui/internal/config"
	"splunk-tui/internal/spl"
	"splunk-tui/internal/splunk"
	"splunk-tui/internal/ui/theme"
)

type focusPanel int

const (
	focusSearch focusPanel = iota
	focusFields
	focusTable
)

type pollTickMsg struct {
	sid string
}

type jobCreatedMsg struct {
	sid   string
	query string
}

type jobCreateErrorMsg struct {
	err error
}

type jobPollResultMsg struct {
	sid      string
	status   *splunk.JobStatus
	events   *splunk.EventsResponse
	results  *splunk.ResultsResponse
	summary  *splunk.SummaryResponse
	timeline []splunk.TimelineBucket
	err      error
}

// SearchModel manages the main Splunk search workspace.
type SearchModel struct {
	theme         theme.Theme
	cfg           *config.Config
	bookmark      config.Bookmark
	client        *splunk.Client
	serverInfo    *splunk.ServerInfo

	currentSID    string
	activeQuery   string
	isSearching   bool
	pageSize      int
	page          int
	totalEvents   int64

	focus         focusPanel
	chartPane     ChartPaneModel
	searchBar     SearchBarModel
	timelineBar   TimelineBarModel
	fieldsSidebar FieldsSidebarModel
	table         TableModel
	detail        *DetailModel
	configModal   *ConfigExplorerModel
	statusBar     StatusBarModel

	width         int
	height        int
}

// New creates a search dashboard model for an active connection.
func New(th theme.Theme, cfg *config.Config, bm config.Bookmark, client *splunk.Client, info *splunk.ServerInfo) SearchModel {
	pageSize := cfg.PageSize
	if pageSize <= 0 {
		pageSize = 50
	}

	chartPane := NewChartPane(th)
	searchBar := NewSearchBar(th, cfg.DefaultTimeRange)
	timelineBar := NewTimelineBar(th)
	fieldsSidebar := NewFieldsSidebar(th)
	table := NewTable(th, pageSize)
	statusBar := NewStatusBar(th)

	m := SearchModel{
		theme:         th,
		cfg:           cfg,
		bookmark:      bm,
		client:        client,
		serverInfo:    info,
		pageSize:      pageSize,
		page:          1,
		focus:         focusSearch,
		width:         80,
		height:        24,
		chartPane:     chartPane,
		searchBar:     searchBar,
		timelineBar:   timelineBar,
		fieldsSidebar: fieldsSidebar,
		table:         table,
		statusBar:     statusBar,
	}
	m.recalculateLayout()
	return m
}

func (m SearchModel) Init() tea.Cmd {
	return tea.Batch(
		m.searchBar.input.Focus(),
		m.statusBar.spinner.Tick,
	)
}

// SearchBarIsFocused reports whether the search text input has focus.
func (m SearchModel) SearchBarIsFocused() bool {
	return m.focus == focusSearch && m.searchBar.IsFocused()
}

// TabTitle returns a concise title for this search tab.
func (m SearchModel) TabTitle() string {
	q := strings.TrimSpace(m.activeQuery)
	if q == "" {
		q = strings.TrimSpace(m.searchBar.Query())
	}
	if q == "" {
		return "Search"
	}
	if len(q) > 22 {
		return q[:20] + "…"
	}
	return q
}

// IsSearching reports whether a search job is currently running.
func (m SearchModel) IsSearching() bool {
	return m.isSearching
}

// Cancel terminates any running search job in this tab.
func (m *SearchModel) Cancel() {
	if m.isSearching && m.currentSID != "" && m.client != nil {
		sid := m.currentSID
		m.isSearching = false
		m.currentSID = ""
		go func(cli *splunk.Client, s string) {
			_ = cli.CancelJob(context.Background(), s)
		}(m.client, sid)
	}
}

// BlurSearch unfocuses the search input (moving focus to the table) if currently focused.
func (m *SearchModel) BlurSearch() {
	if m.focus == focusSearch {
		m.setFocus(focusTable)
	}
}

// FocusSearch focuses the SPL search query input.
func (m *SearchModel) FocusSearch() {
	m.setFocus(focusSearch)
}

func (m SearchModel) Update(msg tea.Msg) (SearchModel, tea.Cmd) {
	// If detail modal is open, delegate
	if m.detail != nil {
		var cmd tea.Cmd
		var closed bool
		*m.detail, cmd, closed = m.detail.Update(msg)
		if closed {
			m.detail = nil
		}
		return m, cmd
	}

	// If config explorer modal is open, delegate (except WindowSizeMsg which resizes both)
	if m.configModal != nil {
		if _, isResize := msg.(tea.WindowSizeMsg); !isResize {
			var cmd tea.Cmd
			var closed bool
			*m.configModal, cmd, closed = m.configModal.Update(msg)
			if closed {
				m.configModal = nil
			}
			return m, cmd
		}
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.recalculateLayout()
		if m.configModal != nil {
			m.configModal.SetDimensions(msg.Width, msg.Height)
		}
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.statusBar.spinner, cmd = m.statusBar.spinner.Update(msg)
		return m, cmd

	case OpenDetailMsg:
		detail := NewDetail(m.theme, msg.Event, msg.Row)
		detail.width = m.width
		detail.height = m.height
		m.detail = &detail
		return m, nil

	case IncludeFieldMsg:
		newQ := spl.IncludeValue(m.searchBar.Query(), msg.Field, msg.Value)
		m.searchBar.SetQuery(newQ)
		return m.RunSearch(newQ)

	case ExcludeFieldMsg:
		newQ := spl.ExcludeValue(m.searchBar.Query(), msg.Field, msg.Value)
		m.searchBar.SetQuery(newQ)
		return m.RunSearch(newQ)

	case ToggleColumnMsg:
		cols := m.fieldsSidebar.SelectedColumns()
		m.table.SetEvents(m.activeQuery, m.table.events, m.totalEvents, m.page, cols)
		return m, nil

	case NextPageMsg:
		hasMore := (m.totalEvents > int64(m.page*m.pageSize)) ||
			(len(m.table.events) >= m.pageSize) ||
			(m.table.isTransforming && len(m.table.results) >= m.pageSize)
		if hasMore {
			m.page++
			return m, m.fetchPage()
		}
		return m, nil

	case PrevPageMsg:
		if m.page > 1 {
			m.page--
			return m, m.fetchPage()
		}
		return m, nil

	case jobCreatedMsg:
		if !m.isSearching {
			// User cancelled before job creation returned
			sid := msg.sid
			go func() {
				_ = m.client.CancelJob(context.Background(), sid)
			}()
			return m, nil
		}
		m.currentSID = msg.sid
		m.activeQuery = msg.query
		m.chartPane.SetQuery(msg.query)
		m.timelineBar.SetSpan(spl.ExtractTimechartSpan(msg.query))
		m.page = 1
		return m, m.pollStatus(msg.sid)

	case jobCreateErrorMsg:
		m.isSearching = false
		m.currentSID = ""
		errMsg := msg.err.Error()
		m.statusBar.SetState(false, nil, m.page, m.pageSize, 0, errMsg)
		m.table.SetError(errMsg)
		m.chartPane.SetError(errMsg)
		return m, nil

	case pollTickMsg:
		if m.isSearching && msg.sid == m.currentSID {
			return m, m.pollStatus(msg.sid)
		}
		return m, nil

	case jobPollResultMsg:
		if !m.isSearching || msg.sid != m.currentSID {
			return m, nil // Discard stale or cancelled job responses
		}

		if msg.err != nil {
			m.isSearching = false
			m.statusBar.SetState(false, nil, m.page, m.pageSize, m.totalEvents, msg.err.Error())
			m.table.SetError(msg.err.Error())
			m.chartPane.SetError(msg.err.Error())
			return m, nil
		}

		if msg.status != nil {
			if errMsg := msg.status.ErrorMessage(); errMsg != "" || msg.status.IsFailed {
				m.isSearching = false
				if errMsg == "" {
					errMsg = "Search job failed"
				}
				m.statusBar.SetState(false, msg.status, m.page, m.pageSize, 0, errMsg)
				m.table.SetError(errMsg)
				m.chartPane.SetError(errMsg)
				return m, nil
			}
		}

		count := msg.status.EventCount
		if msg.status.ResultCount > count {
			count = msg.status.ResultCount
		}
		m.statusBar.SetState(!msg.status.IsDone, msg.status, m.page, m.pageSize, count, "")
		m.totalEvents = count

		// Update chart and timeline
		chartType := spl.DetectChartType(m.activeQuery)
		m.chartPane.SetQuery(m.activeQuery)
		m.chartPane.SetData(chartType, msg.timeline, msg.results)
		m.timelineBar.SetSpan(m.chartPane.ActiveSpan())
		m.timelineBar.SetBuckets(msg.timeline)

		// Update fields and table
		if msg.summary != nil {
			m.fieldsSidebar.SetSummary(msg.summary)
		}

		cols := m.fieldsSidebar.SelectedColumns()
		hasRows := false
		if msg.results != nil && (len(msg.results.Results) > 0 || isTransformingJob(m.activeQuery, msg.status)) {
			if len(msg.results.Results) > 0 {
				hasRows = true
			}
			m.table.SetResults(m.activeQuery, msg.results, count, m.page)
		} else if msg.events != nil {
			if len(msg.events.Results) > 0 {
				hasRows = true
			}
			m.table.SetEvents(m.activeQuery, msg.events.Results, count, m.page, cols)
		}

		if m.page > 1 && !hasRows && msg.status != nil && msg.status.IsDone {
			m.page--
			m.statusBar.SetState(false, msg.status, m.page, m.pageSize, count, "No more results (end of data)")
		}
		m.recalculateLayout()

		if !msg.status.IsDone && !msg.status.IsFailed {
			// Continue polling every 500ms
			return m, tea.Tick(500*time.Millisecond, func(time.Time) tea.Msg {
				return pollTickMsg{sid: msg.sid}
			})
		}

		m.isSearching = false
		return m, nil

	case tea.KeyMsg:
		// Global pagination shortcuts
		if !m.searchBar.HasDropdown() {
			if isNextPageKey(msg) {
				hasMore := (m.totalEvents > int64(m.page*m.pageSize)) ||
					(len(m.table.events) >= m.pageSize) ||
					(m.table.isTransforming && len(m.table.results) >= m.pageSize)
				if hasMore {
					m.page++
					return m, m.fetchPage()
				}
				return m, nil
			}
			if isPrevPageKey(msg) {
				if m.page > 1 {
					m.page--
					return m, m.fetchPage()
				}
				return m, nil
			}
		}

		// Global search shortcuts
		switch msg.String() {
		case "n", "N":
			if m.focus != focusSearch && !m.searchBar.HasDropdown() {
				hasMore := (m.totalEvents > int64(m.page*m.pageSize)) ||
					(len(m.table.events) >= m.pageSize) ||
					(m.table.isTransforming && len(m.table.results) >= m.pageSize)
				if hasMore {
					m.page++
					return m, m.fetchPage()
				}
				return m, nil
			}

		case "p", "P":
			if m.focus != focusSearch && !m.searchBar.HasDropdown() {
				if m.page > 1 {
					m.page--
					return m, m.fetchPage()
				}
				return m, nil
			}

		case "ctrl+f", "/":
			if m.focus != focusSearch {
				m.setFocus(focusSearch)
				return m, nil
			}

		case "ctrl+k", "c":
			if (msg.String() == "ctrl+k" || (msg.String() == "c" && m.focus != focusSearch)) && m.configModal == nil {
				modal := NewConfigExplorer(m.theme, m.client)
				modal.SetDimensions(m.width, m.height)
				m.configModal = &modal
				return m, modal.Init()
			}

		case "ctrl+g":
			m.chartPane.ToggleVisible()
			m.recalculateLayout()
			return m, nil

		case "v", "V":
			if m.focus != focusSearch && !m.searchBar.HasDropdown() && m.chartPane.IsVisible() {
				m.chartPane.CycleChartType()
				return m, nil
			}

		case "h", "H":
			if m.focus != focusSearch && !m.searchBar.HasDropdown() && m.chartPane.IsVisible() {
				m.chartPane.CycleHeight()
				m.recalculateLayout()
				return m, nil
			}

		case "ctrl+h":
			if !m.searchBar.HasDropdown() && m.chartPane.IsVisible() {
				m.chartPane.CycleHeight()
				m.recalculateLayout()
				return m, nil
			}

		case "<", ",":
			if m.focus != focusSearch && !m.searchBar.HasDropdown() {
				m.timelineBar.ScrubLeft()
				return m, nil
			}

		case ">", ".":
			if m.focus != focusSearch && !m.searchBar.HasDropdown() {
				m.timelineBar.ScrubRight()
				return m, nil
			}

		case "z", "Z":
			if m.focus != focusSearch && !m.searchBar.HasDropdown() && m.timelineBar.IsScrubbing() {
				earliest, latest, _, ok := m.timelineBar.GetScrubTimeBounds()
				if ok {
					m.timelineBar.ClearScrub()
					m.searchBar.SetTimeRange(earliest, latest)
					return m.RunSearch(m.searchBar.Query())
				}
			}

		case "ctrl+x", "esc":
			if msg.String() == "esc" && m.searchBar.HasDropdown() {
				break
			}
			if m.timelineBar.IsScrubbing() {
				m.timelineBar.ClearScrub()
				return m, nil
			}
			if m.isSearching {
				sid := m.currentSID
				m.isSearching = false
				m.statusBar.SetState(false, nil, m.page, m.pageSize, m.totalEvents, "Search stopped (esc)")
				if sid != "" {
					return m, func() tea.Msg {
						_ = m.client.CancelJob(context.Background(), sid)
						return nil
					}
				}
				return m, nil
			}
			if msg.String() == "esc" && m.focus == focusSearch {
				m.setFocus(focusTable)
				return m, nil
			}

		case "tab":
			if m.focus == focusSearch {
				if m.searchBar.HasDropdown() {
					break
				}
				if m.searchBar.FocusSection() == FocusToTime {
					m.setFocus(focusFields)
					return m, nil
				}
				break
			} else {
				next := (m.focus + 1) % 3
				if next == focusSearch {
					next = focusFields
				}
				m.setFocus(next)
				return m, nil
			}

		case "shift+tab":
			if m.focus == focusSearch {
				if m.searchBar.HasDropdown() {
					break
				}
				if m.searchBar.FocusSection() == FocusInput {
					m.setFocus(focusTable)
					return m, nil
				}
				break
			} else {
				prev := (m.focus - 1 + 3) % 3
				if prev == focusSearch {
					prev = focusTable
				}
				m.setFocus(prev)
				return m, nil
			}
		}
	}

	// Route input to focused sub-component
	var cmd tea.Cmd
	switch m.focus {
	case focusSearch:
		var submitted bool
		m.searchBar, cmd, submitted = m.searchBar.Update(msg)
		if submitted {
			return m.RunSearch(m.searchBar.Query())
		}
		return m, cmd

	case focusFields:
		m.fieldsSidebar, cmd = m.fieldsSidebar.Update(msg)
		return m, cmd

	case focusTable:
		m.table, cmd = m.table.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m *SearchModel) setFocus(p focusPanel) {
	m.focus = p
	m.searchBar.Blur()
	m.fieldsSidebar.Blur()
	m.table.Blur()

	switch p {
	case focusSearch:
		m.searchBar.Focus()
	case focusFields:
		m.fieldsSidebar.Focus()
	case focusTable:
		m.table.Focus()
	}
}

// RunSearch starts a new Splunk search job for the given SPL query.
func (m SearchModel) RunSearch(query string) (SearchModel, tea.Cmd) {
	if query == "" {
		return m, nil
	}

	// Cancel existing job if running
	if m.currentSID != "" {
		oldSID := m.currentSID
		go func() {
			_ = m.client.CancelJob(context.Background(), oldSID)
		}()
	}

	m.isSearching = true
	m.currentSID = ""
	m.table.ClearError()
	m.chartPane.ClearError()
	m.statusBar.SetState(true, nil, 1, m.pageSize, 0, "")
	m.setFocus(focusTable) // Move focus to results view

	// Update chart pane for the new search type and recalculate layout
	chartType := spl.DetectChartType(query)
	m.chartPane.SetQuery(query)
	m.chartPane.SetData(chartType, nil, nil)
	m.timelineBar.SetSpan(spl.ExtractTimechartSpan(query))
	m.timelineBar.SetBuckets(nil)
	m.recalculateLayout()

	client := m.client
	timeRange := m.searchBar.TimeRange()
	earliest := timeRange.Earliest
	latest := timeRange.Latest
	if qE, qL := spl.ExtractTimeBounds(query); qE != "" || qL != "" {
		if qE != "" {
			earliest = qE
		}
		if qL != "" {
			latest = qL
		}
	}

	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		sid, err := client.CreateJob(ctx, splunk.CreateJobOptions{
			Search:        query,
			EarliestTime:  earliest,
			LatestTime:    latest,
			StatusBuckets: 300,
			Verbose:       true,
		})
		if err != nil {
			return jobCreateErrorMsg{err: err}
		}

		return jobCreatedMsg{sid: sid, query: query}
	}
}

func isTransformingJob(query string, status *splunk.JobStatus) bool {
	trimmed := strings.TrimSpace(query)
	if strings.HasPrefix(trimmed, "|") {
		return true
	}
	if spl.HasTableCommand(query) {
		return true
	}
	if spl.DetectChartType(query) != spl.ChartEvents {
		return true
	}
	if status != nil {
		if status.ReportSearch != "" {
			return true
		}
		if status.EventCount == 0 && status.ResultCount > 0 {
			return true
		}
	}
	return false
}

func (m SearchModel) isTransforming() bool {
	if m.table.IsTransforming() {
		return true
	}
	trimmed := strings.TrimSpace(m.activeQuery)
	if strings.HasPrefix(trimmed, "|") {
		return true
	}
	if spl.HasTableCommand(m.activeQuery) {
		return true
	}
	if spl.DetectChartType(m.activeQuery) != spl.ChartEvents {
		return true
	}
	return false
}

func (m SearchModel) pollStatus(sid string) tea.Cmd {
	client := m.client
	query := m.activeQuery
	page := m.page
	pageSize := m.pageSize

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		status, err := client.GetJobStatus(ctx, sid)
		if err != nil {
			return jobPollResultMsg{sid: sid, err: err}
		}

		res := jobPollResultMsg{
			sid:    sid,
			status: status,
		}

		// Fetch timeline
		if timeline, err := client.GetTimeline(ctx, sid); err == nil {
			res.timeline = timeline
		}

		offset := int64((page - 1) * pageSize)
		isTrans := isTransformingJob(query, status)

		var fetchErr error
		if isTrans {
			var results *splunk.ResultsResponse
			var err error
			if status.IsDone {
				results, err = client.GetResults(ctx, sid, offset, pageSize)
			} else {
				results, err = client.GetResultsPreview(ctx, sid, offset, pageSize)
				if err != nil || (results != nil && len(results.Results) == 0) {
					if r, rErr := client.GetResults(ctx, sid, offset, pageSize); rErr == nil && len(r.Results) > 0 {
						results = r
						err = nil
					}
				}
			}
			if err != nil {
				fetchErr = fmt.Errorf("fetching results: %w", err)
			} else {
				res.results = results
			}
		} else {
			events, err := client.GetEvents(ctx, sid, offset, pageSize)
			if err != nil {
				fetchErr = err
			} else {
				res.events = events
			}

			// Fallback: If GetEvents returned 0 rows (or errored) and Splunk has results in ResultCount,
			// automatically fetch from /results (or results_preview)!
			if (res.events == nil || len(res.events.Results) == 0) && status.ResultCount > 0 {
				var results *splunk.ResultsResponse
				var rErr error
				if status.IsDone {
					results, rErr = client.GetResults(ctx, sid, offset, pageSize)
				} else {
					results, rErr = client.GetResultsPreview(ctx, sid, offset, pageSize)
					if rErr != nil || (results != nil && len(results.Results) == 0) {
						results, rErr = client.GetResults(ctx, sid, offset, pageSize)
					}
				}
				if rErr == nil && results != nil && len(results.Results) > 0 {
					res.results = results
					fetchErr = nil
				}
			}

			if summary, err := client.GetFieldSummary(ctx, sid, 10); err == nil {
				res.summary = summary
			}
		}

		// Surface fetch error if neither events nor results were retrieved and fetch failed
		if res.events == nil && res.results == nil && fetchErr != nil {
			res.err = fetchErr
		}

		return res
	}
}

func (m SearchModel) fetchPage() tea.Cmd {
	return m.pollStatus(m.currentSID)
}

func (m *SearchModel) recalculateLayout() {
	w := m.width
	if w <= 0 {
		w = 80
	}
	h := m.height
	if h <= 0 {
		h = 24
	}

	m.searchBar.SetWidth(w)
	m.timelineBar.SetWidth(w)
	m.statusBar.SetWidth(w)

	chartHeight := 0
	if m.chartPane.IsVisible() {
		if m.chartPane.HeightOption() == ChartHeightFull {
			// Full mode: chart takes all remaining screen height below timeline (timeline and spl input still there)
			// Header (1) + SearchBar (3) + Timeline (4) + StatusBar (1) = 9
			chartHeight = h - 9
			if chartHeight < 8 {
				chartHeight = 8
			}
		} else {
			chartHeight = m.chartPane.HeightOption().TotalRows()
			maxChartH := h - 13
			if maxChartH < 6 {
				maxChartH = 6
			}
			if chartHeight > maxChartH {
				chartHeight = maxChartH
			}
		}
	}
	m.chartPane.SetDimensions(w, chartHeight)

	// Fixed heights: Header (1) + SearchBar (3) + Timeline (4) + StatusBar (1) = 9
	// Plus Chart if visible (chartHeight)
	fixedHeight := 9 + chartHeight
	middleHeight := h - fixedHeight
	if middleHeight < 4 {
		middleHeight = 4
	}

	if m.isTransforming() {
		m.table.SetDimensions(w, middleHeight)
	} else {
		sidebarWidth := 26
		if w < 90 {
			sidebarWidth = 20
		}
		tableWidth := w - sidebarWidth - 1
		if tableWidth < 40 {
			tableWidth = 40
		}

		m.fieldsSidebar.SetDimensions(sidebarWidth, middleHeight)
		m.table.SetDimensions(tableWidth, middleHeight)
	}
}

func (m SearchModel) View() string {
	if m.configModal != nil {
		return m.configModal.View()
	}
	if m.detail != nil {
		return m.detail.View()
	}

	w := m.width
	if w <= 0 {
		w = 80
	}
	h := m.height
	if h <= 0 {
		h = 24
	}

	// 1. Header line (strictly 1 line)
	serverStr := "Splunk"
	if m.serverInfo != nil {
		serverStr = fmt.Sprintf("%s (%s)", m.serverInfo.ServerName, m.serverInfo.Version)
	}
	instanceStr := fmt.Sprintf("⚡ %s [%s]", m.bookmark.Name, m.bookmark.URL)
	headerText := lipgloss.JoinHorizontal(lipgloss.Center, instanceStr, "  •  ", serverStr)
	innerW := w - 2
	if innerW < 20 {
		innerW = 20
	}
	header := m.theme.Header.Width(innerW).MaxHeight(1).Render(headerText)

	// 2. Search Bar
	searchView := m.searchBar.View()

	// If timepicker dropdown is open, layer it
	if m.searchBar.HasDropdown() {
		dropdown := m.searchBar.ViewDropdown()
		searchView = lipgloss.JoinVertical(lipgloss.Left, searchView, dropdown)
	}

	// 3. Timeline Bar
	timelineView := m.timelineBar.View()

	// 4. Middle area: Fields Sidebar + Events Table
	var middleView string
	if m.isTransforming() {
		// In transforming search, give entire width to results table
		m.table.SetDimensions(m.width, m.table.height)
		middleView = m.table.View()
	} else {
		middleView = lipgloss.JoinHorizontal(lipgloss.Top, m.fieldsSidebar.View(), " ", m.table.View())
	}

	// 5. Status bar
	statusView := m.statusBar.View()

	var views []string
	views = append(views, header, searchView, timelineView)

	if m.chartPane.IsVisible() {
		if cv := m.chartPane.View(); cv != "" {
			views = append(views, cv)
		}
	}

	if !m.chartPane.IsVisible() || m.chartPane.HeightOption() != ChartHeightFull {
		views = append(views, middleView)
	}

	views = append(views, statusView)
	fullView := lipgloss.JoinVertical(lipgloss.Left, views...)
	return lipgloss.NewStyle().MaxHeight(h).Render(fullView)
}
