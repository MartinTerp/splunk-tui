package splunk

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSplunkClientFullFlow(t *testing.T) {
	loginCount := 0
	jobsCreated := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/services/auth/login":
			loginCount++
			_ = r.ParseForm()
			if r.Form.Get("username") != "admin" || r.Form.Get("password") != "secret" {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"messages":[{"type":"ERROR","text":"Login failed"}]}`))
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"sessionKey":"mock-session-key-12345"}`))

		case "/services/server/info":
			if r.Header.Get("Authorization") != "Splunk mock-session-key-12345" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"entry": [{
					"content": {
						"serverName": "test-splunk-node",
						"version": "9.2.1",
						"build": "abcdef",
						"os_name": "Linux"
					}
				}]
			}`))

		case "/services/search/jobs":
			jobsCreated++
			_ = r.ParseForm()
			search := r.Form.Get("search")
			if search != "search index=main" {
				t.Errorf("expected search prefixing 'search index=main', got %q", search)
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"sid":"1700000000.1"}`))

		case "/services/search/jobs/1700000000.1":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"entry": [{
					"content": {
						"dispatchState": "DONE",
						"doneProgress": 1.0,
						"eventCount": 2,
						"resultCount": 2,
						"isDone": true,
						"isFailed": false,
						"reportSearch": ""
					}
				}]
			}`))

		case "/services/search/jobs/1700000000.1/events":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"results": [
					{"_time": "2026-10-12T20:00:00Z", "_raw": "event 1", "host": "web01", "status": "200"},
					{"_time": "2026-10-12T20:01:00Z", "_raw": "event 2", "host": "web02", "status": "500"}
				]
			}`))

		case "/services/search/jobs/1700000000.1/summary":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"event_count": 2,
				"fields": {
					"host": {
						"count": 2,
						"distinct_count": 2,
						"modes": [
							{"value": "web01", "count": 1},
							{"value": "web02", "count": 1}
						]
					},
					"status": {
						"count": 2,
						"distinct_count": 2,
						"modes": [
							{"value": "200", "count": 1},
							{"value": "500", "count": 1}
						]
					}
				}
			}`))

		case "/services/search/jobs/1700000000.1/timeline":
			w.Header().Set("Content-Type", "text/xml")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`
				<timeline>
					<bucket t="1700000000.000" d="60.000" c="1" a="1" />
					<bucket t="1700000060.000" d="60.000" c="1" a="1" />
				</timeline>
			`))

		case "/services/search/jobs/1700000000.1/control":
			_ = r.ParseForm()
			if r.Form.Get("action") != "cancel" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusOK)

		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = fmt.Fprintf(w, "unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	ctx := context.Background()
	client, err := NewClient(ClientConfig{
		BaseURL:   server.URL,
		Username:  "admin",
		Password:  "secret",
		VerifySSL: false,
		Timeout:   5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	// 1. Login
	if err := client.Login(ctx); err != nil {
		t.Fatalf("Login failed: %v", err)
	}
	if client.SessionKey() != "mock-session-key-12345" {
		t.Errorf("got sessionKey %q", client.SessionKey())
	}

	// 2. Server info
	info, err := client.GetServerInfo(ctx)
	if err != nil {
		t.Fatalf("GetServerInfo failed: %v", err)
	}
	if info.ServerName != "test-splunk-node" || info.Version != "9.2.1" {
		t.Errorf("unexpected server info: %+v", info)
	}

	// 3. Create Job
	sid, err := client.CreateJob(ctx, CreateJobOptions{
		Search: "index=main",
	})
	if err != nil {
		t.Fatalf("CreateJob failed: %v", err)
	}
	if sid != "1700000000.1" {
		t.Errorf("unexpected SID: %s", sid)
	}

	// 4. Job status
	status, err := client.GetJobStatus(ctx, sid)
	if err != nil {
		t.Fatalf("GetJobStatus failed: %v", err)
	}
	if !status.IsDone || status.EventCount != 2 {
		t.Errorf("unexpected status: %+v", status)
	}

	// 5. Events
	events, err := client.GetEvents(ctx, sid, 0, 50)
	if err != nil {
		t.Fatalf("GetEvents failed: %v", err)
	}
	if len(events.Results) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events.Results))
	}
	if events.Results[0].Raw() != "event 1" || events.Results[0].FieldValue("host") != "web01" {
		t.Errorf("unexpected event 0: %+v", events.Results[0])
	}

	// 6. Summary
	summary, err := client.GetFieldSummary(ctx, sid, 10)
	if err != nil {
		t.Fatalf("GetFieldSummary failed: %v", err)
	}
	if summary.EventCount != 2 {
		t.Errorf("expected 2 events in summary, got %d", summary.EventCount)
	}
	hostField, ok := summary.Fields["host"]
	if !ok || hostField.Coverage != 1.0 {
		t.Errorf("expected 100%% coverage for host, got %+v", hostField)
	}

	// 7. Timeline
	timeline, err := client.GetTimeline(ctx, sid)
	if err != nil {
		t.Fatalf("GetTimeline failed: %v", err)
	}
	if len(timeline) != 2 {
		t.Fatalf("expected 2 buckets, got %d", len(timeline))
	}
	if timeline[0].Count != 1 || timeline[0].Duration != 60*time.Second {
		t.Errorf("unexpected bucket: %+v", timeline[0])
	}

	// 8. Cancel
	if err := client.CancelJob(ctx, sid); err != nil {
		t.Fatalf("CancelJob failed: %v", err)
	}
}

func TestGetResultsDynamicFieldsAndRawFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/services/search/jobs/job-no-fields/results":
			// Splunk standard output_mode=json (omits top-level fields)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"preview": false,
				"init_offset": 0,
				"messages": [],
				"results": [
					{"_time": "2026-10-13T10:00:00Z", "sourcetype": "splunkd", "count": "42"},
					{"_time": "2026-10-13T10:01:00Z", "sourcetype": "mongod", "count": "10"}
				]
			}`))

		case "/services/search/jobs/job-raw-array/results":
			// Raw JSON array
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[
				{"host": "web01", "status": "200"},
				{"host": "web02", "status": "500"}
			]`))

		case "/services/search/jobs/job-raw-array/events":
			// Raw JSON array for events
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[
				{"_raw": "syslog line 1", "host": "web01"},
				{"_raw": "syslog line 2", "host": "web02"}
			]`))

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	ctx := context.Background()
	client, err := NewClient(ClientConfig{
		BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	// 1. Results with omitted fields array
	res, err := client.GetResults(ctx, "job-no-fields", 0, 50)
	if err != nil {
		t.Fatalf("GetResults failed: %v", err)
	}
	if len(res.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(res.Results))
	}
	if len(res.Fields) == 0 {
		t.Fatalf("expected dynamic fields to be discovered, got empty")
	}
	if res.Fields[0] != "_time" {
		t.Errorf("expected _time as first field, got %s", res.Fields[0])
	}
	if res.Results[0]["sourcetype"] != "splunkd" || res.Results[0]["count"] != "42" {
		t.Errorf("unexpected row: %+v", res.Results[0])
	}

	// 2. Results with raw JSON array
	resRaw, err := client.GetResults(ctx, "job-raw-array", 0, 50)
	if err != nil {
		t.Fatalf("GetResults raw array failed: %v", err)
	}
	if len(resRaw.Results) != 2 {
		t.Fatalf("expected 2 results from raw array, got %d", len(resRaw.Results))
	}
	if resRaw.Results[0]["host"] != "web01" {
		t.Errorf("unexpected host in raw result: %+v", resRaw.Results[0])
	}

	// 3. Events with raw JSON array
	eventsRaw, err := client.GetEvents(ctx, "job-raw-array", 0, 50)
	if err != nil {
		t.Fatalf("GetEvents raw array failed: %v", err)
	}
	if len(eventsRaw.Results) != 2 {
		t.Fatalf("expected 2 events from raw array, got %d", len(eventsRaw.Results))
	}
	if eventsRaw.Results[0].Raw() != "syslog line 1" {
		t.Errorf("unexpected raw event: %+v", eventsRaw.Results[0])
	}
}

func TestSplunkJobErrorHandling(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/services/search/jobs":
			_ = r.ParseForm()
			search := r.Form.Get("search")
			if strings.Contains(search, "bad_json") {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"messages":[{"type":"FATAL","text":"Error in 'stats' command: The argument 'by' is invalid."}]}`))
				return
			}
			if strings.Contains(search, "bad_xml") {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`<response><messages><msg type="FATAL">Unknown search command 'foobar'.</msg></messages></response>`))
				return
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"sid":"failed-job-123"}`))

		case "/services/search/jobs/failed-job-123":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"entry": [{
					"content": {
						"dispatchState": "FAILED",
						"isDone": true,
						"isFailed": true,
						"messages": [
							{"type": "FATAL", "text": "Search job failed: Subsearch query resulted in an error."}
						]
					}
				}]
			}`))

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	ctx := context.Background()
	client, err := NewClient(ClientConfig{BaseURL: server.URL})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	// 1. CreateJob returning JSON error message
	_, errJSON := client.CreateJob(ctx, CreateJobOptions{Search: "search index=main | bad_json"})
	if errJSON == nil {
		t.Fatalf("expected error from bad_json search")
	}
	if !strings.Contains(errJSON.Error(), "Error in 'stats' command") {
		t.Errorf("expected error message to contain 'Error in 'stats' command', got: %v", errJSON)
	}

	// 2. CreateJob returning XML error message
	_, errXML := client.CreateJob(ctx, CreateJobOptions{Search: "search index=main | bad_xml"})
	if errXML == nil {
		t.Fatalf("expected error from bad_xml search")
	}
	if !strings.Contains(errXML.Error(), "Unknown search command 'foobar'") {
		t.Errorf("expected error message to contain 'Unknown search command', got: %v", errXML)
	}

	// 3. GetJobStatus returning isFailed with messages
	status, err := client.GetJobStatus(ctx, "failed-job-123")
	if err != nil {
		t.Fatalf("GetJobStatus failed: %v", err)
	}
	if !status.IsFailed {
		t.Errorf("expected status.IsFailed to be true")
	}
	errMsg := status.ErrorMessage()
	if !strings.Contains(errMsg, "Subsearch query resulted in an error") {
		t.Errorf("expected status.ErrorMessage() to contain 'Subsearch query resulted in an error', got: %q", errMsg)
	}
}
