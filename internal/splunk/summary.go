package splunk

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
)

type rawSummaryPayload struct {
	EventCount int64 `json:"event_count"`
	Fields     map[string]struct {
		Count         int64 `json:"count"`
		DistinctCount int64 `json:"distinct_count"`
		Modes         []struct {
			Value string `json:"value"`
			Count int64  `json:"count"`
		} `json:"modes"`
	} `json:"fields"`
}

// GetFieldSummary fetches statistics and top values for all extracted fields.
func (c *Client) GetFieldSummary(ctx context.Context, sid string, topCount int) (*SummaryResponse, error) {
	if sid == "" {
		return nil, fmt.Errorf("sid is required")
	}
	if topCount <= 0 {
		topCount = 10
	}

	query := url.Values{
		"output_mode": {"json"},
		"top_count":   {strconv.Itoa(topCount)},
	}

	resp, err := c.request(ctx, http.MethodGet, "/services/search/jobs/"+url.PathEscape(sid)+"/summary", query, nil, "")
	if err != nil {
		return nil, fmt.Errorf("fetching summary for %s: %w", sid, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("summary request returned %d: %s", resp.StatusCode, string(b))
	}

	var payload rawSummaryPayload
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decoding summary: %w", err)
	}

	result := &SummaryResponse{
		EventCount: payload.EventCount,
		Fields:     make(map[string]FieldSummary, len(payload.Fields)),
	}

	for fName, rawField := range payload.Fields {
		var coverage float64
		if payload.EventCount > 0 {
			coverage = float64(rawField.Count) / float64(payload.EventCount)
		}

		modes := make([]TopValue, 0, len(rawField.Modes))
		for _, m := range rawField.Modes {
			modes = append(modes, TopValue{
				Value: m.Value,
				Count: m.Count,
			})
		}

		result.Fields[fName] = FieldSummary{
			Name:          fName,
			Count:         rawField.Count,
			DistinctCount: rawField.DistinctCount,
			Coverage:      coverage,
			TopValues:     modes,
		}
	}

	return result, nil
}
