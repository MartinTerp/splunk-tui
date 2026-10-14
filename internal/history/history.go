package history

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	// MaxHistoryEntries is the maximum number of searches retained per instance.
	MaxHistoryEntries = 1000
)

// FilePath returns the history file path for a given instance within configDir.
// If configDir is empty, it resolves to the default user configuration directory.
func FilePath(configDir, instanceName string) string {
	if configDir == "" || configDir == "." {
		if uDir, err := os.UserConfigDir(); err == nil {
			configDir = filepath.Join(uDir, "splunk-tui")
		}
	}
	safeName := sanitizeName(instanceName)
	if safeName == "" {
		safeName = "default"
	}
	return filepath.Join(configDir, fmt.Sprintf("history_%s.txt", safeName))
}

func sanitizeName(name string) string {
	return strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r == ':' || r == '*' || r == '?' || r == '"' || r == '<' || r == '>' || r == '|' || r == ' ' {
			return '_'
		}
		return r
	}, strings.TrimSpace(name))
}

// Load reads all search queries from filePath in chronological order (oldest first).
func Load(filePath string) ([]string, error) {
	if filePath == "" {
		return []string{}, nil
	}
	f, err := os.Open(filePath)
	if os.IsNotExist(err) {
		return []string{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("opening history file: %w", err)
	}
	defer f.Close()

	var queries []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			queries = append(queries, line)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading history file: %w", err)
	}
	return queries, nil
}

// LoadRecentFirst reads queries from filePath with the newest entry at index 0.
func LoadRecentFirst(filePath string) ([]string, error) {
	queries, err := Load(filePath)
	if err != nil {
		return nil, err
	}
	n := len(queries)
	recent := make([]string, n)
	for i, q := range queries {
		recent[n-1-i] = q
	}
	return recent, nil
}

// Append adds a query to the history file, removing any previous duplicate of that query.
// It collapses newlines into spaces so each query is a single line, and limits history to MaxHistoryEntries.
func Append(filePath, query string) error {
	clean := strings.TrimSpace(query)
	if clean == "" || filePath == "" {
		return nil
	}
	clean = strings.Join(strings.Fields(clean), " ")

	existing, err := Load(filePath)
	if err != nil && !os.IsNotExist(err) {
		existing = []string{}
	}

	// Remove any prior occurrence of this query (deduplicate)
	var deduplicated []string
	for _, item := range existing {
		if item != clean {
			deduplicated = append(deduplicated, item)
		}
	}

	// Append query as the newest entry
	deduplicated = append(deduplicated, clean)

	// Cap at MaxHistoryEntries
	if len(deduplicated) > MaxHistoryEntries {
		deduplicated = deduplicated[len(deduplicated)-MaxHistoryEntries:]
	}

	// Write atomically with 0600 permissions
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating history directory: %w", err)
	}

	tmpFile := filePath + ".tmp"
	f, err := os.OpenFile(tmpFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("creating temporary history file: %w", err)
	}

	w := bufio.NewWriter(f)
	for _, item := range deduplicated {
		if _, err := w.WriteString(item + "\n"); err != nil {
			_ = f.Close()
			_ = os.Remove(tmpFile)
			return fmt.Errorf("writing history item: %w", err)
		}
	}
	if err := w.Flush(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpFile)
		return fmt.Errorf("flushing history: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmpFile)
		return fmt.Errorf("closing temp history file: %w", err)
	}

	if err := os.Rename(tmpFile, filePath); err != nil {
		_ = os.Remove(tmpFile)
		return fmt.Errorf("renaming history file: %w", err)
	}

	return nil
}
