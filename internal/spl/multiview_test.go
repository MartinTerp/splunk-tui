package spl

import (
	"testing"
)

func TestIsMultiViewSupported(t *testing.T) {
	tests := []struct {
		query     string
		supported bool
		wantCmd   MultiViewCommandType
		wantChart ChartType
	}{
		{"index=main", false, MultiViewNone, ChartNone},
		{"index=main | search error", false, MultiViewNone, ChartNone},
		{"index=main | timechart span=1h count by sourcetype", true, MultiViewTimechart, ChartLine},
		{"index=main | chart count over host by status", true, MultiViewChart, ChartBar},
		{"index=main | stats count, avg(duration) by host", true, MultiViewStats, ChartBar},
		{"index=main | table _time host status message", true, MultiViewTable, ChartNone},
		{"| stats count", true, MultiViewStats, ChartBar},
		{"index=main | timechart count | table _time count", true, MultiViewTable, ChartNone},
		{"index=main [ search index=audit | stats count ] | timechart count", true, MultiViewTimechart, ChartLine},
	}

	for _, tt := range tests {
		supported, cmdType, chartType, reason := IsMultiViewSupported(tt.query)
		if supported != tt.supported {
			t.Errorf("IsMultiViewSupported(%q) supported = %v, want %v (reason: %s)", tt.query, supported, tt.supported, reason)
		}
		if cmdType != tt.wantCmd {
			t.Errorf("IsMultiViewSupported(%q) cmdType = %v, want %v", tt.query, cmdType, tt.wantCmd)
		}
		if chartType != tt.wantChart {
			t.Errorf("IsMultiViewSupported(%q) chartType = %v, want %v", tt.query, chartType, tt.wantChart)
		}
	}
}
