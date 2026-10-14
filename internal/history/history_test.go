package history

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHistoryFilePath(t *testing.T) {
	configDir := filepath.Join(os.TempDir(), "splunk-tui-test")
	p1 := FilePath(configDir, "boot")
	expected1 := filepath.Join(configDir, "history_boot.txt")
	if p1 != expected1 {
		t.Errorf("expected %q, got %q", expected1, p1)
	}

	p2 := FilePath(configDir, "my/custom:instance*name?")
	expected2 := filepath.Join(configDir, "history_my_custom_instance_name_.txt")
	if p2 != expected2 {
		t.Errorf("expected sanitized %q, got %q", expected2, p2)
	}

	p3 := FilePath(configDir, "")
	expected3 := filepath.Join(configDir, "history_default.txt")
	if p3 != expected3 {
		t.Errorf("expected %q for empty name, got %q", expected3, p3)
	}
}

func TestHistoryAppendAndDeduplication(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "history-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	filePath := filepath.Join(tempDir, "history_test.txt")

	// 1. Initial load from non-existent file returns empty
	initial, err := Load(filePath)
	if err != nil {
		t.Fatalf("unexpected error on non-existent file: %v", err)
	}
	if len(initial) != 0 {
		t.Fatalf("expected empty initial slice, got %d", len(initial))
	}

	// 2. Append searches
	if err := Append(filePath, "index=_internal | head 10"); err != nil {
		t.Fatalf("append failed: %v", err)
	}
	if err := Append(filePath, "index=main error"); err != nil {
		t.Fatalf("append failed: %v", err)
	}
	if err := Append(filePath, "index=_internal | stats count by sourcetype"); err != nil {
		t.Fatalf("append failed: %v", err)
	}

	queries, err := Load(filePath)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if len(queries) != 3 {
		t.Fatalf("expected 3 queries, got %d", len(queries))
	}
	if queries[0] != "index=_internal | head 10" ||
		queries[1] != "index=main error" ||
		queries[2] != "index=_internal | stats count by sourcetype" {
		t.Errorf("unexpected queries: %v", queries)
	}

	// 3. Append duplicate: "index=main error"
	// Should remove older position and move to newest (end of file)
	if err := Append(filePath, "index=main error"); err != nil {
		t.Fatalf("append duplicate failed: %v", err)
	}

	updated, err := Load(filePath)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if len(updated) != 3 {
		t.Fatalf("expected still 3 queries after duplicate, got %d: %v", len(updated), updated)
	}
	if updated[0] != "index=_internal | head 10" ||
		updated[1] != "index=_internal | stats count by sourcetype" ||
		updated[2] != "index=main error" {
		t.Errorf("expected duplicate moved to end, got %v", updated)
	}

	// 4. Test LoadRecentFirst
	recentFirst, err := LoadRecentFirst(filePath)
	if err != nil {
		t.Fatalf("LoadRecentFirst failed: %v", err)
	}
	if len(recentFirst) != 3 {
		t.Fatalf("expected 3 queries in recentFirst, got %d", len(recentFirst))
	}
	if recentFirst[0] != "index=main error" {
		t.Errorf("expected most recent at index 0, got %q", recentFirst[0])
	}
	if recentFirst[2] != "index=_internal | head 10" {
		t.Errorf("expected oldest at index 2, got %q", recentFirst[2])
	}

	// 5. Test empty string does not append
	if err := Append(filePath, "   "); err != nil {
		t.Fatalf("append whitespace failed: %v", err)
	}
	afterEmpty, _ := Load(filePath)
	if len(afterEmpty) != 3 {
		t.Errorf("expected whitespace to be ignored, got %d entries", len(afterEmpty))
	}

	// 6. Test multi-line query collapses into single line
	multiLine := "index=_internal\n| stats count\nby host"
	if err := Append(filePath, multiLine); err != nil {
		t.Fatalf("append multiline failed: %v", err)
	}
	afterMulti, _ := Load(filePath)
	if len(afterMulti) != 4 {
		t.Fatalf("expected 4 entries, got %d", len(afterMulti))
	}
	if afterMulti[3] != "index=_internal | stats count by host" {
		t.Errorf("expected single-line collapsed query, got %q", afterMulti[3])
	}
}
