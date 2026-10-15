package search

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"splunk-tui/internal/splunk"
	"splunk-tui/internal/ui/theme"
)

func TestFormatRawJSON(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantJSON   bool
		mustContain []string
	}{
		{
			name:       "minified json object",
			input:      `{"status":200,"service":"auth","active":true,"tags":["prod","api"]}`,
			wantJSON:   true,
			mustContain: []string{
				"{\n",
				`  "status": 200,`,
				`  "service": "auth",`,
				`  "active": true,`,
				`  "tags": [`,
				`    "prod",`,
				`    "api"`,
			},
		},
		{
			name:       "json array",
			input:      `[{"id":1,"name":"first"},{"id":2,"name":"second"}]`,
			wantJSON:   true,
			mustContain: []string{
				"[\n",
				`  {`,
				`    "id": 1,`,
				`    "name": "first"`,
			},
		},
		{
			name:       "json with surrounding whitespace and newlines",
			input:      "  \r\n\t  {\"event\":\"logout\",\"user_id\":42}  \n ",
			wantJSON:   true,
			mustContain: []string{
				`  "event": "logout",`,
				`  "user_id": 42`,
			},
		},
		{
			name:       "prefixed log with json payload",
			input:      `2026-10-15 13:00:00.123 [INFO] host=srv01 {"action":"checkout","amount":99.95}`,
			wantJSON:   true,
			mustContain: []string{
				"2026-10-15 13:00:00.123 [INFO] host=srv01",
				"{\n",
				`  "action": "checkout",`,
				`  "amount": 99.95`,
			},
		},
		{
			name:       "plain text log without json",
			input:      "Oct 15 13:00:00 srv01 sshd[4012]: Accepted publickey for admin from 10.0.0.1",
			wantJSON:   false,
			mustContain: []string{"Accepted publickey"},
		},
		{
			name:       "plain text with braces that is not json",
			input:      "Failed query template: SELECT * FROM {table} WHERE {condition}",
			wantJSON:   false,
			mustContain: []string{"SELECT * FROM"},
		},
		{
			name:       "bare numbers or strings",
			input:      "12345",
			wantJSON:   false,
			mustContain: []string{"12345"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			formatted, isJSON := FormatRawJSON(tc.input)
			if isJSON != tc.wantJSON {
				t.Fatalf("FormatRawJSON() isJSON = %v, want %v", isJSON, tc.wantJSON)
			}
			for _, exp := range tc.mustContain {
				if !strings.Contains(formatted, exp) {
					t.Errorf("expected formatted output to contain %q, got:\n%s", exp, formatted)
				}
			}
		})
	}
}

func TestHighlightJSON(t *testing.T) {
	th := theme.DefaultTheme()
	jsonText := "{\n  \"status\": 200,\n  \"message\": \"success\",\n  \"active\": true,\n  \"extra\": null\n}"
	highlighted := HighlightJSON(jsonText, th)

	// Verify all parts are preserved in the highlighted text
	for _, expected := range []string{`"status"`, "200", `"message"`, `"success"`, "true", "null"} {
		if !strings.Contains(highlighted, expected) {
			t.Errorf("expected highlighted output to contain %q, got:\n%s", expected, highlighted)
		}
	}
}

func TestDetailModelJSONPrettyPrinting(t *testing.T) {
	th := theme.DefaultTheme()
	rawJSON := `{"timestamp":"2026-10-15T12:00:00Z","user":"alice","action":"transfer","amount":500,"success":true}`
	ev := splunk.Event{
		"_time": "2026-10-15 12:00:00",
		"host":  "web-server-01",
		"_raw":  rawJSON,
	}

	detail := NewDetail(th, ev, nil)
	detail.width = 90
	detail.height = 30

	view := detail.View()

	// 1. Check title and JSON badge
	if !strings.Contains(view, "Event Inspector") {
		t.Errorf("expected view to contain Event Inspector title")
	}
	if !strings.Contains(view, "JSON") {
		t.Errorf("expected view to contain JSON badge")
	}

	// 2. Check pretty-printed lines (multi-line format with 2 space indent)
	if !strings.Contains(view, `"user": "alice"`) {
		t.Errorf("expected pretty JSON user field, got:\n%s", view)
	}
	if !strings.Contains(view, `"amount": 500`) {
		t.Errorf("expected pretty JSON amount field, got:\n%s", view)
	}
	if !strings.Contains(view, `"success": true`) {
		t.Errorf("expected pretty JSON success field, got:\n%s", view)
	}

	// 3. Test scrolling down
	detail, _, _ = detail.Update(tea.KeyMsg{Type: tea.KeyDown})
	if detail.scroll != 1 {
		t.Errorf("expected scroll = 1 after KeyDown, got %d", detail.scroll)
	}

	// Test page down
	detail, _, _ = detail.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	if detail.scroll != 11 {
		t.Errorf("expected scroll = 11 after PgDown, got %d", detail.scroll)
	}

	// Test home
	detail, _, _ = detail.Update(tea.KeyMsg{Type: tea.KeyHome})
	if detail.scroll != 0 {
		t.Errorf("expected scroll = 0 after Home, got %d", detail.scroll)
	}

	// Test close with esc
	_, _, closeRequested := detail.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if !closeRequested {
		t.Errorf("expected closeRequested = true on Esc")
	}
}

func TestDetailModelNonJSON(t *testing.T) {
	th := theme.DefaultTheme()
	rawText := "2026-10-15 12:00:00 [ERROR] Connection refused to database 10.0.1.2:5432"
	ev := splunk.Event{
		"_time": "2026-10-15 12:00:00",
		"host":  "db-proxy",
		"_raw":  rawText,
	}

	detail := NewDetail(th, ev, nil)
	detail.width = 90
	detail.height = 30

	view := detail.View()

	// Should not have the JSON badge
	if strings.Contains(view, "JSON") {
		t.Errorf("non-JSON event should not have JSON badge")
	}
	// Should have raw text
	if !strings.Contains(view, "Connection refused to database") {
		t.Errorf("expected raw text in view")
	}
}

func TestDetailModelTransformingRowWithRawJSON(t *testing.T) {
	th := theme.DefaultTheme()
	row := map[string]string{
		"host": "api-gateway",
		"_raw": `{"route":"/v1/auth","code":401,"error":"unauthorized"}`,
	}

	detail := NewDetail(th, nil, row)
	detail.width = 90
	detail.height = 30

	view := detail.View()

	if !strings.Contains(view, "JSON") {
		t.Errorf("expected JSON badge for transforming row with _raw JSON")
	}
	if !strings.Contains(view, `"route": "/v1/auth"`) {
		t.Errorf("expected pretty JSON in transforming detail view, got:\n%s", view)
	}
}
