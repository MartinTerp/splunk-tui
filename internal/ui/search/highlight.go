package search

import (
	"bytes"
	"encoding/json"
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

var (
	reJSONKey   = regexp.MustCompile(`^(\s*)("([^"\\]*(\\.[^"\\]*)*)")\s*:(.*)$`)
	reJSONValue = regexp.MustCompile(`("([^"\\]*(\\.[^"\\]*)*)"|-?[0-9]+(?:\.[0-9]+)?(?:[eE][+-]?[0-9]+)?\b|\btrue\b|\bfalse\b|\bnull\b|[{}\[\],])`)
)

// FormatRawJSON attempts to parse raw log text as JSON and return pretty-printed JSON.
// It returns the formatted text and true if raw is valid JSON (or contains a JSON payload),
// or the original text and false if not.
func FormatRawJSON(raw string) (string, bool) {
	trimmed := strings.TrimSpace(raw)
	if len(trimmed) < 2 {
		return raw, false
	}

	// 1. Direct JSON Object or Array
	if (strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}")) ||
		(strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]")) {
		var buf bytes.Buffer
		if err := json.Indent(&buf, []byte(trimmed), "", "  "); err == nil {
			return buf.String(), true
		}
	}

	// 2. Prefixed log with JSON payload (e.g. "TIMESTAMP [LEVEL] {...}")
	firstBrace := strings.Index(trimmed, "{")
	if firstBrace > 0 && strings.HasSuffix(trimmed, "}") {
		prefix := strings.TrimSpace(trimmed[:firstBrace])
		jsonPart := trimmed[firstBrace:]
		var buf bytes.Buffer
		if err := json.Indent(&buf, []byte(jsonPart), "", "  "); err == nil {
			return prefix + "\n" + buf.String(), true
		}
	}

	firstBracket := strings.Index(trimmed, "[")
	if firstBracket > 0 && strings.HasSuffix(trimmed, "]") {
		prefix := strings.TrimSpace(trimmed[:firstBracket])
		jsonPart := trimmed[firstBracket:]
		var buf bytes.Buffer
		if err := json.Indent(&buf, []byte(jsonPart), "", "  "); err == nil {
			return prefix + "\n" + buf.String(), true
		}
	}

	return raw, false
}

// HighlightJSON applies syntax highlighting to pretty-printed JSON lines.
func HighlightJSON(prettyJSON string, th theme.Theme) string {
	if prettyJSON == "" {
		return ""
	}

	keyStyle := lipgloss.NewStyle().Foreground(th.Highlight).Bold(true)
	strStyle := lipgloss.NewStyle().Foreground(th.Success)
	numStyle := lipgloss.NewStyle().Foreground(th.Warning)
	boolStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#C678DD")).Bold(true)
	nullStyle := lipgloss.NewStyle().Foreground(th.SubtleText).Italic(true)
	punctStyle := lipgloss.NewStyle().Foreground(th.SubtleText)

	lines := strings.Split(prettyJSON, "\n")
	highlightedLines := make([]string, len(lines))

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			highlightedLines[i] = line
			continue
		}

		// Check if line looks like a log prefix line (not starting with JSON delimiters, quotes, or values)
		firstChar := trimmed[0]
		isJSONLine := strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "}") ||
			strings.HasPrefix(trimmed, "[") || strings.HasPrefix(trimmed, "]") ||
			strings.HasPrefix(trimmed, `"`) || (firstChar >= '0' && firstChar <= '9') ||
			firstChar == '-' || strings.HasPrefix(trimmed, "true") ||
			strings.HasPrefix(trimmed, "false") || strings.HasPrefix(trimmed, "null")

		if !isJSONLine {
			highlightedLines[i] = HighlightLogText(line, th)
			continue
		}

		if match := reJSONKey.FindStringSubmatch(line); len(match) >= 6 {
			indent := match[1]
			key := match[2]
			colon := ":"
			rest := match[5]

			styledKey := keyStyle.Render(key)
			styledColon := punctStyle.Render(colon)
			styledRest := reJSONValue.ReplaceAllStringFunc(rest, func(token string) string {
				return styleJSONToken(token, strStyle, numStyle, boolStyle, nullStyle, punctStyle)
			})

			highlightedLines[i] = indent + styledKey + styledColon + styledRest
		} else {
			highlightedLines[i] = reJSONValue.ReplaceAllStringFunc(line, func(token string) string {
				return styleJSONToken(token, strStyle, numStyle, boolStyle, nullStyle, punctStyle)
			})
		}
	}

	return strings.Join(highlightedLines, "\n")
}

func styleJSONToken(token string, strStyle, numStyle, boolStyle, nullStyle, punctStyle lipgloss.Style) string {
	if strings.HasPrefix(token, `"`) {
		return strStyle.Render(token)
	}
	switch token {
	case "true", "false":
		return boolStyle.Render(token)
	case "null":
		return nullStyle.Render(token)
	case "{", "}", "[", "]", ",":
		return punctStyle.Render(token)
	default:
		// Numbers
		return numStyle.Render(token)
	}
}
