package splunk

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

type createJobResponse struct {
	SID      string `json:"sid"`
	Messages []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"messages"`
}

type xmlMessagesResponse struct {
	Messages []struct {
		Type string `xml:"type,attr"`
		Text string `xml:",chardata"`
	} `xml:"messages>msg"`
}

func parseErrorMessage(statusCode int, body []byte) string {
	// 1. Try JSON parsing
	var jsonResp struct {
		Messages []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(body, &jsonResp); err == nil && len(jsonResp.Messages) > 0 {
		for _, m := range jsonResp.Messages {
			u := strings.ToUpper(m.Type)
			if u == "FATAL" || u == "ERROR" {
				return m.Text
			}
		}
		return jsonResp.Messages[0].Text
	}

	// 2. Try XML parsing
	var xmlResp xmlMessagesResponse
	if err := xml.Unmarshal(body, &xmlResp); err == nil && len(xmlResp.Messages) > 0 {
		for _, m := range xmlResp.Messages {
			u := strings.ToUpper(m.Type)
			if u == "FATAL" || u == "ERROR" {
				return strings.TrimSpace(m.Text)
			}
		}
		return strings.TrimSpace(xmlResp.Messages[0].Text)
	}

	// 3. Fallback: extract substring between <msg...> and </msg> if present
	s := string(body)
	if idx := strings.Index(s, "<msg"); idx != -1 {
		if closeTag := strings.Index(s[idx:], ">"); closeTag != -1 {
			start := idx + closeTag + 1
			if endTag := strings.Index(s[start:], "</msg>"); endTag != -1 {
				msg := strings.TrimSpace(s[start : start+endTag])
				if msg != "" {
					return msg
				}
			}
		}
	}

	trimmed := strings.TrimSpace(s)
	if trimmed != "" {
		return fmt.Sprintf("HTTP %d: %s", statusCode, trimmed)
	}
	return fmt.Sprintf("HTTP %d: request failed", statusCode)
}

type jobStatusResponse struct {
	Messages []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"messages"`
	Entry []struct {
		Messages []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"messages"`
		Content struct {
			DispatchState string  `json:"dispatchState"`
			DoneProgress  float64 `json:"doneProgress"`
			EventCount    int64   `json:"eventCount"`
			ResultCount   int64   `json:"resultCount"`
			ScanCount     int64   `json:"scanCount"`
			RunDuration   float64 `json:"runDuration"`
			IsDone        bool    `json:"isDone"`
			IsFailed      bool    `json:"isFailed"`
			ReportSearch  string  `json:"reportSearch"`
			Messages      []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"messages"`
		} `json:"content"`
	} `json:"entry"`
}

// CreateJob dispatches a new search job in Splunk and returns its SID.
func (c *Client) CreateJob(ctx context.Context, opts CreateJobOptions) (string, error) {
	query := strings.TrimSpace(opts.Search)
	if query == "" {
		return "", fmt.Errorf("search query cannot be empty")
	}

	// In Splunk REST, if search doesn't start with "|" or "search ", prefix it with "search "
	if !strings.HasPrefix(query, "|") && !strings.HasPrefix(strings.ToLower(query), "search ") {
		query = "search " + query
	}

	form := url.Values{
		"search":      {query},
		"output_mode": {"json"},
		"exec_mode":   {"normal"},
	}

	if opts.EarliestTime != "" {
		form.Set("earliest_time", opts.EarliestTime)
	}
	if opts.LatestTime != "" {
		form.Set("latest_time", opts.LatestTime)
	}

	buckets := opts.StatusBuckets
	if buckets <= 0 {
		buckets = 300
	}
	form.Set("status_buckets", strconv.Itoa(buckets))

	if opts.Verbose {
		form.Set("adhoc_search_level", "verbose")
	} else {
		form.Set("adhoc_search_level", "smart")
	}

	if opts.MaxCount > 0 {
		form.Set("max_count", strconv.Itoa(opts.MaxCount))
	}

	resp, err := c.request(ctx, http.MethodPost, "/services/search/jobs", nil, strings.NewReader(form.Encode()), "application/x-www-form-urlencoded")
	if err != nil {
		return "", fmt.Errorf("creating search job: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading create job response: %w", err)
	}

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%s", parseErrorMessage(resp.StatusCode, bodyBytes))
	}

	var res createJobResponse
	if err := json.Unmarshal(bodyBytes, &res); err != nil {
		return "", fmt.Errorf("parsing create job response: %w", err)
	}

	if res.SID == "" {
		return "", fmt.Errorf("no SID returned by Splunk")
	}

	return res.SID, nil
}

// GetJobStatus polls the progress and counts for an ongoing search job.
func (c *Client) GetJobStatus(ctx context.Context, sid string) (*JobStatus, error) {
	if sid == "" {
		return nil, fmt.Errorf("sid is required")
	}

	resp, err := c.request(ctx, http.MethodGet, "/services/search/jobs/"+url.PathEscape(sid), url.Values{"output_mode": {"json"}}, nil, "")
	if err != nil {
		return nil, fmt.Errorf("polling job %s: %w", sid, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("%s", parseErrorMessage(resp.StatusCode, b))
	}

	var res jobStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, fmt.Errorf("decoding job status: %w", err)
	}

	if len(res.Entry) == 0 {
		return nil, fmt.Errorf("job %s not found in response", sid)
	}

	content := res.Entry[0].Content
	status := &JobStatus{
		SID:           sid,
		DispatchState: content.DispatchState,
		DoneProgress:  content.DoneProgress,
		EventCount:    content.EventCount,
		ResultCount:   content.ResultCount,
		ScanCount:     content.ScanCount,
		RunDuration:   content.RunDuration,
		IsDone:        content.IsDone,
		IsFailed:      content.IsFailed,
		ReportSearch:  content.ReportSearch,
	}

	for _, m := range content.Messages {
		status.Messages = append(status.Messages, Message{
			Type: m.Type,
			Text: m.Text,
		})
	}
	for _, m := range res.Entry[0].Messages {
		status.Messages = append(status.Messages, Message{
			Type: m.Type,
			Text: m.Text,
		})
	}
	for _, m := range res.Messages {
		status.Messages = append(status.Messages, Message{
			Type: m.Type,
			Text: m.Text,
		})
	}

	return status, nil
}

// CancelJob terminates the running search job.
func (c *Client) CancelJob(ctx context.Context, sid string) error {
	if sid == "" {
		return nil
	}

	form := url.Values{"action": {"cancel"}}
	resp, err := c.request(ctx, http.MethodPost, "/services/search/jobs/"+url.PathEscape(sid)+"/control", nil, strings.NewReader(form.Encode()), "application/x-www-form-urlencoded")
	if err != nil {
		return fmt.Errorf("cancelling job %s: %w", sid, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("cancel returned %d: %s", resp.StatusCode, string(b))
	}

	return nil
}
