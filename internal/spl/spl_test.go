package spl

import (
	"reflect"
	"testing"
)

func TestQuoteValue(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"200", `"200"`},
		{`GET /api/v1`, `"GET /api/v1"`},
		{`error: "failed"`, `"error: \"failed\""`},
		{`path\to\file`, `"path\\to\\file"`},
	}

	for _, tt := range tests {
		got := QuoteValue(tt.in)
		if got != tt.want {
			t.Errorf("QuoteValue(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestFormatCondition(t *testing.T) {
	if got := FormatCondition("status", "200", true); got != `status="200"` {
		t.Errorf("include condition got %q, want %q", got, `status="200"`)
	}
	if got := FormatCondition("status", "500", false); got != `status!="500"` {
		t.Errorf("exclude condition got %q, want %q", got, `status!="500"`)
	}
}

func TestSplitTopLevelPipes(t *testing.T) {
	tests := []struct {
		query string
		want  int
	}{
		{"index=main", 1},
		{"index=main | stats count by host", 2},
		{"index=main | eval x=\"a|b\" | stats count", 3},
		{"index=main [ search index=audit | stats count ] | stats sum(bytes)", 2},
	}

	for _, tt := range tests {
		parts := SplitTopLevelPipes(tt.query)
		if len(parts) != tt.want {
			t.Errorf("SplitTopLevelPipes(%q) gave %d parts, want %d", tt.query, len(parts), tt.want)
		}
	}
}

func TestIncludeAndExcludeValue(t *testing.T) {
	baseQuery := "index=main sourcetype=access"
	inc := IncludeValue(baseQuery, "status", "200")
	if inc != `index=main sourcetype=access status="200"` {
		t.Errorf("IncludeValue failed: got %q", inc)
	}

	exc := ExcludeValue(baseQuery, "status", "404")
	if exc != `index=main sourcetype=access status!="404"` {
		t.Errorf("ExcludeValue failed: got %q", exc)
	}

	pipedQuery := "index=main sourcetype=access | stats count by host"
	incPiped := IncludeValue(pipedQuery, "host", "web01")
	if incPiped != `index=main sourcetype=access host="web01" | stats count by host` {
		t.Errorf("IncludeValue on piped query failed: got %q", incPiped)
	}
}

func TestDetectChartType(t *testing.T) {
	tests := []struct {
		query string
		want  ChartType
	}{
		{"index=main", ChartEvents},
		{"index=main | timechart span=1h count by host", ChartTimechart},
		{"index=main | stats count by status", ChartBar},
		{"index=main | chart count by method", ChartBar},
		{"index=main | top limit=10 host", ChartBar},
		{"index=main | timechart count | table _time count", ChartTimechart}, // timechart transforms earlier
	}

	for _, tt := range tests {
		got := DetectChartType(tt.query)
		if got != tt.want {
			t.Errorf("DetectChartType(%q) = %v, want %v", tt.query, got, tt.want)
		}
	}
}

func TestTableCommands(t *testing.T) {
	q1 := "index=main | table _time host status message"
	if !HasTableCommand(q1) {
		t.Errorf("expected HasTableCommand to be true for %q", q1)
	}

	cols := ExtractTableColumns(q1)
	expectedCols := []string{"_time", "host", "status", "message"}
	if !reflect.DeepEqual(cols, expectedCols) {
		t.Errorf("ExtractTableColumns(%q) = %v, want %v", q1, cols, expectedCols)
	}

	q2 := "index=main | stats count by host"
	if HasTableCommand(q2) {
		t.Errorf("expected HasTableCommand to be false for %q", q2)
	}
	if cols := ExtractTableColumns(q2); cols != nil {
		t.Errorf("expected nil columns, got %v", cols)
	}
}

func TestCompatibleChartTypes(t *testing.T) {
	cat := CompatibleChartTypes(ChartBar)
	if len(cat) != 3 || cat[0] != ChartBar || cat[1] != ChartPie || cat[2] != ChartColumn {
		t.Errorf("unexpected categorical types: %v", cat)
	}

	timeTypes := CompatibleChartTypes(ChartLine)
	if len(timeTypes) != 3 || timeTypes[0] != ChartLine || timeTypes[1] != ChartColumnTime || timeTypes[2] != ChartSparklines {
		t.Errorf("unexpected timechart types: %v", timeTypes)
	}
}

func TestExtractTimechartSpan(t *testing.T) {
	tests := []struct {
		query string
		want  string
	}{
		{"index=main | timechart span=1h count", "1h"},
		{"index=main | timechart span=5m count by host", "5m"},
		{"index=main | timechart count span=1d", "1d"},
		{"index=main | timechart count by host span=15m", "15m"},
		{"index=main | timechart span=\"30s\" count", "30s"},
		{"index=main | timechart span='10m' count", "10m"},
		{"index=main | timechart span = 2h count", "2h"},
		{"index=main | timechart bins=100 count", "bins=100"},
		{"index=main | stats count by host", ""},
		{"index=main | timechart count", ""},
	}

	for _, tt := range tests {
		got := ExtractTimechartSpan(tt.query)
		if got != tt.want {
			t.Errorf("ExtractTimechartSpan(%q) = %q, want %q", tt.query, got, tt.want)
		}
	}
}

func TestFormatSpanSeconds(t *testing.T) {
	if got := FormatSpanSeconds(3600); got != "1h" {
		t.Errorf("expected 1h, got %s", got)
	}
	if got := FormatSpanSeconds(300); got != "5m" {
		t.Errorf("expected 5m, got %s", got)
	}
	if got := FormatSpanSeconds(86400); got != "1d" {
		t.Errorf("expected 1d, got %s", got)
	}
	if got := FormatSpanSeconds(45); got != "45s" {
		t.Errorf("expected 45s, got %s", got)
	}
	if got := FormatSpanSeconds(0); got != "" {
		t.Errorf("expected empty string for 0, got %s", got)
	}
}

func TestExtractTimeBounds(t *testing.T) {
	tests := []struct {
		query        string
		wantEarliest string
		wantLatest   string
	}{
		{"index=main earliest=-24h latest=now", "-24h", "now"},
		{"index=main earliest=\"-7d@d\" latest='@d' | stats count", "-7d@d", "@d"},
		{"index=main latest=-1h | timechart count", "", "-1h"},
		{"index=main earliest=-15m | table _time host", "-15m", ""},
		{"index=main | stats count by sourcetype", "", ""},
	}

	for _, tt := range tests {
		e, l := ExtractTimeBounds(tt.query)
		if e != tt.wantEarliest || l != tt.wantLatest {
			t.Errorf("ExtractTimeBounds(%q) = (%q, %q), want (%q, %q)", tt.query, e, l, tt.wantEarliest, tt.wantLatest)
		}
	}
}

func TestExtractByAndLimit(t *testing.T) {
	tests := []struct {
		query     string
		wantBy    bool
		wantLimit int
		wantHas   bool
	}{
		{"index=main | timechart count BY host", true, 0, false},
		{"index=main | timechart limit=5 count by host", true, 5, true},
		{"index=main | timechart limit=0 count by host", true, 0, true},
		{"index=main | timechart count by host limit=15", true, 15, true},
		{"index=main | stats count by status", true, 0, false},
		{"index=main | stats limit=3 count by status", true, 3, true},
		{"index=main | stats limit=0 count by status", true, 0, true},
		{"index=main | chart count by host", true, 0, false},
		{"index=main | chart limit=20 count by host", true, 20, true},
		{"index=main | timechart count", false, 0, false},
		{"index=main | stats count", false, 0, false},
	}

	for _, tt := range tests {
		by, lim, has := ExtractByAndLimit(tt.query)
		if by != tt.wantBy || lim != tt.wantLimit || has != tt.wantHas {
			t.Errorf("ExtractByAndLimit(%q) = (%v, %d, %v), want (%v, %d, %v)", tt.query, by, lim, has, tt.wantBy, tt.wantLimit, tt.wantHas)
		}
	}
}

func TestEffectiveLimit(t *testing.T) {
	tests := []struct {
		query        string
		wantLimit    int
		wantUseOther bool
	}{
		// Default 10 when 'by' is used without limit
		{"index=main | timechart count BY host", 10, true},
		{"index=main | stats count by status", 10, true},
		{"index=main | chart count by host", 10, true},

		// Explicit limit
		{"index=main | timechart limit=5 count by host", 5, true},
		{"index=main | stats limit=3 count by status", 3, true},

		// limit=0 means ALL, no other
		{"index=main | timechart limit=0 count by host", 0, false},
		{"index=main | stats limit=0 count by status", 0, false},
		{"index=main | chart limit=0 count by host", 0, false},

		// No by and no limit
		{"index=main | timechart count", 0, false},
		{"index=main | stats count", 0, false},
	}

	for _, tt := range tests {
		lim, other := EffectiveLimit(tt.query)
		if lim != tt.wantLimit || other != tt.wantUseOther {
			t.Errorf("EffectiveLimit(%q) = (%d, %v), want (%d, %v)", tt.query, lim, other, tt.wantLimit, tt.wantUseOther)
		}
	}
}


