package search

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"splunk-tui/internal/config"
	"splunk-tui/internal/spl"
	"splunk-tui/internal/splunk"
	"splunk-tui/internal/ui/theme"
)

func TestChartPaneSpanSupport(t *testing.T) {
	th := theme.DefaultTheme()
	pane := NewChartPane(th)
	pane.SetDimensions(80, 10)

	// SPL with explicit span argument
	query := "index=_internal | timechart span=1h count"
	pane.SetQuery(query)
	if pane.ActiveSpan() != "1h" {
		t.Fatalf("expected ActiveSpan '1h', got %q", pane.ActiveSpan())
	}

	res := &splunk.ResultsResponse{
		Fields: []string{"_time", "_span", "count"},
		Results: []map[string]string{
			{"_time": "2026-10-13T14:00:00.000+00:00", "_span": "3600", "count": "100"},
			{"_time": "2026-10-13T15:00:00.000+00:00", "_span": "3600", "count": "250"},
			{"_time": "2026-10-13T16:00:00.000+00:00", "_span": "3600", "count": "50"},
		},
	}

	chartType := spl.DetectChartType(query)
	pane.SetData(chartType, nil, res)

	// Header should contain the span argument
	viewOutput := pane.View()
	if !strings.Contains(viewOutput, "(span: 1h)") {
		t.Errorf("expected view to contain '(span: 1h)', got:\n%s", viewOutput)
	}

	// Time boundaries should format correctly
	t1, t2 := pane.getTimeBounds()
	if t1 != "10/13 14:00" || t2 != "10/13 16:00" {
		t.Errorf("expected time bounds '10/13 14:00' and '10/13 16:00', got %q and %q", t1, t2)
	}

	// Cycle to Bar and Sparklines
	pane.CycleChartType()
	if pane.ActiveType() != spl.ChartColumnTime {
		t.Fatalf("expected active type ChartColumnTime (Bar), got %v", pane.ActiveType())
	}
	barView := pane.View()
	if !strings.Contains(barView, "(span: 1h)") || !strings.Contains(barView, "Bar") {
		t.Errorf("expected bar view to contain '(span: 1h)' and 'Bar', got:\n%s", barView)
	}

	pane.CycleChartType()
	if pane.ActiveType() != spl.ChartSparklines {
		t.Fatalf("expected active type Sparklines, got %v", pane.ActiveType())
	}
	sparkView := pane.View()
	if !strings.Contains(sparkView, "(span: 1h)") {
		t.Errorf("expected sparklines view to contain '(span: 1h)', got:\n%s", sparkView)
	}
}

func TestChartPaneSpanDerivedFromResults(t *testing.T) {
	th := theme.DefaultTheme()
	pane := NewChartPane(th)
	pane.SetDimensions(80, 10)

	// SPL without explicit span
	query := "index=_internal | timechart count"
	pane.SetQuery(query)
	if pane.ActiveSpan() != "" {
		t.Fatalf("expected empty ActiveSpan, got %q", pane.ActiveSpan())
	}

	// Splunk returns _span: "300" (5m)
	res := &splunk.ResultsResponse{
		Fields: []string{"_time", "_span", "count"},
		Results: []map[string]string{
			{"_time": "2026-10-13T14:00:00.000+00:00", "_span": "300", "count": "42"},
			{"_time": "2026-10-13T14:05:00.000+00:00", "_span": "300", "count": "88"},
		},
	}

	pane.SetData(spl.ChartLine, nil, res)
	if pane.ActiveSpan() != "5m" {
		t.Errorf("expected ActiveSpan to be derived as '5m', got %q", pane.ActiveSpan())
	}

	viewOutput := pane.View()
	if !strings.Contains(viewOutput, "(span: 5m)") {
		t.Errorf("expected view to contain '(span: 5m)', got:\n%s", viewOutput)
	}
}

func TestTimelineBarSpan(t *testing.T) {
	th := theme.DefaultTheme()
	tl := NewTimelineBar(th)
	tl.SetWidth(80)
	tl.SetSpan("15m")

	// Empty state
	viewEmpty := tl.View()
	if !strings.Contains(viewEmpty, "Timeline (span: 15m)") {
		t.Errorf("expected empty timeline to show span, got:\n%s", viewEmpty)
	}

	// Populated state
	buckets := []splunk.TimelineBucket{
		{Time: time.Now(), Count: 10},
		{Time: time.Now().Add(15 * time.Minute), Count: 25},
	}
	tl.SetBuckets(buckets)
	viewPopulated := tl.View()
	if !strings.Contains(viewPopulated, "Timeline (span: 15m)") {
		t.Errorf("expected populated timeline to show span, got:\n%s", viewPopulated)
	}
}

func TestChartPaneHeightOptions(t *testing.T) {
	th := theme.DefaultTheme()
	pane := NewChartPane(th)
	pane.SetDimensions(80, 8)

	// SPL transforming query to populate chart
	query := "index=_internal | timechart count"
	pane.SetQuery(query)

	res := &splunk.ResultsResponse{
		Fields: []string{"_time", "count"},
		Results: []map[string]string{
			{"_time": "2026-10-13T10:00:00Z", "count": "10"},
			{"_time": "2026-10-13T11:00:00Z", "count": "45"},
			{"_time": "2026-10-13T12:00:00Z", "count": "20"},
		},
	}
	pane.SetData(spl.ChartLine, nil, res)

	// 1. Default should be Small (8 rows)
	if pane.HeightOption() != ChartHeightSmall {
		t.Fatalf("expected initial HeightOption ChartHeightSmall, got %v", pane.HeightOption())
	}
	if pane.HeightOption().TotalRows() != 8 {
		t.Fatalf("expected Small rows = 8, got %d", pane.HeightOption().TotalRows())
	}
	viewSmall := pane.View()
	if !strings.Contains(viewSmall, "Small") || !strings.Contains(viewSmall, "h: height") {
		t.Errorf("expected Small view to contain height pill and hint, got:\n%s", viewSmall)
	}

	// 2. Cycle to Medium (14 rows)
	pane.CycleHeight()
	if pane.HeightOption() != ChartHeightMedium {
		t.Fatalf("expected HeightOption ChartHeightMedium after 1st cycle, got %v", pane.HeightOption())
	}
	if pane.HeightOption().TotalRows() != 14 {
		t.Fatalf("expected Medium rows = 14, got %d", pane.HeightOption().TotalRows())
	}
	viewMedium := pane.View()
	if !strings.Contains(viewMedium, "Medium") {
		t.Errorf("expected Medium view to contain Medium pill, got:\n%s", viewMedium)
	}

	// 3. Cycle to Large (20 rows)
	pane.CycleHeight()
	if pane.HeightOption() != ChartHeightLarge {
		t.Fatalf("expected HeightOption ChartHeightLarge after 2nd cycle, got %v", pane.HeightOption())
	}
	if pane.HeightOption().TotalRows() != 20 {
		t.Fatalf("expected Large rows = 20, got %d", pane.HeightOption().TotalRows())
	}
	viewLarge := pane.View()
	if !strings.Contains(viewLarge, "Large") {
		t.Errorf("expected Large view to contain Large pill, got:\n%s", viewLarge)
	}

	// 4. Cycle to X-Large (28 rows)
	pane.CycleHeight()
	if pane.HeightOption() != ChartHeightXLarge {
		t.Fatalf("expected HeightOption ChartHeightXLarge after 3rd cycle, got %v", pane.HeightOption())
	}
	if pane.HeightOption().TotalRows() != 28 {
		t.Fatalf("expected X-Large rows = 28, got %d", pane.HeightOption().TotalRows())
	}
	viewXLarge := pane.View()
	if !strings.Contains(viewXLarge, "X-Large") {
		t.Errorf("expected X-Large view to contain X-Large pill, got:\n%s", viewXLarge)
	}

	// 5. Cycle to Full
	pane.CycleHeight()
	if pane.HeightOption() != ChartHeightFull {
		t.Fatalf("expected HeightOption ChartHeightFull after 4th cycle, got %v", pane.HeightOption())
	}
	viewFull := pane.View()
	if !strings.Contains(viewFull, "Full") {
		t.Errorf("expected Full view to contain Full pill, got:\n%s", viewFull)
	}

	// 6. Cycle wraps back to Small (8 rows)
	pane.CycleHeight()
	if pane.HeightOption() != ChartHeightSmall {
		t.Fatalf("expected HeightOption ChartHeightSmall after 5th cycle, got %v", pane.HeightOption())
	}
}

func TestTimechartByHostMultiLineAndStackedBar(t *testing.T) {
	th := theme.DefaultTheme()
	pane := NewChartPane(th)
	pane.SetDimensions(80, 14)

	query := "index=_internal | timechart count BY host"
	pane.SetQuery(query)

	res := &splunk.ResultsResponse{
		Fields: []string{"_time", "server-alpha", "server-beta", "server-gamma"},
		Results: []map[string]string{
			{"_time": "2026-10-13T10:00:00Z", "server-alpha": "10", "server-beta": "30", "server-gamma": "5"},
			{"_time": "2026-10-13T11:00:00Z", "server-alpha": "45", "server-beta": "20", "server-gamma": "15"},
			{"_time": "2026-10-13T12:00:00Z", "server-alpha": "20", "server-beta": "50", "server-gamma": "40"},
		},
	}
	pane.SetData(spl.ChartLine, nil, res)

	// 1. Line chart should render legend with hosts
	viewLine := pane.View()
	if !strings.Contains(viewLine, "server-alpha") || !strings.Contains(viewLine, "server-beta") || !strings.Contains(viewLine, "server-gamma") {
		t.Errorf("expected multi-line chart to contain host legend items, got:\n%s", viewLine)
	}

	// 2. Switch to Bar chart (stacked)
	pane.CycleChartType()
	if pane.ActiveType() != spl.ChartColumnTime {
		t.Fatalf("expected active type ChartColumnTime (Bar), got %v", pane.ActiveType())
	}
	viewBar := pane.View()
	if !strings.Contains(viewBar, "Bar") {
		t.Errorf("expected header to show 'Bar', got:\n%s", viewBar)
	}
	if !strings.Contains(viewBar, "server-alpha") || !strings.Contains(viewBar, "server-beta") || !strings.Contains(viewBar, "server-gamma") {
		t.Errorf("expected stacked bar chart to contain host legend items, got:\n%s", viewBar)
	}
}

func TestTimechartLimitAndDefaultTop10Other(t *testing.T) {
	th := theme.DefaultTheme()
	pane := NewChartPane(th)
	pane.SetDimensions(120, 14)

	// Create 12 series: host01 (val 120), host02 (val 110), ... host12 (val 10)
	fields := []string{"_time"}
	row := map[string]string{"_time": "2026-10-13T10:00:00Z"}
	for i := 1; i <= 12; i++ {
		h := fmt.Sprintf("host%02d", i)
		fields = append(fields, h)
		val := fmt.Sprintf("%d", (13-i)*10)
		row[h] = val
	}
	res := &splunk.ResultsResponse{
		Fields:  fields,
		Results: []map[string]string{row},
	}

	// 1. Default with 'by host' -> top 10 + "other"
	pane.SetQuery("index=main | timechart count by host")
	pane.SetData(spl.ChartLine, nil, res)
	cols, vals := pane.extractTimeSeries()
	if len(cols) != 11 {
		t.Fatalf("expected 11 columns (10 top + other), got %d: %v", len(cols), cols)
	}
	for i := 0; i < 10; i++ {
		expectedHost := fmt.Sprintf("host%02d", i+1)
		if cols[i] != expectedHost {
			t.Errorf("expected cols[%d] = %q, got %q", i, expectedHost, cols[i])
		}
	}
	if cols[10] != "other" {
		t.Errorf("expected last column to be 'other', got %q", cols[10])
	}
	// Other value should be host11 (20) + host12 (10) = 30
	if vals[10][0] != 30 {
		t.Errorf("expected other value 30, got %f", vals[10][0])
	}

	// 2. Explicit limit=5 -> top 5 + "other"
	pane.SetQuery("index=main | timechart limit=5 count by host")
	colsLim5, valsLim5 := pane.extractTimeSeries()
	if len(colsLim5) != 6 {
		t.Fatalf("expected 6 columns (5 top + other), got %d: %v", len(colsLim5), colsLim5)
	}
	if colsLim5[5] != "other" {
		t.Errorf("expected last column to be 'other', got %q", colsLim5[5])
	}
	// Other should be sum of host06..host12 = 70+60+50+40+30+20+10 = 280
	if valsLim5[5][0] != 280 {
		t.Errorf("expected other value 280, got %f", valsLim5[5][0])
	}

	// 3. limit=0 -> ALL 12, no "other"
	pane.SetQuery("index=main | timechart limit=0 count by host")
	colsLim0, _ := pane.extractTimeSeries()
	if len(colsLim0) != 12 {
		t.Fatalf("expected all 12 columns for limit=0, got %d: %v", len(colsLim0), colsLim0)
	}
	for _, c := range colsLim0 {
		if c == "other" {
			t.Errorf("unexpected 'other' column when limit=0")
		}
	}
}

func TestStatsLimitAndDefaultTop10Other(t *testing.T) {
	th := theme.DefaultTheme()
	pane := NewChartPane(th)
	pane.SetDimensions(120, 14)

	// Create 15 categories: status01 (150), status02 (140), ... status15 (10)
	var results []map[string]string
	for i := 1; i <= 15; i++ {
		results = append(results, map[string]string{
			"status": fmt.Sprintf("status%02d", i),
			"count":  fmt.Sprintf("%d", (16-i)*10),
		})
	}
	res := &splunk.ResultsResponse{
		Fields:  []string{"status", "count"},
		Results: results,
	}

	// 1. Default with 'by status' -> top 10 + "other"
	pane.SetQuery("index=main | stats count by status")
	pane.SetData(spl.ChartBar, nil, res)
	items := pane.extractCategoricalData()
	if len(items) != 11 {
		t.Fatalf("expected 11 items (10 top + other), got %d: %v", len(items), items)
	}
	for i := 0; i < 10; i++ {
		expectedStatus := fmt.Sprintf("status%02d", i+1)
		if items[i].name != expectedStatus {
			t.Errorf("expected items[%d].name = %q, got %q", i, expectedStatus, items[i].name)
		}
	}
	if items[10].name != "other" {
		t.Errorf("expected last item to be 'other', got %q", items[10].name)
	}
	// status11..status15 = 50+40+30+20+10 = 150
	if items[10].val != 150 {
		t.Errorf("expected other value 150, got %f", items[10].val)
	}

	// 2. Explicit limit=3 -> top 3 + "other"
	pane.SetQuery("index=main | stats limit=3 count by status")
	itemsLim3 := pane.extractCategoricalData()
	if len(itemsLim3) != 4 {
		t.Fatalf("expected 4 items (3 top + other), got %d: %v", len(itemsLim3), itemsLim3)
	}
	if itemsLim3[3].name != "other" {
		t.Errorf("expected last item to be 'other', got %q", itemsLim3[3].name)
	}

	// 3. limit=0 -> ALL 15, no "other"
	pane.SetQuery("index=main | stats limit=0 count by status")
	itemsLim0 := pane.extractCategoricalData()
	if len(itemsLim0) != 15 {
		t.Fatalf("expected all 15 items for limit=0, got %d: %v", len(itemsLim0), itemsLim0)
	}
	for _, it := range itemsLim0 {
		if it.name == "other" {
			t.Errorf("unexpected 'other' category when limit=0")
		}
	}
}

func TestSearchChartHeightFullLayout(t *testing.T) {
	th := theme.DefaultTheme()
	cfg := &config.Config{PageSize: 50}
	bm := config.Bookmark{Name: "test"}
	m := New(th, cfg, bm, nil, nil)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	// Make chart pane visible with transforming query
	m.chartPane.SetData(spl.ChartLine, nil, &splunk.ResultsResponse{
		Fields: []string{"_time", "count"},
		Results: []map[string]string{
			{"_time": "2026-10-13T10:00:00Z", "count": "100"},
		},
	})
	m.chartPane.SetHeightOption(ChartHeightFull)
	m.recalculateLayout()

	expectedChartH := 30 - 9 // 21 rows
	if m.chartPane.height != expectedChartH {
		t.Errorf("expected chartPane height %d, got %d", expectedChartH, m.chartPane.height)
	}

	view := m.View()
	// Should contain timeline
	if !strings.Contains(view, "Timeline") {
		t.Errorf("expected view to contain Timeline in Full mode")
	}
	// Should contain SPL input
	if !strings.Contains(view, "From:") {
		t.Errorf("expected view to contain From: in Full mode")
	}
	// Should contain chart
	if !strings.Contains(view, "Line") || !strings.Contains(view, "Full") {
		t.Errorf("expected view to contain Line and Full in Full mode")
	}
}

func TestChartPaneErrorMessage(t *testing.T) {
	th := theme.DefaultTheme()
	pane := NewChartPane(th)
	pane.SetDimensions(80, 10)
	pane.SetData(spl.ChartLine, nil, &splunk.ResultsResponse{
		Fields: []string{"_time", "count"},
		Results: []map[string]string{{"_time": "2026-10-13T10:00:00Z", "count": "100"}},
	})

	pane.SetError("Error in 'stats' command: The argument 'coun' is invalid.")
	view := pane.View()

	if !strings.Contains(view, "Search Error") {
		t.Errorf("expected chart view to contain 'Search Error', got:\n%s", view)
	}
	if !strings.Contains(view, "The argument 'coun' is invalid") {
		t.Errorf("expected chart view to contain error message, got:\n%s", view)
	}

	// SetData clears the error
	pane.SetData(spl.ChartLine, nil, &splunk.ResultsResponse{
		Fields: []string{"_time", "count"},
		Results: []map[string]string{{"_time": "2026-10-13T10:00:00Z", "count": "100"}},
	})
	viewAfter := pane.View()
	if strings.Contains(viewAfter, "Search Error") {
		t.Errorf("expected error to be cleared after SetData")
	}
}

func TestChartPaneMultiTickXAxis(t *testing.T) {
	th := theme.DefaultTheme()
	pane := NewChartPane(th)

	// Create 49 results over 4 hours (07:50 to 11:50 with 5m interval) matching the user's search
	baseTime := time.Date(2026, 10, 14, 7, 50, 0, 0, time.UTC)
	var rows []map[string]string
	for i := 0; i <= 48; i++ {
		ts := baseTime.Add(time.Duration(i*5) * time.Minute)
		rows = append(rows, map[string]string{
			"_time": ts.Format(time.RFC3339),
			"count": fmt.Sprintf("%d", 100+(i%10)*20),
		})
	}

	res := &splunk.ResultsResponse{
		Fields:  []string{"_time", "count"},
		Results: rows,
	}
	pane.SetData(spl.ChartLine, nil, res)

	// Test wide grid (140 columns)
	bottomAxis, timeAxis := pane.renderXAxis(140)
	if !strings.Contains(timeAxis, "10/14 07:50") {
		t.Errorf("expected timeAxis to contain earliest '10/14 07:50', got:\n%s", timeAxis)
	}
	if !strings.Contains(timeAxis, "10/14 11:50") {
		t.Errorf("expected timeAxis to contain latest '10/14 11:50', got:\n%s", timeAxis)
	}
	// At 140 columns, intermediate ticks should be present
	tickCount := strings.Count(bottomAxis, "┴")
	if tickCount < 4 {
		t.Errorf("expected at least 4 ticks on bottomAxis for width 140, got %d:\n%s", tickCount, bottomAxis)
	}
	// Verify intermediate timestamps exist (e.g. 08:30, 08:50, 09:10, 09:50, 10:30, etc.)
	foundIntermediate := false
	for _, expected := range []string{"08:30", "08:50", "09:10", "09:50", "10:30", "10:50"} {
		if strings.Contains(timeAxis, expected) {
			foundIntermediate = true
			break
		}
	}
	if !foundIntermediate {
		t.Errorf("expected intermediate timestamps in timeAxis, got:\n%s", timeAxis)
	}

	// Test medium grid (90 columns)
	_, timeAxisMed := pane.renderXAxis(90)
	if !strings.Contains(timeAxisMed, "10/14 07:50") || !strings.Contains(timeAxisMed, "10/14 11:50") {
		t.Errorf("expected timeAxisMed to contain start and end timestamps, got:\n%s", timeAxisMed)
	}
	if !strings.Contains(timeAxisMed, "10/14 09:50") {
		t.Errorf("expected timeAxisMed at 90 cols to contain midpoint '10/14 09:50', got:\n%s", timeAxisMed)
	}

	// Test narrow grid (25 columns) - should not crash or produce overlapping garbage
	_, timeAxisNarrow := pane.renderXAxis(25)
	if len(timeAxisNarrow) == 0 {
		t.Errorf("expected non-empty timeAxis for narrow width")
	}

	// Test timeline bucket fallback
	paneTimeline := NewChartPane(th)
	var buckets []splunk.TimelineBucket
	for i := 0; i <= 24; i++ {
		buckets = append(buckets, splunk.TimelineBucket{
			Time:  baseTime.Add(time.Duration(i*10) * time.Minute),
			Count: int64(10 + i),
		})
	}
	paneTimeline.SetData(spl.ChartEvents, buckets, nil)
	_, tlAxis := paneTimeline.renderXAxis(100)
	if !strings.Contains(tlAxis, "10/14 07:50") {
		t.Errorf("expected timeline X-axis to contain start timestamp, got:\n%s", tlAxis)
	}
}

func TestFormatTimeString(t *testing.T) {
	// Standard ISO / RFC3339
	iso := "2026-10-14T07:50:00.000Z"
	if formatted := formatTimeString(iso); formatted != "10/14 07:50" {
		t.Errorf("expected '10/14 07:50', got %q", formatted)
	}

	// Space separated
	space := "2026-10-14 07:50:00"
	if formatted := formatTimeString(space); formatted != "10/14 07:50" {
		t.Errorf("expected '10/14 07:50', got %q", formatted)
	}

	// Unix epoch float
	epochFloat := "1728892200.000" // 2024-10-14 07:50:00 UTC
	formattedEpoch := formatTimeString(epochFloat)
	if !strings.Contains(formattedEpoch, ":") || len(formattedEpoch) < 8 {
		t.Errorf("expected epoch to format to date/time, got %q", formattedEpoch)
	}

	// Empty string
	if formatted := formatTimeString(""); formatted != "" {
		t.Errorf("expected empty string, got %q", formatted)
	}
}


