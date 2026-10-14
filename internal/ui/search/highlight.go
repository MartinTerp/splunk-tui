package search

import (
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"splunk-tui/internal/ui/theme"
)

// LogLevel represents the severity of a log event.
type LogLevel int

const (
	LogLevelUnknown LogLevel = iota
	LogLevelDebug
	LogLevelInfo
	LogLevelWarn
	LogLevelError
	LogLevelFatal
)

var (
	reFatal = regexp.MustCompile(`(?i)\b(FATAL|CRITICAL|EMERGENCY|SEVERE)\b`)
	reError = regexp.MustCompile(`(?i)\b(ERROR|ERR|FAILED|FAILURE)\b`)
	reWarn  = regexp.MustCompile(`(?i)\b(WARN|WARNING)\b`)
	reInfo  = regexp.MustCompile(`(?i)\b(INFO|NOTICE)\b`)
	reDebug = regexp.MustCompile(`(?i)\b(DEBUG|TRACE)\b`)

	reIPv4 = regexp.MustCompile(`\b(?:[0-9]{1,3}\.){3}[0-9]{1,3}(?::[0-9]{1,5})?\b`)
	reHTTP = regexp.MustCompile(`\b([1-5][0-9]{2})\b`)
	reKV   = regexp.MustCompile(`\b([a-zA-Z0-9_\-\.]+)=(".*?"|'.*?'|[^\s,;]+)`)
)

// DetectLogLevel inspects text to identify the predominant log severity level.
func DetectLogLevel(text string) LogLevel {
	if reFatal.MatchString(text) {
		return LogLevelFatal
	}
	if reError.MatchString(text) {
		return LogLevelError
	}
	if reWarn.MatchString(text) {
		return LogLevelWarn
	}
	if reInfo.MatchString(text) {
		return LogLevelInfo
	}
	if reDebug.MatchString(text) {
		return LogLevelDebug
	}
	return LogLevelUnknown
}

// LogLevelBadge returns a compact colored status marker for the given log level.
func LogLevelBadge(level LogLevel, th theme.Theme) string {
	switch level {
	case LogLevelFatal:
		return lipgloss.NewStyle().Foreground(th.Error).Bold(true).Render("✖ FATAL")
	case LogLevelError:
		return lipgloss.NewStyle().Foreground(th.Error).Bold(true).Render("✖ ERROR")
	case LogLevelWarn:
		return lipgloss.NewStyle().Foreground(th.Warning).Bold(true).Render("⚠ WARN")
	case LogLevelInfo:
		return lipgloss.NewStyle().Foreground(th.Success).Render("● INFO")
	case LogLevelDebug:
		return lipgloss.NewStyle().Foreground(th.SubtleText).Render("○ DEBUG")
	default:
		return ""
	}
}

// HighlightLogText applies syntax highlighting to raw log text, colorizing log levels,
// key=value pairs, IP addresses, and HTTP status codes.
func HighlightLogText(text string, th theme.Theme) string {
	if text == "" {
		return ""
	}

	fatalStyle := lipgloss.NewStyle().Foreground(th.Error).Bold(true)
	errorStyle := lipgloss.NewStyle().Foreground(th.Error).Bold(true)
	warnStyle := lipgloss.NewStyle().Foreground(th.Warning).Bold(true)
	infoStyle := lipgloss.NewStyle().Foreground(th.Success).Bold(true)
	debugStyle := lipgloss.NewStyle().Foreground(th.SubtleText).Italic(true)
	ipStyle := lipgloss.NewStyle().Foreground(th.Highlight).Underline(false)
	keyStyle := lipgloss.NewStyle().Foreground(th.Highlight)
	valStyle := lipgloss.NewStyle().Foreground(th.Text)

	// 1. Highlight Key=Value pairs
	res := reKV.ReplaceAllStringFunc(text, func(m string) string {
		parts := strings.SplitN(m, "=", 2)
		if len(parts) == 2 {
			k := keyStyle.Render(parts[0])
			v := valStyle.Render(parts[1])
			return k + "=" + v
		}
		return m
	})

	// 2. Highlight IP addresses
	res = reIPv4.ReplaceAllStringFunc(res, func(m string) string {
		return ipStyle.Render(m)
	})

	// 3. Highlight Log Levels
	res = reFatal.ReplaceAllStringFunc(res, func(m string) string {
		return fatalStyle.Render(strings.ToUpper(m))
	})
	res = reError.ReplaceAllStringFunc(res, func(m string) string {
		return errorStyle.Render(strings.ToUpper(m))
	})
	res = reWarn.ReplaceAllStringFunc(res, func(m string) string {
		return warnStyle.Render(strings.ToUpper(m))
	})
	res = reInfo.ReplaceAllStringFunc(res, func(m string) string {
		return infoStyle.Render(strings.ToUpper(m))
	})
	res = reDebug.ReplaceAllStringFunc(res, func(m string) string {
		return debugStyle.Render(strings.ToUpper(m))
	})

	return res
}
