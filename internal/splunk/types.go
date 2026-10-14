package splunk

import (
	"fmt"
	"strings"
	"time"
)

// ServerInfo holds basic Splunk instance metadata.
type ServerInfo struct {
	ServerName string `json:"serverName"`
	Version    string `json:"version"`
	Build      string `json:"build"`
	OSName     string `json:"os_name"`
}

// CreateJobOptions defines parameters for starting a search job.
type CreateJobOptions struct {
	Search        string
	EarliestTime  string
	LatestTime    string
	StatusBuckets int
	Verbose       bool
}

// JobStatus represents the state of an ongoing or completed search job.
type JobStatus struct {
	SID           string    `json:"sid"`
	DispatchState string    `json:"dispatchState"`
	DoneProgress  float64   `json:"doneProgress"`
	EventCount    int64     `json:"eventCount"`
	ResultCount   int64     `json:"resultCount"`
	ScanCount     int64     `json:"scanCount"`
	RunDuration   float64   `json:"runDuration"`
	IsDone        bool      `json:"isDone"`
	IsFailed      bool      `json:"isFailed"`
	ReportSearch  string    `json:"reportSearch"`
	Messages      []Message `json:"messages"`
}

// ErrorMessage returns any fatal or error message from the job status, or an empty string if none.
func (s *JobStatus) ErrorMessage() string {
	if s == nil {
		return ""
	}
	for _, m := range s.Messages {
		upper := strings.ToUpper(m.Type)
		if upper == "FATAL" || upper == "ERROR" {
			return m.Text
		}
	}
	if s.IsFailed {
		if len(s.Messages) > 0 {
			return s.Messages[0].Text
		}
		if s.DispatchState != "" && s.DispatchState != "FAILED" {
			return fmt.Sprintf("Search job failed: %s", s.DispatchState)
		}
		return "Search job failed"
	}
	return ""
}

// Message is an info/warning/error message from Splunk job dispatch.
type Message struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// Event represents a single Splunk event from /events.
// Values can be string or []string for multivalued fields.
type Event map[string]any

// Raw returns the _raw text of the event.
func (e Event) Raw() string {
	if v, ok := e["_raw"]; ok {
		return stringifyValue(v)
	}
	return ""
}

// Time returns the _time field as string.
func (e Event) Time() string {
	if v, ok := e["_time"]; ok {
		return stringifyValue(v)
	}
	return ""
}

// FieldValue returns string representation of a field value.
func (e Event) FieldValue(name string) string {
	v, ok := e[name]
	if !ok {
		return ""
	}
	return stringifyValue(v)
}

// EventsResponse wraps /events results and pagination metadata.
type EventsResponse struct {
	Results []Event `json:"results"`
	Total   int64   `json:"total"`
	Offset  int64   `json:"offset"`
	Count   int     `json:"count"`
}

// ResultsResponse wraps /results (transforming / stats results).
type ResultsResponse struct {
	Results []map[string]string `json:"results"`
	Fields  []string            `json:"fields"`
	Total   int64               `json:"total"`
	Offset  int64               `json:"offset"`
}

// TopValue is a frequent value for a field in the summary.
type TopValue struct {
	Value string `json:"value"`
	Count int64  `json:"count"`
}

// FieldSummary holds statistical distribution data for a field.
type FieldSummary struct {
	Name          string     `json:"name"`
	Count         int64      `json:"count"`
	DistinctCount int64      `json:"distinct_count"`
	Coverage      float64    `json:"coverage"` // count / totalEvents
	TopValues     []TopValue `json:"modes"`
}

// SummaryResponse contains summary metrics across all fields.
type SummaryResponse struct {
	EventCount int64                   `json:"event_count"`
	Fields     map[string]FieldSummary `json:"fields"`
}

// TimelineBucket is one time bucket from /timeline XML output.
type TimelineBucket struct {
	Time      time.Time
	Duration  time.Duration
	Count     int64
	Available bool
}
