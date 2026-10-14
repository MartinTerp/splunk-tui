package search

import (
	"fmt"
	"math"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/lipgloss"
	"splunk-tui/internal/splunk"
	"splunk-tui/internal/ui/theme"
)

// StatusBarModel renders the bottom status, progress, and pagination bar.
type StatusBarModel struct {
	theme       theme.Theme
	spinner     spinner.Model
	isSearching bool
	jobStatus   *splunk.JobStatus
	page        int
	pageSize    int
	totalCount  int64
	errMsg      string
	width       int
}

// NewStatusBar creates a status bar model.
func NewStatusBar(th theme.Theme) StatusBarModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(th.Primary).Bold(true)

	return StatusBarModel{
		theme:   th,
		spinner: s,
	}
}

// SetState updates search and pagination status.
func (m *StatusBarModel) SetState(searching bool, status *splunk.JobStatus, page, pageSize int, total int64, errStr string) {
	m.isSearching = searching
	m.jobStatus = status
	m.page = page
	m.pageSize = pageSize
	m.totalCount = total
	m.errMsg = errStr
}

// SetWidth updates width.
func (m *StatusBarModel) SetWidth(w int) {
	m.width = w
}

func (m StatusBarModel) View() string {
	boxWidth := m.width - 2
	if boxWidth < 40 {
		boxWidth = 40
	}

	var leftParts []string

	if m.isSearching {
		spinView := m.spinner.View()
		if m.jobStatus != nil {
			pct := int(math.Round(m.jobStatus.DoneProgress * 100))
			progressText := renderProgressBar(pct, 12)
			stateText := fmt.Sprintf("%s %s %d%% %s", spinView, m.jobStatus.DispatchState, pct, progressText)
			leftParts = append(leftParts, lipgloss.NewStyle().Foreground(m.theme.Primary).Bold(true).Render(stateText))
		} else {
			leftParts = append(leftParts, lipgloss.NewStyle().Foreground(m.theme.Primary).Bold(true).Render(spinView+" Submitting search..."))
		}
	} else if m.errMsg != "" {
		errIcon := lipgloss.NewStyle().Foreground(m.theme.Error).Bold(true).Render("✖ ")
		leftParts = append(leftParts, errIcon+lipgloss.NewStyle().Foreground(m.theme.Error).Render(m.errMsg))
	} else if m.jobStatus != nil && m.jobStatus.IsDone {
		leftParts = append(leftParts, lipgloss.NewStyle().Foreground(m.theme.Success).Render("✔ DONE"))
	} else {
		leftParts = append(leftParts, lipgloss.NewStyle().Foreground(m.theme.SubtleText).Render("READY"))
	}

	// Counts and duration
	if m.errMsg == "" && m.jobStatus != nil {
		var statsText string
		if m.jobStatus.ResultCount > 0 && m.jobStatus.EventCount > 0 && m.jobStatus.ResultCount != m.jobStatus.EventCount {
			statsText = fmt.Sprintf("%d results (%d events) • %d scanned • %.2fs", m.jobStatus.ResultCount, m.jobStatus.EventCount, m.jobStatus.ScanCount, m.jobStatus.RunDuration)
		} else if m.jobStatus.ResultCount > 0 {
			statsText = fmt.Sprintf("%d results • %d scanned • %.2fs", m.jobStatus.ResultCount, m.jobStatus.ScanCount, m.jobStatus.RunDuration)
		} else {
			statsText = fmt.Sprintf("%d events • %d scanned • %.2fs", m.jobStatus.EventCount, m.jobStatus.ScanCount, m.jobStatus.RunDuration)
		}
		leftParts = append(leftParts, lipgloss.NewStyle().Foreground(m.theme.SubtleText).Render(statsText))
	}

	// Pagination
	if m.errMsg == "" {
		totalPages := 1
		if m.pageSize > 0 && m.totalCount > 0 {
			totalPages = int(math.Ceil(float64(m.totalCount) / float64(m.pageSize)))
		}
		pageText := fmt.Sprintf("Page %d/%d (n/p)", m.page, totalPages)
		leftParts = append(leftParts, lipgloss.NewStyle().Foreground(m.theme.Highlight).Render(pageText))
	}

	leftContent := strings.Join(leftParts, "  │  ")

	innerW := boxWidth - 2
	if innerW < 20 {
		innerW = 20
	}

	var rightContent string
	if m.isSearching {
		rightContent = lipgloss.NewStyle().Foreground(m.theme.Highlight).Bold(true).Render("esc: stop") +
			lipgloss.NewStyle().Foreground(m.theme.SubtleText).Render(" • ? help • q quit")
	} else {
		fullHints := "? help • / search • v chart • ctrl+k configs • ctrl+o switch • q quit"
		shortHints := "? help • v chart • ctrl+k • ctrl+o • q"
		minHints := "? help • q"

		avail := innerW - lipgloss.Width(leftContent) - 2
		if lipgloss.Width(fullHints) <= avail {
			rightContent = lipgloss.NewStyle().Foreground(m.theme.SubtleText).Render(fullHints)
		} else if lipgloss.Width(shortHints) <= avail {
			rightContent = lipgloss.NewStyle().Foreground(m.theme.SubtleText).Render(shortHints)
		} else if lipgloss.Width(minHints) <= avail {
			rightContent = lipgloss.NewStyle().Foreground(m.theme.SubtleText).Render(minHints)
		} else {
			rightContent = ""
		}
	}

	maxLeft := innerW - lipgloss.Width(rightContent) - 2
	if maxLeft < 10 {
		maxLeft = 10
	}
	if lipgloss.Width(leftContent) > maxLeft {
		leftContent = truncate(leftContent, maxLeft)
	}

	padding := innerW - lipgloss.Width(leftContent) - lipgloss.Width(rightContent)
	if padding < 1 {
		padding = 1
	}

	bar := leftContent + strings.Repeat(" ", padding) + rightContent
	return m.theme.StatusBar.Width(m.width).MaxHeight(1).Render(bar)
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
