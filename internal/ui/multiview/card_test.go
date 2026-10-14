package multiview

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"splunk-tui/internal/config"
	"splunk-tui/internal/spl"
	"splunk-tui/internal/splunk"
	"splunk-tui/internal/ui/browser"
	"splunk-tui/internal/ui/theme"
)

func TestFindMetricFieldIgnoresInternalFields(t *testing.T) {
	// Results with _time, _span, and actual metric count
	res := &splunk.ResultsResponse{
		Fields: []string{"_time", "_span", "count"},
		Results: []map[string]string{
			{"_time": "2026-10-13T14:00:00.000+00:00", "_span": "3600", "count": "150"},
		},
	}

	field := findMetricField(res)
	if field != "count" {
		t.Errorf("expected findMetricField to return 'count', but got %q", field)
	}

	// Results with _subsecond, _time, _span, avg_load
	res2 := &splunk.ResultsResponse{
		Fields: []string{"_time", "_span", "_subsecond", "avg_load"},
		Results: []map[string]string{
			{"_time": "2026-10-13T14:00:00.000+00:00", "_span": "300", "avg_load": "2.4"},
		},
	}

	field2 := findMetricField(res2)
	if field2 != "avg_load" {
		t.Errorf("expected findMetricField to return 'avg_load', but got %q", field2)
	}
}

func TestCardSpanSupport(t *testing.T) {
	th := theme.DefaultTheme()
	conn := browser.InstanceConnection{
		Bookmark: config.Bookmark{Name: "prod-us-east-1", URL: "https://splunk1.example.com:8089"},
	}
	card := NewInstanceCard(0, conn, th)
	card.SetDimensions(80, 12)

	query := "index=_internal | timechart span=1h count"
	_ = card.RunSearch(query, "-24h", "now")

	if card.activeSpan != "1h" {
		t.Fatalf("expected card.activeSpan '1h', got %q", card.activeSpan)
	}

	res := &splunk.ResultsResponse{
		Fields: []string{"_time", "_span", "count"},
		Results: []map[string]string{
			{"_time": "2026-10-13T10:00:00.000+00:00", "_span": "3600", "count": "50"},
			{"_time": "2026-10-13T11:00:00.000+00:00", "_span": "3600", "count": "120"},
			{"_time": "2026-10-13T12:00:00.000+00:00", "_span": "3600", "count": "80"},
		},
	}

	card, _ = card.Update(CardPollResultMsg{
		CardIndex: 0,
		Status: &splunk.JobStatus{
			IsDone:      true,
			ResultCount: 3,
			EventCount:  250,
			RunDuration: 0.45,
		},
		Results: res,
	})

	viewOutput := card.View()
	if !strings.Contains(viewOutput, "span: 1h") {
		t.Errorf("expected card view to display 'span: 1h', got:\n%s", viewOutput)
	}

	// Verify URL information has been removed from view
	if strings.Contains(viewOutput, "https://splunk1.example.com") {
		t.Errorf("expected URL to be removed from card header, but found in:\n%s", viewOutput)
	}

	// Verify first content line inside the card contains instance name, status, and span
	lines := strings.Split(viewOutput, "\n")
	firstContentLine := ""
	if len(lines) > 1 {
		firstContentLine = lines[1]
	}
	if !strings.Contains(firstContentLine, "prod-us-east-1") || !strings.Contains(firstContentLine, "DONE") {
		t.Errorf("expected first content line to contain instance name and status, got:\n%s", firstContentLine)
	}
	if !strings.Contains(firstContentLine, "span: 1h") {
		t.Errorf("expected first content line to contain 'span: 1h', got:\n%s", firstContentLine)
	}

	// Verify time boundaries formatted properly
	t1, t2 := card.getTimeBounds()
	if t1 != "10/13 10:00" || t2 != "10/13 12:00" {
		t.Errorf("expected bounds '10/13 10:00' and '10/13 12:00', got %q and %q", t1, t2)
	}
}

func TestResampleFullWidth(t *testing.T) {
	// 49 points (like the user's search) into 75 target length
	series := make([]float64, 49)
	for i := range series {
		series[i] = float64(i)
	}

	targetLen := 75
	res := resample(series, targetLen)
	if len(res) != targetLen {
		t.Fatalf("expected resampled length %d, got %d", targetLen, len(res))
	}
	if res[0] != 0 || res[targetLen-1] != 48 {
		t.Errorf("expected bounds 0 and 48, got %f and %f", res[0], res[targetLen-1])
	}
}

func TestCardMultiSeriesTimechart(t *testing.T) {
	th := theme.DefaultTheme()
	conn := browser.InstanceConnection{
		Bookmark: config.Bookmark{Name: "prod-us-east-1", URL: "https://splunk1.example.com:8089"},
	}
	card := NewInstanceCard(0, conn, th)
	card.SetDimensions(80, 12)

	query := "index=_internal | timechart count BY host"
	_ = card.RunSearch(query, "-24h", "now")

	res := &splunk.ResultsResponse{
		Fields: []string{"_time", "host1", "host2"},
		Results: []map[string]string{
			{"_time": "2026-10-13T10:00:00.000+00:00", "host1": "50", "host2": "30"},
			{"_time": "2026-10-13T11:00:00.000+00:00", "host1": "120", "host2": "80"},
		},
	}

	card, _ = card.Update(CardPollResultMsg{
		CardIndex: 0,
		Status: &splunk.JobStatus{
			IsDone:      true,
			ResultCount: 2,
			EventCount:  200,
			RunDuration: 0.25,
		},
		Results: res,
	})

	viewOutput := card.View()
	if !strings.Contains(viewOutput, "host1") || !strings.Contains(viewOutput, "host2") {
		t.Errorf("expected multi-series card to contain host legend items, got:\n%s", viewOutput)
	}

	// Switch to Bar
	card.CycleChartType()
	barOutput := card.View()
	if !strings.Contains(barOutput, "Bar") {
		t.Errorf("expected card badge to show Bar, got:\n%s", barOutput)
	}
	if !strings.Contains(barOutput, "host1") || !strings.Contains(barOutput, "host2") {
		t.Errorf("expected stacked bar card to contain host legend items, got:\n%s", barOutput)
	}
}

func TestCardLimitAndDefaultTop10Other(t *testing.T) {
	th := theme.DefaultTheme()
	conn := browser.InstanceConnection{
		Bookmark: config.Bookmark{Name: "prod-us-east-1", URL: "https://splunk1.example.com:8089"},
	}
	card := NewInstanceCard(0, conn, th)
	card.SetDimensions(80, 12)

	fields := []string{"_time"}
	row := map[string]string{"_time": "2026-10-13T10:00:00Z"}
	for i := 1; i <= 12; i++ {
		h := fmt.Sprintf("host%02d", i)
		fields = append(fields, h)
		row[h] = fmt.Sprintf("%d", (13-i)*10)
	}
	res := &splunk.ResultsResponse{
		Fields:  fields,
		Results: []map[string]string{row},
	}

	// 1. timechart count by host -> default 10 + other
	_ = card.RunSearch("index=_internal | timechart count BY host", "-1h", "now")
	card.results = res
	cols, vals := card.extractTimeSeries()
	if len(cols) != 11 {
		t.Fatalf("expected 11 columns, got %d", len(cols))
	}
	if cols[10] != "other" {
		t.Errorf("expected last column to be 'other', got %q", cols[10])
	}
	if vals[10][0] != 30 {
		t.Errorf("expected other value 30, got %f", vals[10][0])
	}

	// 2. limit=3
	_ = card.RunSearch("index=_internal | timechart limit=3 count BY host", "-1h", "now")
	card.results = res
	colsLim3, valsLim3 := card.extractTimeSeries()
	if len(colsLim3) != 4 {
		t.Fatalf("expected 4 columns, got %d", len(colsLim3))
	}
	if colsLim3[3] != "other" {
		t.Errorf("expected last column to be 'other', got %q", colsLim3[3])
	}
	if valsLim3[3][0] != 450 {
		t.Errorf("expected other value 450, got %f", valsLim3[3][0])
	}

	// 3. limit=0
	_ = card.RunSearch("index=_internal | timechart limit=0 count BY host", "-1h", "now")
	card.results = res
	colsLim0, _ := card.extractTimeSeries()
	if len(colsLim0) != 12 {
		t.Fatalf("expected 12 columns for limit=0, got %d", len(colsLim0))
	}
}

func TestCardJobErrorHandling(t *testing.T) {
	th := theme.DefaultTheme()
	conn := browser.InstanceConnection{
		Bookmark: config.Bookmark{Name: "prod-us-east-1", URL: "https://splunk1.example.com:8089"},
	}
	card := NewInstanceCard(0, conn, th)
	card.SetDimensions(80, 12)

	// Simulate failed job update
	failedStatus := &splunk.JobStatus{
		IsDone:   true,
		IsFailed: true,
		Messages: []splunk.Message{
			{Type: "FATAL", Text: "Error in 'stats' command: The argument 'coun' is invalid."},
		},
	}

	card, _ = card.Update(CardPollResultMsg{
		CardIndex: 0,
		Status:    failedStatus,
	})

	if card.err == nil {
		t.Fatalf("expected card.err to be set when job fails")
	}
	if !strings.Contains(card.err.Error(), "The argument 'coun' is invalid") {
		t.Errorf("expected card error to contain failure message, got: %v", card.err)
	}

	view := card.View()
	if !strings.Contains(view, "The argument 'coun' is invalid") {
		t.Errorf("expected card view to display error message, got:\n%s", view)
	}
}

func TestInstanceCardMultiTickXAxis(t *testing.T) {
	th := theme.DefaultTheme()
	conn := browser.InstanceConnection{
		Bookmark: config.Bookmark{Name: "prod", URL: "https://splunk.example.com:8089"},
	}
	card := NewInstanceCard(0, conn, th)

	baseTime := time.Date(2026, 10, 14, 8, 0, 0, 0, time.UTC)
	var rows []map[string]string
	for i := 0; i <= 24; i++ {
		ts := baseTime.Add(time.Duration(i*10) * time.Minute)
		rows = append(rows, map[string]string{
			"_time": ts.Format(time.RFC3339),
			"count": fmt.Sprintf("%d", 50+i*5),
		})
	}

	res := &splunk.ResultsResponse{
		Fields:  []string{"_time", "count"},
		Results: rows,
	}
	card.cmdType = spl.MultiViewTimechart
	card.results = res

	bottomAxis, timeAxis := card.renderXAxis(100)
	if !strings.Contains(timeAxis, "10/14 08:00") {
		t.Errorf("expected start timestamp in card timeAxis, got:\n%s", timeAxis)
	}
	if !strings.Contains(timeAxis, "10/14 12:00") {
		t.Errorf("expected end timestamp in card timeAxis, got:\n%s", timeAxis)
	}
	if strings.Count(bottomAxis, "┴") < 2 {
		t.Errorf("expected intermediate ticks in bottomAxis, got:\n%s", bottomAxis)
	}
}


