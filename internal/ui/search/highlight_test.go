package search

import (
	"strings"
	"testing"

	"splunk-tui/internal/ui/theme"
)

func TestDetectLogLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected LogLevel
	}{
		{"2026-10-14 08:00:00 [FATAL] System crashed", LogLevelFatal},
		{"2026-10-14 08:00:00 [CRITICAL] Core dump generated", LogLevelFatal},
		{"2026-10-14 08:00:00 ERROR: Connection refused to redis", LogLevelError},
		{"2026-10-14 08:00:00 [WARN] Disk space threshold exceeded: 85%", LogLevelWarn},
		{"2026-10-14 08:00:00 [INFO] Request completed in 12ms", LogLevelInfo},
		{"2026-10-14 08:00:00 [DEBUG] Entering function parseQuery", LogLevelDebug},
		{"Just a normal unstructured message without any level tag", LogLevelUnknown},
	}

	for _, tc := range tests {
		got := DetectLogLevel(tc.input)
		if got != tc.expected {
			t.Errorf("DetectLogLevel(%q) = %v; want %v", tc.input, got, tc.expected)
		}
	}
}

func TestHighlightLogText(t *testing.T) {
	th := theme.DefaultTheme()
	raw := "2026-10-14 08:00:00 [ERROR] user=admin ip=192.168.1.50 action=login status=failed"
	highlighted := HighlightLogText(raw, th)

	if !strings.Contains(highlighted, "user=") {
		t.Errorf("expected highlighted output to preserve keys, got:\n%s", highlighted)
	}
	if !strings.Contains(highlighted, "192.168.1.50") {
		t.Errorf("expected highlighted output to preserve IP, got:\n%s", highlighted)
	}
	if !strings.Contains(highlighted, "ERROR") {
		t.Errorf("expected highlighted output to preserve ERROR tag, got:\n%s", highlighted)
	}
}
