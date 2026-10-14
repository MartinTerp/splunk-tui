package spl

// MultiViewCommandType identifies the transforming command used in Multi View.
type MultiViewCommandType int

const (
	MultiViewNone MultiViewCommandType = iota
	MultiViewTimechart
	MultiViewChart
	MultiViewStats
	MultiViewTable
)

// Label returns a display label for the Multi View command type.
func (c MultiViewCommandType) Label() string {
	switch c {
	case MultiViewTimechart:
		return "timechart"
	case MultiViewChart:
		return "chart"
	case MultiViewStats:
		return "stats"
	case MultiViewTable:
		return "table"
	default:
		return "none"
	}
}

// IsMultiViewSupported validates whether an SPL query is supported in Multi View.
// Multi View strictly requires | timechart, | chart, | stats, or | table.
// Returns:
//   - supported: true if supported
//   - cmdType: MultiViewCommandType
//   - chartType: base ChartType
//   - reason: helpful explanation if unsupported
func IsMultiViewSupported(query string) (bool, MultiViewCommandType, ChartType, string) {
	pipes := SplitTopLevelPipes(query)
	if len(pipes) == 0 {
		return false, MultiViewNone, ChartNone, "Query is empty"
	}

	// Search backwards for the last transforming/table command
	for i := len(pipes) - 1; i >= 0; i-- {
		cmd := extractCommand(pipes[i])
		switch cmd {
		case "timechart":
			return true, MultiViewTimechart, ChartLine, ""
		case "chart":
			return true, MultiViewChart, ChartBar, ""
		case "stats":
			return true, MultiViewStats, ChartBar, ""
		case "table":
			return true, MultiViewTable, ChartNone, ""
		}
	}

	return false, MultiViewNone, ChartNone, "Multi View requires a transforming command: | timechart, | chart, | stats, or | table"
}
