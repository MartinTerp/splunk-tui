package splunk

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

type rawEventsPayload struct {
	Results []Event `json:"results"`
}

type rawResultsPayload struct {
	// Rows are kept raw so the original JSON key order can be recovered
	// (Go maps do not preserve order).
	Results []json.RawMessage `json:"results"`
	// Splunk emits fields either as [{"name": "x"}, ...] or ["x", ...]
	// depending on version/endpoint, so decode them lazily.
	Fields []json.RawMessage `json:"fields"`
}

// GetEvents retrieves paginated events from a search job.
func (c *Client) GetEvents(ctx context.Context, sid string, offset int64, count int) (*EventsResponse, error) {
	body, err := c.fetchJobData(ctx, sid, "events", offset, count)
	if err != nil {
		return nil, err
	}

	res := &EventsResponse{Offset: offset, Count: count}
	if len(body) == 0 {
		return res, nil
	}

	switch body[0] {
	case '{':
		var payload rawEventsPayload
		if err := json.Unmarshal(body, &payload); err != nil {
			return nil, fmt.Errorf("decoding events: %w", err)
		}
		res.Results = payload.Results
	case '[':
		if err := json.Unmarshal(body, &res.Results); err != nil {
			return nil, fmt.Errorf("decoding events: %w", err)
		}
	default:
		return nil, fmt.Errorf("decoding events: unexpected response: %s", snippet(body))
	}

	return res, nil
}

// GetResults retrieves final tabular records for a search job (stats, chart, table, ...).
// Splunk only serves this endpoint once the job is done.
func (c *Client) GetResults(ctx context.Context, sid string, offset int64, count int) (*ResultsResponse, error) {
	return c.getResults(ctx, sid, "results", offset, count)
}

// GetResultsPreview retrieves partial tabular records while a search job is still running.
func (c *Client) GetResultsPreview(ctx context.Context, sid string, offset int64, count int) (*ResultsResponse, error) {
	return c.getResults(ctx, sid, "results_preview", offset, count)
}

func (c *Client) getResults(ctx context.Context, sid, endpoint string, offset int64, count int) (*ResultsResponse, error) {
	body, err := c.fetchJobData(ctx, sid, endpoint, offset, count)
	if err != nil {
		return nil, err
	}

	res := &ResultsResponse{Offset: offset}
	if len(body) == 0 {
		return res, nil
	}

	var rawRows []json.RawMessage
	var fieldNames []string

	switch body[0] {
	case '{':
		var payload rawResultsPayload
		if err := json.Unmarshal(body, &payload); err != nil {
			return nil, fmt.Errorf("decoding %s: %w", endpoint, err)
		}
		rawRows = payload.Results
		fieldNames = parseFieldNames(payload.Fields)
	case '[':
		if err := json.Unmarshal(body, &rawRows); err != nil {
			return nil, fmt.Errorf("decoding %s: %w", endpoint, err)
		}
	default:
		return nil, fmt.Errorf("decoding %s: unexpected response: %s", endpoint, snippet(body))
	}

	rows := make([]map[string]string, 0, len(rawRows))
	var orderedKeys []string
	seen := make(map[string]bool)

	for _, raw := range rawRows {
		var obj map[string]any
		if err := json.Unmarshal(raw, &obj); err != nil {
			return nil, fmt.Errorf("decoding %s row: %w", endpoint, err)
		}
		rows = append(rows, normalizeRow(obj))

		// Only needed when Splunk did not send an explicit field list.
		if len(fieldNames) == 0 {
			for _, k := range objectKeys(raw) {
				if !seen[k] {
					seen[k] = true
					orderedKeys = append(orderedKeys, k)
				}
			}
		}
	}

	if len(fieldNames) == 0 {
		fieldNames = orderedKeys
	}

	res.Results = rows
	res.Fields = displayFields(fieldNames)
	return res, nil
}

// fetchJobData performs a GET against a job sub-endpoint and returns the trimmed body.
// A nil body with nil error means Splunk had no content yet (HTTP 204).
func (c *Client) fetchJobData(ctx context.Context, sid, endpoint string, offset int64, count int) ([]byte, error) {
	if sid == "" {
		return nil, fmt.Errorf("sid is required")
	}

	query := url.Values{
		"output_mode": {"json"},
		"offset":      {strconv.FormatInt(offset, 10)},
		"count":       {strconv.Itoa(count)},
	}

	resp, err := c.request(ctx, http.MethodGet, "/services/search/jobs/"+url.PathEscape(sid)+"/"+endpoint, query, nil, "")
	if err != nil {
		return nil, fmt.Errorf("fetching %s for %s: %w", endpoint, sid, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading %s response: %w", endpoint, err)
	}

	switch resp.StatusCode {
	case http.StatusOK:
		return bytes.TrimSpace(body), nil
	case http.StatusNoContent:
		// Splunk answers 204 while a running job has nothing to serve on this endpoint yet.
		return nil, nil
	default:
		return nil, fmt.Errorf("%s request returned %d: %s", endpoint, resp.StatusCode, snippet(body))
	}
}

// parseFieldNames accepts both [{"name":"x"}] and ["x"] encodings.
func parseFieldNames(raw []json.RawMessage) []string {
	var names []string
	for _, r := range raw {
		var s string
		if err := json.Unmarshal(r, &s); err == nil {
			if s != "" {
				names = append(names, s)
			}
			continue
		}
		var obj struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(r, &obj); err == nil && obj.Name != "" {
			names = append(names, obj.Name)
		}
	}
	return names
}

// displayFields drops Splunk internal fields (_bkt, _cd, _si, _span, ...) while
// keeping _time and _raw, preserving the original order. If every field is
// internal, all of them are kept so the table is never empty.
func displayFields(fields []string) []string {
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		if !strings.HasPrefix(f, "_") || f == "_time" || f == "_raw" {
			out = append(out, f)
		}
	}
	if len(out) == 0 {
		return fields
	}
	return out
}

// objectKeys returns the top-level keys of a JSON object in document order.
func objectKeys(raw []byte) []string {
	dec := json.NewDecoder(bytes.NewReader(raw))
	tok, err := dec.Token()
	if err != nil || tok != json.Delim('{') {
		return nil
	}

	var keys []string
	for dec.More() {
		t, err := dec.Token()
		if err != nil {
			return keys
		}
		k, ok := t.(string)
		if !ok {
			return keys
		}
		keys = append(keys, k)

		var skip json.RawMessage
		if err := dec.Decode(&skip); err != nil {
			return keys
		}
	}
	return keys
}

func normalizeRow(obj map[string]any) map[string]string {
	row := make(map[string]string, len(obj))
	for k, v := range obj {
		if v == nil {
			continue
		}
		row[k] = stringifyValue(v)
	}
	return row
}

// stringifyValue renders a decoded JSON value for display; multivalue fields are comma-joined.
func stringifyValue(v any) string {
	switch val := v.(type) {
	case nil:
		return ""
	case string:
		return val
	case float64:
		return strconv.FormatFloat(val, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(val)
	case []any:
		parts := make([]string, 0, len(val))
		for _, item := range val {
			parts = append(parts, stringifyValue(item))
		}
		return strings.Join(parts, ", ")
	default:
		return fmt.Sprint(val)
	}
}

// snippet keeps error messages short enough to fit in the status bar.
func snippet(b []byte) string {
	const max = 200
	s := strings.TrimSpace(string(b))
	if len(s) > max {
		return s[:max] + "…"
	}
	return s
}
