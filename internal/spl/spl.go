// Package spl provides utilities for analyzing and modifying Splunk Processing Language (SPL) queries.
package spl

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// ChartType represents the visualization mode for query results.
type ChartType int

const (
	ChartNone ChartType = iota
	ChartEvents
	ChartBar
	ChartPie
	ChartColumn
	ChartLine
	ChartSparklines
	ChartColumnTime
)

// ChartTimechart is a backward-compatible alias for ChartLine.
const ChartTimechart = ChartLine

// Label returns a short display name for the chart type.
func (c ChartType) Label() string {
	switch c {
	case ChartEvents:
		return "Timeline"
	case ChartBar:
		return "Bar"
	case ChartPie:
		return "Pie"
	case ChartColumn:
		return "Column"
	case ChartLine:
		return "Line"
	case ChartSparklines:
		return "Sparklines"
	case ChartColumnTime:
		return "Bar"
	default:
		return "None"
	}
}

// CompatibleChartTypes returns visualizations supported for the given base chart type.
func CompatibleChartTypes(base ChartType) []ChartType {
	switch base {
	case ChartBar, ChartPie, ChartColumn:
		return []ChartType{ChartBar, ChartPie, ChartColumn}
	case ChartLine, ChartColumnTime, ChartSparklines:
		return []ChartType{ChartLine, ChartColumnTime, ChartSparklines}
	case ChartEvents:
		return []ChartType{ChartEvents}
	default:
		return nil
	}
}

// QuoteValue formats a field value safely inside double quotes,
// escaping internal quotes and backslashes.
func QuoteValue(v string) string {
	escaped := strings.ReplaceAll(v, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	return fmt.Sprintf(`"%s"`, escaped)
}

// FormatCondition builds a field inclusion or exclusion clause.
//   inclusion: field="value"
//   exclusion: field!="value"
func FormatCondition(field, value string, include bool) string {
	op := "="
	if !include {
		op = "!="
	}
	return fmt.Sprintf(`%s%s%s`, field, op, QuoteValue(value))
}

// SplitTopLevelPipes splits an SPL query on top-level '|' characters,
// correctly ignoring pipes inside quotes and subsearches [...].
func SplitTopLevelPipes(query string) []string {
	var parts []string
	var current strings.Builder

	inQuotes := false
	quoteChar := rune(0)
	subsearchDepth := 0
	escaped := false

	runes := []rune(query)
	for i := 0; i < len(runes); i++ {
		r := runes[i]

		if escaped {
			current.WriteRune(r)
			escaped = false
			continue
		}

		if r == '\\' {
			current.WriteRune(r)
			escaped = true
			continue
		}

		if inQuotes {
			current.WriteRune(r)
			if r == quoteChar {
				inQuotes = false
			}
			continue
		}

		switch r {
		case '"', '\'':
			inQuotes = true
			quoteChar = r
			current.WriteRune(r)
		case '[':
			subsearchDepth++
			current.WriteRune(r)
		case ']':
			if subsearchDepth > 0 {
				subsearchDepth--
			}
			current.WriteRune(r)
		case '|':
			if subsearchDepth == 0 {
				parts = append(parts, current.String())
				current.Reset()
				continue
			}
			current.WriteRune(r)
		default:
			current.WriteRune(r)
		}
	}

	parts = append(parts, current.String())
	return parts
}

// ModifyBaseSearch injects a filter clause into the base search segment (before the first pipe).
func ModifyBaseSearch(query, clause string) string {
	q := strings.TrimSpace(query)
	clause = strings.TrimSpace(clause)
	if clause == "" {
		return q
	}
	if q == "" {
		return clause
	}

	pipes := SplitTopLevelPipes(q)
	if len(pipes) == 0 {
		return clause
	}

	// First part is the base search
	base := strings.TrimSpace(pipes[0])

	// If the entire query started with a pipe (generating command e.g. "| inputlookup hosts")
	if strings.HasPrefix(q, "|") {
		// Append as "| search <clause>"
		filterCmd := fmt.Sprintf("search %s", clause)
		if len(pipes) > 1 {
			// Insert after the generating command
			newPipes := append([]string{pipes[0], " " + filterCmd + " "}, pipes[1:]...)
			return strings.Join(newPipes, "|")
		}
		return q + " | " + filterCmd
	}

	// Normal search: append clause to base search
	newBase := base + " " + clause
	pipes[0] = newBase

	for i := range pipes {
		pipes[i] = strings.TrimSpace(pipes[i])
	}

	return strings.Join(pipes, " | ")
}

// IncludeValue adds `field="value"` to the base search.
func IncludeValue(query, field, value string) string {
	clause := FormatCondition(field, value, true)
	return ModifyBaseSearch(query, clause)
}

// ExcludeValue adds `field!="value"` to the base search.
func ExcludeValue(query, field, value string) string {
	clause := FormatCondition(field, value, false)
	return ModifyBaseSearch(query, clause)
}

// DetectChartType analyzes the SPL pipes to determine which chart view to display.
func DetectChartType(query string) ChartType {
	pipes := SplitTopLevelPipes(query)
	// Iterate pipes backwards to find the last transforming visualization command
	for i := len(pipes) - 1; i >= 0; i-- {
		cmd := extractCommand(pipes[i])
		switch cmd {
		case "timechart":
			return ChartTimechart
		case "chart", "stats", "top", "rare":
			return ChartBar
		}
	}
	return ChartEvents
}

// HasTableCommand checks if the query contains a `| table ...` pipe.
func HasTableCommand(query string) bool {
	pipes := SplitTopLevelPipes(query)
	for _, p := range pipes {
		if extractCommand(p) == "table" {
			return true
		}
	}
	return false
}

// ExtractTableColumns parses field names from the last `| table ...` command.
func ExtractTableColumns(query string) []string {
	pipes := SplitTopLevelPipes(query)
	for i := len(pipes) - 1; i >= 0; i-- {
		p := strings.TrimSpace(pipes[i])
		if extractCommand(p) == "table" {
			// Remove the word "table"
			fieldsPart := strings.TrimSpace(p[len("table"):])
			fields := strings.Fields(fieldsPart)
			var cols []string
			for _, f := range fields {
				col := strings.Trim(f, `,"'`)
				if col != "" {
					cols = append(cols, col)
				}
			}
			return cols
		}
	}
	return nil
}

// ExtractTimechartSpan parses the span duration string from an SPL query containing a timechart command.
// Examples:
//   "index=main | timechart span=1h count" -> "1h"
//   "index=main | timechart span=5m count by host" -> "5m"
//   "index=main | timechart span="10s" count" -> "10s"
//   "index=main | timechart count span=1d" -> "1d"
//   "index=main | timechart bins=100 count" -> "bins=100"
func ExtractTimechartSpan(query string) string {
	pipes := SplitTopLevelPipes(query)
	for i := len(pipes) - 1; i >= 0; i-- {
		p := strings.TrimSpace(pipes[i])
		if extractCommand(p) != "timechart" {
			continue
		}

		fields := strings.Fields(p)
		for _, f := range fields {
			lower := strings.ToLower(f)
			if strings.HasPrefix(lower, "span=") {
				val := strings.TrimPrefix(lower, "span=")
				return strings.Trim(val, `,"'`)
			}
			if strings.HasPrefix(lower, "bins=") {
				val := strings.TrimPrefix(lower, "bins=")
				return "bins=" + strings.Trim(val, `,"'`)
			}
		}

		// Also handle spaces around '=' e.g. "span = 1h"
		lower := strings.ToLower(p)
		if idx := strings.Index(lower, "span"); idx != -1 {
			rem := strings.TrimSpace(p[idx+len("span"):])
			if strings.HasPrefix(rem, "=") {
				valPart := strings.TrimSpace(rem[1:])
				val := readToken(valPart)
				if val != "" {
					return val
				}
			}
		}
	}
	return ""
}

// FormatSpanSeconds converts a duration in seconds into a standard Splunk span string (e.g. 3600 -> "1h").
func FormatSpanSeconds(sec int64) string {
	if sec <= 0 {
		return ""
	}
	if sec%86400 == 0 {
		return fmt.Sprintf("%dd", sec/86400)
	}
	if sec%3600 == 0 {
		return fmt.Sprintf("%dh", sec/3600)
	}
	if sec%60 == 0 {
		return fmt.Sprintf("%dm", sec/60)
	}
	return fmt.Sprintf("%ds", sec)
}

func readToken(s string) string {
	s = strings.TrimSpace(s)
	if len(s) == 0 {
		return ""
	}
	if s[0] == '"' || s[0] == '\'' {
		quote := s[0]
		end := strings.IndexByte(s[1:], quote)
		if end != -1 {
			return s[1 : end+1]
		}
		return strings.Trim(s, `"'`)
	}
	end := strings.IndexFunc(s, func(r rune) bool {
		return unicode.IsSpace(r) || r == ',' || r == '|'
	})
	if end != -1 {
		return s[:end]
	}
	return s
}

// extractCommand extracts the primary command word from a pipe segment.
func extractCommand(pipeSegment string) string {
	trimmed := strings.TrimLeftFunc(pipeSegment, unicode.IsSpace)
	fields := strings.Fields(trimmed)
	if len(fields) == 0 {
		return ""
	}
	return strings.ToLower(fields[0])
}

// ExtractTimeBounds parses explicit earliest= and latest= parameters from an SPL query.
func ExtractTimeBounds(query string) (string, string) {
	var earliest, latest string
	tokens := strings.Fields(query)
	for _, tok := range tokens {
		lower := strings.ToLower(tok)
		if strings.HasPrefix(lower, "earliest=") {
			val := tok[len("earliest="):]
			val = strings.Trim(val, "\"'")
			earliest = val
		} else if strings.HasPrefix(lower, "latest=") {
			val := tok[len("latest="):]
			val = strings.Trim(val, "\"'")
			latest = val
		}
	}
	return earliest, latest
}

// ExtractByAndLimit parses the last transforming command (timechart, chart, stats)
// in the query to determine if a 'BY' clause is present and if a 'limit=N' option was set.
func ExtractByAndLimit(query string) (hasBy bool, limit int, hasLimit bool) {
	pipes := SplitTopLevelPipes(query)
	for i := len(pipes) - 1; i >= 0; i-- {
		p := strings.TrimSpace(pipes[i])
		cmd := extractCommand(p)
		if cmd != "timechart" && cmd != "chart" && cmd != "stats" && cmd != "timeline" {
			continue
		}

		fields := strings.Fields(p)
		for j, f := range fields {
			lower := strings.ToLower(f)
			if lower == "by" || strings.HasPrefix(lower, "by(") || strings.HasPrefix(lower, "by=") {
				hasBy = true
			}
			if strings.HasPrefix(lower, "limit=") {
				valStr := strings.TrimPrefix(lower, "limit=")
				valStr = strings.Trim(valStr, `,"'`)
				if v, err := strconv.Atoi(valStr); err == nil && v >= 0 {
					limit = v
					hasLimit = true
				}
			} else if lower == "limit" && j+1 < len(fields) {
				next := strings.Trim(fields[j+1], `,"'`)
				if strings.HasPrefix(next, "=") {
					next = strings.TrimPrefix(next, "=")
					if next == "" && j+2 < len(fields) {
						next = strings.Trim(fields[j+2], `,"'`)
					}
				}
				if v, err := strconv.Atoi(next); err == nil && v >= 0 {
					limit = v
					hasLimit = true
				}
			}
		}

		// Also check raw string in case spaces around '=' e.g. "limit = 10"
		if !hasLimit {
			lower := strings.ToLower(p)
			if idx := strings.Index(lower, "limit"); idx != -1 {
				rem := strings.TrimSpace(p[idx+len("limit"):])
				if strings.HasPrefix(rem, "=") {
					valPart := strings.TrimSpace(rem[1:])
					valTok := readToken(valPart)
					if valTok == "" {
						fieldsRem := strings.Fields(valPart)
						if len(fieldsRem) > 0 {
							valTok = fieldsRem[0]
						}
					}
					if v, err := strconv.Atoi(valTok); err == nil && v >= 0 {
						limit = v
						hasLimit = true
					}
				}
			}
		}

		return hasBy, limit, hasLimit
	}
	return false, 0, false
}

// EffectiveLimit returns the resolved limit (0 for unlimited, or N > 0)
// and whether remaining items beyond the limit should be grouped into an 'other' bucket.
// Rules:
// 1. If limit=0 was specified: limit is 0 (all), no 'other' bucket.
// 2. If limit=N (N > 0) was specified: limit is N, group remaining into 'other'.
// 3. If 'by' was used without limit=: default to limit=10, group remaining into 'other'.
// 4. If no 'by' and no limit=: limit is 0 (all), no 'other'.
func EffectiveLimit(query string) (limit int, useOther bool) {
	hasBy, lim, hasLimit := ExtractByAndLimit(query)
	if hasLimit {
		if lim == 0 {
			return 0, false
		}
		return lim, true
	}
	if hasBy {
		return 10, true
	}
	return 0, false
}

