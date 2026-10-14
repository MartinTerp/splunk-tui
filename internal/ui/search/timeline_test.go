package search

import (
	"strings"
	"testing"
	"time"

	"splunk-tui/internal/splunk"
	"splunk-tui/internal/ui/theme"
)

func TestTimelineScrubAndZoom(t *testing.T) {
	th := theme.DefaultTheme()
	tb := NewTimelineBar(th)
	tb.SetWidth(80)

	baseTime := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	var buckets []splunk.TimelineBucket
	for i := 0; i < 60; i++ {
		buckets = append(buckets, splunk.TimelineBucket{
			Time:  baseTime.Add(time.Duration(i) * time.Minute),
			Count: int64(10 + i),
		})
	}

	tb.SetBuckets(buckets)

	if tb.IsScrubbing() {
		t.Fatalf("expected initial scrubbing to be false")
	}

	// Press '>' to start scrubbing
	tb.ScrubRight()
	if !tb.IsScrubbing() {
		t.Fatalf("expected scrubbing to become true after ScrubRight")
	}

	// View should contain scrubbing indicator
	view := tb.View()
	if !strings.Contains(view, "Scrub:") || !strings.Contains(view, "zoom in") {
		t.Errorf("expected view to contain scrubbing prompt, got:\n%s", view)
	}

	// Get scrub time bounds
	earliest, latest, count, ok := tb.GetScrubTimeBounds()
	if !ok {
		t.Fatalf("expected GetScrubTimeBounds to return ok=true")
	}
	if earliest == "" || latest == "" {
		t.Fatalf("expected valid earliest and latest timestamps, got %q, %q", earliest, latest)
	}
	if count <= 0 {
		t.Errorf("expected count > 0, got %d", count)
	}

	// Clear scrub
	tb.ClearScrub()
	if tb.IsScrubbing() {
		t.Fatalf("expected ClearScrub to turn off scrubbing")
	}
}
