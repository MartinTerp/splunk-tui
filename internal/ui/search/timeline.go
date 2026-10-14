package search

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"splunk-tui/internal/splunk"
	"splunk-tui/internal/ui/theme"
)

// TimelineBarModel renders a compact visual density bar to detect gaps in data,
// with interactive scrubbing (< / >) and zoom-in (z).
type TimelineBarModel struct {
	theme     theme.Theme
	buckets   []splunk.TimelineBucket
	span      string
	width     int
	scrubbing bool
	cursorCol int
}

// NewTimelineBar creates a timeline bar.
func NewTimelineBar(th theme.Theme) TimelineBarModel {
	return TimelineBarModel{
		theme: th,
	}
}

// SetBuckets updates the timeline buckets.
func (m *TimelineBarModel) SetBuckets(buckets []splunk.TimelineBucket) {
	m.buckets = buckets
	if len(buckets) == 0 {
		m.scrubbing = false
		m.cursorCol = 0
	}
}

// SetSpan sets an explicit timechart span label.
func (m *TimelineBarModel) SetSpan(span string) {
	m.span = span
}

// SetWidth updates width.
func (m *TimelineBarModel) SetWidth(w int) {
	m.width = w
}

func (m TimelineBarModel) barWidth() int {
	cardW := m.width - 4
	if cardW < 20 {
		cardW = 20
	}
	contentWidth := cardW - 2
	prefixLen := len("Timeline: ")
	if m.span != "" {
		prefixLen = len(fmt.Sprintf("Timeline (span: %s): ", m.span))
	}
	bw := contentWidth - prefixLen
	if bw < 10 {
		bw = 10
	}
	return bw
}

// ScrubLeft moves the scrubbing cursor left.
func (m *TimelineBarModel) ScrubLeft() {
	if len(m.buckets) == 0 {
		return
	}
	bw := m.barWidth()
	if bw <= 0 {
		return
	}
	if !m.scrubbing {
		m.scrubbing = true
		m.cursorCol = bw / 2
		return
	}
	if m.cursorCol > 0 {
		m.cursorCol--
	}
}

// ScrubRight moves the scrubbing cursor right.
func (m *TimelineBarModel) ScrubRight() {
	if len(m.buckets) == 0 {
		return
	}
	bw := m.barWidth()
	if bw <= 0 {
		return
	}
	if !m.scrubbing {
		m.scrubbing = true
		m.cursorCol = bw / 2
		return
	}
	if m.cursorCol < bw-1 {
		m.cursorCol++
	}
}

// IsScrubbing reports whether timeline scrubbing is currently active.
func (m TimelineBarModel) IsScrubbing() bool {
	return m.scrubbing
}

// ClearScrub exits scrubbing mode.
func (m *TimelineBarModel) ClearScrub() {
	m.scrubbing = false
}

// GetScrubTimeBounds calculates the time window and event count under the current scrubber cursor.
func (m TimelineBarModel) GetScrubTimeBounds() (earliest string, latest string, count int64, ok bool) {
	if len(m.buckets) == 0 || !m.scrubbing {
		return "", "", 0, false
	}
	bw := m.barWidth()
	if bw <= 0 {
		return "", "", 0, false
	}

	ratio := float64(len(m.buckets)) / float64(bw)
	start := int(float64(m.cursorCol) * ratio)
	end := int(float64(m.cursorCol+1) * ratio)
	if end > len(m.buckets) {
		end = len(m.buckets)
	}
	if start >= len(m.buckets) {
		start = len(m.buckets) - 1
	}
	if start < 0 {
		start = 0
	}
	if end <= start {
		end = start + 1
	}

	var sumCount int64
	for i := start; i < end; i++ {
		sumCount += m.buckets[i].Count
	}

	bStart := m.buckets[start].Time
	bEnd := m.buckets[end-1].Time

	var step time.Duration = time.Minute
	if len(m.buckets) > 1 {
		step = m.buckets[1].Time.Sub(m.buckets[0].Time)
		if step <= 0 {
			step = time.Minute
		}
	}

	tLatest := bEnd.Add(step)
	return bStart.UTC().Format(time.RFC3339), tLatest.UTC().Format(time.RFC3339), sumCount, true
}

func (m TimelineBarModel) View() string {
	cardW := m.width - 4
	if cardW < 20 {
		cardW = 20
	}
	contentWidth := cardW - 2
	if contentWidth < 10 {
		contentWidth = 10
	}

	label := "Timeline"
	if m.span != "" {
		label = fmt.Sprintf("Timeline (span: %s)", m.span)
	}
	prefix := label + ": "

	if len(m.buckets) == 0 {
		emptyBarW := contentWidth - len(prefix)
		if emptyBarW < 2 {
			emptyBarW = 2
		}
		emptyBar := strings.Repeat("┄", emptyBarW)
		return m.theme.Card.Width(cardW).Height(2).MaxHeight(4).Render(
			lipgloss.NewStyle().Foreground(m.theme.SubtleText).Render(prefix + emptyBar),
		)
	}

	barWidth := contentWidth - len(prefix)
	if barWidth < 10 {
		barWidth = 10
	}

	colCounts := make([]int64, barWidth)
	var maxCount int64

	ratio := float64(len(m.buckets)) / float64(barWidth)
	for col := 0; col < barWidth; col++ {
		start := int(float64(col) * ratio)
		end := int(float64(col+1) * ratio)
		if end > len(m.buckets) {
			end = len(m.buckets)
		}
		if start >= len(m.buckets) {
			start = len(m.buckets) - 1
		}
		if end <= start {
			end = start + 1
		}

		var count int64
		for i := start; i < end; i++ {
			count += m.buckets[i].Count
		}
		colCounts[col] = count
		if count > maxCount {
			maxCount = count
		}
	}

	var bar strings.Builder

	for col, cnt := range colCounts {
		if m.scrubbing && col == m.cursorCol {
			// Inverted scrubber cursor
			cursorMarker := lipgloss.NewStyle().
				Foreground(m.theme.Background).
				Background(m.theme.Highlight).
				Bold(true).
				Render("▲")
			bar.WriteString(cursorMarker)
			continue
		}

		if cnt == 0 {
			gapMark := lipgloss.NewStyle().Foreground(m.theme.Warning).Render(" ")
			bar.WriteString(gapMark)
		} else {
			fraction := float64(cnt) / float64(maxCount)
			idx := int(fraction * float64(len(blockLevels)-1))
			if idx < 2 {
				idx = 2
			}
			if idx >= len(blockLevels) {
				idx = len(blockLevels) - 1
			}
			block := lipgloss.NewStyle().Foreground(m.theme.Primary).Render(string(blockLevels[idx]))
			bar.WriteString(block)
		}
	}

	line1 := fmt.Sprintf("%s%s", prefix, bar.String())

	var line2 string
	if m.scrubbing {
		earliest, latest, cnt, ok := m.GetScrubTimeBounds()
		if ok {
			eT, _ := time.Parse(time.RFC3339, earliest)
			lT, _ := time.Parse(time.RFC3339, latest)
			line2 = fmt.Sprintf("Scrub: [%s - %s] (%d events) • z: zoom in • esc: clear", eT.Format("15:04:05"), lT.Format("15:04:05"), cnt)
		}
	} else {
		startTime := m.buckets[0].Time.Format("15:04:05")
		endTime := m.buckets[len(m.buckets)-1].Time.Format("15:04:05")
		padding := barWidth - len(startTime) - len(endTime)
		if padding < 2 {
			padding = 2
		}
		line2 = fmt.Sprintf("%s%s%s%s", strings.Repeat(" ", len(prefix)), startTime, strings.Repeat(" ", padding), endTime)
	}

	styledLine2 := lipgloss.NewStyle().Foreground(m.theme.Highlight).Render(line2)
	if !m.scrubbing {
		styledLine2 = lipgloss.NewStyle().Foreground(m.theme.SubtleText).Render(line2)
	}

	cardStyle := m.theme.Card
	if m.scrubbing {
		cardStyle = m.theme.FocusedCard
	}

	return cardStyle.Width(cardW).MaxHeight(4).Render(lipgloss.JoinVertical(lipgloss.Left, line1, styledLine2))
}
