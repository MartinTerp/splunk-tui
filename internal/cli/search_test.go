package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"splunk-tui/internal/config"
	"splunk-tui/internal/splunk"
)

func TestPrintEventsText(t *testing.T) {
	events := []splunk.Event{
		{
			"_time": "2026-10-13T12:00:00Z",
			"_raw":  "127.0.0.1 GET /index.html 200\n",
		},
		{
			"_time": "2026-10-13T12:01:00Z",
			"_raw":  "127.0.0.1 POST /login 401\r\n",
		},
		{
			"_raw": "System rebooted",
		},
	}

	var buf bytes.Buffer
	if err := PrintEventsText(&buf, events); err != nil {
		t.Fatalf("PrintEventsText failed: %v", err)
	}

	expected := "2026-10-13T12:00:00Z  127.0.0.1 GET /index.html 200\n" +
		"2026-10-13T12:01:00Z  127.0.0.1 POST /login 401\n" +
		"System rebooted\n"

	if buf.String() != expected {
		t.Errorf("got:\n%q\nwant:\n%q", buf.String(), expected)
	}
}

func TestPrintEventsJSON(t *testing.T) {
	events := []splunk.Event{
		{
			"_time": "2026-10-13T12:00:00Z",
			"_raw":  "test event 1",
			"host":  "server1",
		},
	}

	var buf bytes.Buffer
	if err := PrintEventsJSON(&buf, events); err != nil {
		t.Fatalf("PrintEventsJSON failed: %v", err)
	}

	var parsed []map[string]any
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("Failed to parse JSON output: %v", err)
	}

	if len(parsed) != 1 {
		t.Fatalf("expected 1 event, got %d", len(parsed))
	}
	if parsed[0]["_raw"] != "test event 1" || parsed[0]["host"] != "server1" {
		t.Errorf("unexpected parsed event: %+v", parsed[0])
	}
}

func TestPrintResultsText(t *testing.T) {
	t.Run("Tabular Stats", func(t *testing.T) {
		res := &splunk.ResultsResponse{
			Fields: []string{"sourcetype", "count"},
			Results: []map[string]string{
				{"sourcetype": "access_combined", "count": "150"},
				{"sourcetype": "syslog", "count": "45"},
			},
		}

		var buf bytes.Buffer
		if err := PrintResultsText(&buf, res); err != nil {
			t.Fatalf("PrintResultsText failed: %v", err)
		}

		expected := "sourcetype\tcount\naccess_combined\t150\nsyslog\t45\n"
		if buf.String() != expected {
			t.Errorf("got:\n%q\nwant:\n%q", buf.String(), expected)
		}
	})

	t.Run("Results With Raw", func(t *testing.T) {
		res := &splunk.ResultsResponse{
			Fields: []string{"_time", "_raw"},
			Results: []map[string]string{
				{"_time": "2026-10-13T14:00:00Z", "_raw": "event with raw"},
			},
		}

		var buf bytes.Buffer
		if err := PrintResultsText(&buf, res); err != nil {
			t.Fatalf("PrintResultsText failed: %v", err)
		}

		expected := "2026-10-13T14:00:00Z  event with raw\n"
		if buf.String() != expected {
			t.Errorf("got:\n%q\nwant:\n%q", buf.String(), expected)
		}
	})
}

func TestPrintResultsJSON(t *testing.T) {
	rows := []map[string]string{
		{"host": "prod-1", "cpu": "85%"},
		{"host": "prod-2", "cpu": "22%"},
	}

	var buf bytes.Buffer
	if err := PrintResultsJSON(&buf, rows); err != nil {
		t.Fatalf("PrintResultsJSON failed: %v", err)
	}

	var parsed []map[string]string
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	if len(parsed) != 2 || parsed[0]["host"] != "prod-1" {
		t.Errorf("unexpected json result: %+v", parsed)
	}
}

func TestRunValidation(t *testing.T) {
	t.Run("Empty Query", func(t *testing.T) {
		err := Run(context.Background(), SearchOptions{
			SPL: "   ",
		})
		if err == nil || !strings.Contains(err.Error(), "empty") {
			t.Errorf("expected empty query error, got: %v", err)
		}
	})

	t.Run("Instance Required", func(t *testing.T) {
		cfg := &config.Config{
			Bookmarks: []config.Bookmark{
				{Name: "cluster-a", URL: "https://a:8089", Username: "admin"},
			},
		}
		err := Run(context.Background(), SearchOptions{
			Config: cfg,
			SPL:    "index=_internal",
		})
		if err == nil || !strings.Contains(err.Error(), "--instance is required") {
			t.Errorf("expected --instance is required error, got: %v", err)
		}
	})

	t.Run("Instance Not Found", func(t *testing.T) {
		cfg := &config.Config{
			Bookmarks: []config.Bookmark{
				{Name: "cluster-a", URL: "https://a:8089", Username: "admin"},
			},
		}
		err := Run(context.Background(), SearchOptions{
			Config:       cfg,
			InstanceName: "non-existent",
			SPL:          "index=_internal",
		})
		if err == nil || !strings.Contains(err.Error(), "not found") {
			t.Errorf("expected not found error, got: %v", err)
		}
	})

	t.Run("Token Auth Missing Token", func(t *testing.T) {
		cfg := &config.Config{
			Bookmarks: []config.Bookmark{
				{Name: "token-cluster", URL: "https://a:8089", AuthType: "token"},
			},
		}
		err := Run(context.Background(), SearchOptions{
			Config:       cfg,
			InstanceName: "token-cluster",
			SPL:          "index=_internal",
		})
		if err == nil || !strings.Contains(err.Error(), "no token found") {
			t.Errorf("expected 'no token found' error, got: %v", err)
		}
	})
}
