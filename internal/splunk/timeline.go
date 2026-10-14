package splunk

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

type xmlBucket struct {
	T string `xml:"t,attr"` // epoch timestamp string e.g. "1672531199.000"
	D string `xml:"d,attr"` // duration in seconds e.g. "300.000"
	C int64  `xml:"c,attr"` // count
	A string `xml:"a,attr"` // available flag e.g. "1"
}

type xmlTimeline struct {
	XMLName xml.Name    `xml:"timeline"`
	Buckets []xmlBucket `xml:"bucket"`
}

// GetTimeline fetches the histogram of events over time from the job.
func (c *Client) GetTimeline(ctx context.Context, sid string) ([]TimelineBucket, error) {
	if sid == "" {
		return nil, fmt.Errorf("sid is required")
	}

	resp, err := c.request(ctx, http.MethodGet, "/services/search/jobs/"+url.PathEscape(sid)+"/timeline", nil, nil, "")
	if err != nil {
		return nil, fmt.Errorf("fetching timeline for %s: %w", sid, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("timeline request returned %d: %s", resp.StatusCode, string(b))
	}

	var parsed xmlTimeline
	if err := xml.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("decoding timeline xml: %w", err)
	}

	buckets := make([]TimelineBucket, 0, len(parsed.Buckets))
	for _, b := range parsed.Buckets {
		epochSec, err := strconv.ParseFloat(b.T, 64)
		if err != nil {
			continue
		}

		sec := int64(epochSec)
		nsec := int64((epochSec - float64(sec)) * 1e9)
		bucketTime := time.Unix(sec, nsec)

		durSec, _ := strconv.ParseFloat(b.D, 64)
		dur := time.Duration(math.Round(durSec * float64(time.Second)))

		buckets = append(buckets, TimelineBucket{
			Time:      bucketTime,
			Duration:  dur,
			Count:     b.C,
			Available: b.A != "0",
		})
	}

	return buckets, nil
}
