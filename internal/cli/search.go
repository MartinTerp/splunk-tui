package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"splunk-tui/internal/config"
	"splunk-tui/internal/secrets"
	"splunk-tui/internal/spl"
	"splunk-tui/internal/splunk"
)

// SearchOptions holds parameters for running a non-interactive search from the CLI.
type SearchOptions struct {
	Config       *config.Config
	Secrets      secrets.Store
	InstanceName string
	SPL          string
	JSON         bool
	Earliest     string
	Latest       string
	Count        int
	Token        string
	Out          io.Writer
	ErrOut       io.Writer
}

// Run executes an SPL search non-interactively and formats output to Out.
func Run(ctx context.Context, opts SearchOptions) error {
	if opts.Out == nil {
		opts.Out = os.Stdout
	}
	if opts.ErrOut == nil {
		opts.ErrOut = os.Stderr
	}

	query := strings.TrimSpace(opts.SPL)
	if query == "" {
		return fmt.Errorf("search query cannot be empty")
	}

	// 1. Resolve Splunk instance bookmark (instance is strictly required)
	if opts.Config == nil {
		return fmt.Errorf("configuration is required")
	}

	if strings.TrimSpace(opts.InstanceName) == "" {
		known := opts.Config.Names()
		if len(known) == 0 {
			return fmt.Errorf("--instance is required, but no instances are configured in config")
		}
		return fmt.Errorf("--instance is required. Available instances: %s", strings.Join(known, ", "))
	}

	found, ok := opts.Config.Find(opts.InstanceName)
	if !ok {
		known := opts.Config.Names()
		if len(known) == 0 {
			return fmt.Errorf("instance %q not found: no instances configured", opts.InstanceName)
		}
		return fmt.Errorf("instance %q not found. Available instances: %s", opts.InstanceName, strings.Join(known, ", "))
	}
	bm := found

	// 2. Retrieve credentials
	var secret string
	if opts.Secrets != nil {
		p, err := opts.Secrets.Get(bm.Name)
		if err == nil {
			secret = p
		}
	}

	normURL, err := bm.NormalizedURL()
	if err != nil {
		return fmt.Errorf("invalid URL for instance %q: %w", bm.Name, err)
	}

	clientCfg := splunk.ClientConfig{
		BaseURL:   normURL,
		Username:  bm.Username,
		VerifySSL: bm.VerifySSL,
		Timeout:   opts.Config.APITimeout(),
	}

	useToken := opts.Token != "" || bm.IsTokenAuth()
	if useToken {
		token := opts.Token
		if token == "" {
			token = bm.Token
		}
		if token == "" {
			token = secret
		}
		if token == "" {
			if envToken := os.Getenv("SPLUNK_TOKEN"); envToken != "" {
				token = envToken
			} else if envToken := os.Getenv("SPLUNK_TUI_TOKEN"); envToken != "" {
				token = envToken
			}
		}
		if token == "" {
			return fmt.Errorf("no token found in secrets store or environment for instance %q", bm.Name)
		}
		clientCfg.Token = token
		clientCfg.AuthType = "token"
	} else {
		password := secret
		if password == "" {
			if envPass := os.Getenv("SPLUNK_TUI_PASSWORD"); envPass != "" {
				password = envPass
			} else if envPass := os.Getenv("SPLUNK_PASSWORD"); envPass != "" {
				password = envPass
			}
		}
		if bm.Username != "" && password == "" {
			return fmt.Errorf("no password found in secrets store for instance %q", bm.Name)
		}
		clientCfg.Password = password
		clientCfg.AuthType = "password"
	}

	// 3. Connect and authenticate
	client, err := splunk.NewClient(clientCfg)
	if err != nil {
		return fmt.Errorf("initializing Splunk client: %w", err)
	}

	loginCtx, loginCancel := context.WithTimeout(ctx, opts.Config.APITimeout())
	defer loginCancel()
	if err := client.Login(loginCtx); err != nil {
		return fmt.Errorf("login failed for %s (%s): %w", bm.Name, bm.URL, err)
	}

	// 4. Determine time range
	earliest := opts.Earliest
	if earliest == "" && opts.Config.DefaultTimeRange != "" {
		earliest = opts.Config.DefaultTimeRange
	}
	if earliest == "" {
		earliest = "-24h"
	}
	latest := opts.Latest
	if qE, qL := spl.ExtractTimeBounds(query); qE != "" || qL != "" {
		if qE != "" && opts.Earliest == "" {
			earliest = qE
		}
		if qL != "" && opts.Latest == "" {
			latest = qL
		}
	}

	// 5. Dispatch search job
	maxCount := 0
	if opts.Count > 0 {
		maxCount = opts.Count
	}
	sid, err := client.CreateJob(ctx, splunk.CreateJobOptions{
		Search:        query,
		EarliestTime:  earliest,
		LatestTime:    latest,
		StatusBuckets: 0,
		Verbose:       true,
		MaxCount:      maxCount,
	})
	if err != nil {
		return fmt.Errorf("dispatching search job: %w", err)
	}

	// 6. Monitor cancellation via OS signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	// 7. Poll for completion
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-sigCh:
			_ = client.CancelJob(context.Background(), sid)
			return fmt.Errorf("search interrupted")
		case <-ctx.Done():
			_ = client.CancelJob(context.Background(), sid)
			return ctx.Err()
		case <-ticker.C:
			status, err := client.GetJobStatus(ctx, sid)
			if err != nil {
				return fmt.Errorf("checking search status: %w", err)
			}

			if status.IsDone {
				return fetchAndOutputResults(ctx, client, sid, opts)
			}

			if status.IsFailed {
				var msgs []string
				for _, m := range status.Messages {
					msgs = append(msgs, fmt.Sprintf("[%s] %s", m.Type, m.Text))
				}
				if len(msgs) > 0 {
					return fmt.Errorf("search job failed: %s", strings.Join(msgs, "; "))
				}
				return fmt.Errorf("search job failed")
			}
		}
	}
}

func fetchAndOutputResults(ctx context.Context, client *splunk.Client, sid string, opts SearchOptions) error {
	batchSize := 1000
	if opts.Count > 0 && opts.Count < batchSize {
		batchSize = opts.Count
	}

	// 1. Try events endpoint first with pagination
	offset := int64(0)
	totalEvents := 0
	var collectedEvents []splunk.Event
	hasEvents := false

	for {
		fetchCount := batchSize
		if opts.Count > 0 && (opts.Count-totalEvents) < batchSize {
			fetchCount = opts.Count - totalEvents
		}

		events, err := client.GetEvents(ctx, sid, offset, fetchCount)
		if err != nil || events == nil || len(events.Results) == 0 {
			break
		}

		hasEvents = true
		num := len(events.Results)

		if opts.JSON {
			collectedEvents = append(collectedEvents, events.Results...)
		} else {
			if err := PrintEventsText(opts.Out, events.Results); err != nil {
				return err
			}
		}

		totalEvents += num
		offset += int64(num)

		if (opts.Count > 0 && totalEvents >= opts.Count) || num < fetchCount {
			break
		}
	}

	if hasEvents {
		if opts.JSON {
			return PrintEventsJSON(opts.Out, collectedEvents)
		}
		return nil
	}

	// 2. Fallback to transforming/tabular results endpoint with pagination
	offset = 0
	totalResults := 0
	var collectedRows []map[string]string
	var headers []string
	hasRaw := false
	hasResults := false

	for {
		fetchCount := batchSize
		if opts.Count > 0 && (opts.Count-totalResults) < batchSize {
			fetchCount = opts.Count - totalResults
		}

		results, err := client.GetResults(ctx, sid, offset, fetchCount)
		if err != nil {
			if !hasResults {
				return fmt.Errorf("fetching search results: %w", err)
			}
			break
		}
		if results == nil || len(results.Results) == 0 {
			break
		}

		hasResults = true
		num := len(results.Results)

		if opts.JSON {
			collectedRows = append(collectedRows, results.Results...)
		} else {
			// On first batch, inspect columns and header
			if offset == 0 {
				headers = results.Fields
				if len(headers) == 0 && len(results.Results) > 0 {
					for k := range results.Results[0] {
						headers = append(headers, k)
					}
				}
				for _, row := range results.Results {
					if _, ok := row["_raw"]; ok {
						hasRaw = true
						break
					}
				}
				if !hasRaw && len(headers) > 0 {
					if _, err := fmt.Fprintln(opts.Out, strings.Join(headers, "\t")); err != nil {
						return err
					}
				}
			}

			if hasRaw {
				for _, row := range results.Results {
					ts := row["_time"]
					raw := strings.TrimRight(row["_raw"], "\r\n")
					var line string
					if ts != "" {
						line = fmt.Sprintf("%s  %s\n", ts, raw)
					} else {
						line = fmt.Sprintf("%s\n", raw)
					}
					if _, err := io.WriteString(opts.Out, line); err != nil {
						return err
					}
				}
			} else {
				for _, row := range results.Results {
					cols := make([]string, len(headers))
					for i, h := range headers {
						cols[i] = row[h]
					}
					if _, err := fmt.Fprintln(opts.Out, strings.Join(cols, "\t")); err != nil {
						return err
					}
				}
			}
		}

		totalResults += num
		offset += int64(num)

		if (opts.Count > 0 && totalResults >= opts.Count) || num < fetchCount {
			break
		}
	}

	if hasResults {
		if opts.JSON {
			return PrintResultsJSON(opts.Out, collectedRows)
		}
		return nil
	}

	// No results returned
	if opts.JSON {
		_, err := fmt.Fprintln(opts.Out, "[]")
		return err
	}
	return nil
}

// PrintEventsText prints events as "[_time]  _raw" or "_time  _raw".
func PrintEventsText(w io.Writer, events []splunk.Event) error {
	for _, ev := range events {
		ts := ev.Time()
		raw := strings.TrimRight(ev.Raw(), "\r\n")
		var line string
		if ts != "" {
			line = fmt.Sprintf("%s  %s\n", ts, raw)
		} else {
			line = fmt.Sprintf("%s\n", raw)
		}
		if _, err := io.WriteString(w, line); err != nil {
			return err
		}
	}
	return nil
}

// PrintEventsJSON outputs raw events formatted as pretty-printed JSON.
func PrintEventsJSON(w io.Writer, events []splunk.Event) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(events)
}

// PrintResultsJSON outputs transforming results formatted as pretty-printed JSON.
func PrintResultsJSON(w io.Writer, results []map[string]string) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(results)
}

// PrintResultsText outputs transforming or tabular results in plain text.
func PrintResultsText(w io.Writer, res *splunk.ResultsResponse) error {
	if res == nil || len(res.Results) == 0 {
		return nil
	}

	// Check if results contain _raw
	hasRaw := false
	for _, row := range res.Results {
		if _, ok := row["_raw"]; ok {
			hasRaw = true
			break
		}
	}

	if hasRaw {
		for _, row := range res.Results {
			ts := row["_time"]
			raw := strings.TrimRight(row["_raw"], "\r\n")
			var line string
			if ts != "" {
				line = fmt.Sprintf("%s  %s\n", ts, raw)
			} else {
				line = fmt.Sprintf("%s\n", raw)
			}
			if _, err := io.WriteString(w, line); err != nil {
				return err
			}
		}
		return nil
	}

	headers := res.Fields
	if len(headers) == 0 && len(res.Results) > 0 {
		for k := range res.Results[0] {
			headers = append(headers, k)
		}
	}

	if _, err := fmt.Fprintln(w, strings.Join(headers, "\t")); err != nil {
		return err
	}
	for _, row := range res.Results {
		cols := make([]string, len(headers))
		for i, h := range headers {
			cols[i] = row[h]
		}
		if _, err := fmt.Fprintln(w, strings.Join(cols, "\t")); err != nil {
			return err
		}
	}
	return nil
}
