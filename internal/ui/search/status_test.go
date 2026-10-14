package search

import (
	"strings"
	"testing"

	"splunk-tui/internal/splunk"
	"splunk-tui/internal/ui/theme"
)

func TestStatusBarErrorState(t *testing.T) {
	th := theme.DefaultTheme()
	sb := NewStatusBar(th)
	sb.SetWidth(120)

	// In error state
	sb.SetState(false, nil, 1, 20, 0, "Error in 'stats' command: invalid argument")
	view := sb.View()

	if !strings.Contains(view, "Error in 'stats' command") {
		t.Errorf("expected status bar to contain error message, got:\n%s", view)
	}
	if strings.Contains(view, "Page 1/1") {
		t.Errorf("expected status bar not to contain pagination when in error state, got:\n%s", view)
	}

	// In success state
	status := &splunk.JobStatus{
		IsDone:      true,
		EventCount:  42,
		ResultCount: 42,
		ScanCount:   100,
		RunDuration: 0.5,
	}
	sb.SetState(false, status, 1, 20, 42, "")
	viewSuccess := sb.View()

	if !strings.Contains(viewSuccess, "DONE") {
		t.Errorf("expected view to contain DONE, got:\n%s", viewSuccess)
	}
	if !strings.Contains(viewSuccess, "Page 1/3") {
		t.Errorf("expected view to contain Page 1/3, got:\n%s", viewSuccess)
	}
}
