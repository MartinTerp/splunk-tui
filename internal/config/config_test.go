package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestNormalizeURL(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"https://splunk.corp:8089", "https://splunk.corp:8089", false},
		{"https://splunk.corp:8090", "https://splunk.corp:8090", false},
		{"https://splunk.corp", "https://splunk.corp:8089", false},
		{"https://splunk.corp/", "https://splunk.corp:8089", false},
		{"https://splunk.corp:8089/services/", "https://splunk.corp:8089", false},
		{"splunk.corp", "https://splunk.corp:8089", false},
		{"splunk.corp:9000", "https://splunk.corp:9000", false},
		{"http://10.0.0.5/", "http://10.0.0.5:8089", false},
		{"HTTPS://Splunk.Corp", "https://Splunk.Corp:8089", false},
		{"  https://splunk.corp  ", "https://splunk.corp:8089", false},
		{"https://[::1]", "https://[::1]:8089", false},
		{"https://[::1]:9089", "https://[::1]:9089", false},
		{"https://user:pw@splunk.corp", "https://splunk.corp:8089", false},
		{"https://splunk.corp:", "https://splunk.corp:8089", false},
		{"", "", true},
		{"ftp://splunk.corp", "", true},
		{"https://", "", true},
		{"https://splunk.corp:99999", "", true},
		{"https://splunk.corp:abc", "", true},
	}
	for _, tt := range tests {
		got, err := NormalizeURL(tt.in)
		if tt.wantErr {
			if err == nil {
				t.Errorf("NormalizeURL(%q) = %q, want error", tt.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("NormalizeURL(%q) unexpected error: %v", tt.in, err)
			continue
		}
		if got != tt.want {
			t.Errorf("NormalizeURL(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestLoadMissingGivesDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nope", "config.yaml")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PageSize != DefaultPageSize {
		t.Errorf("PageSize = %d, want %d", cfg.PageSize, DefaultPageSize)
	}
	if cfg.DefaultTimeRange != DefaultTimeRange {
		t.Errorf("DefaultTimeRange = %q", cfg.DefaultTimeRange)
	}
	if cfg.KeyringBackend != DefaultKeyringBackend {
		t.Errorf("KeyringBackend = %q", cfg.KeyringBackend)
	}
	if cfg.Path() != path {
		t.Errorf("Path = %q", cfg.Path())
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "config.yaml")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg.PageSize = 25
	if err := cfg.Upsert("", Bookmark{Name: "prod", URL: "splunk.corp", Username: "admin", VerifySSL: true}); err != nil {
		t.Fatal(err)
	}
	if err := cfg.Upsert("", Bookmark{Name: "lab", URL: "https://10.0.0.5:8089", Username: "admin"}); err != nil {
		t.Fatal(err)
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		st, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if st.Mode().Perm() != 0o600 {
			t.Errorf("mode = %v, want 0600", st.Mode().Perm())
		}
	}

	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.PageSize != 25 {
		t.Errorf("PageSize = %d", got.PageSize)
	}
	if len(got.Bookmarks) != 2 {
		t.Fatalf("bookmarks = %d", len(got.Bookmarks))
	}
	b, ok := got.Find("prod")
	if !ok || b.URL != "splunk.corp" || !b.VerifySSL || b.Username != "admin" {
		t.Errorf("prod bookmark = %+v", b)
	}
}

func TestUpsertAndDelete(t *testing.T) {
	cfg := &Config{}
	if err := cfg.Upsert("", Bookmark{Name: "a", URL: "h", Username: "u"}); err != nil {
		t.Fatal(err)
	}
	if err := cfg.Upsert("", Bookmark{Name: "a", URL: "h", Username: "u"}); err == nil {
		t.Error("duplicate add should fail")
	}
	if err := cfg.Upsert("", Bookmark{Name: "b", URL: "h", Username: "u"}); err != nil {
		t.Fatal(err)
	}
	// rename a -> b collides
	if err := cfg.Upsert("a", Bookmark{Name: "b", URL: "h", Username: "u"}); err == nil {
		t.Error("rename onto existing name should fail")
	}
	// rename a -> c
	if err := cfg.Upsert("a", Bookmark{Name: "c", URL: "h2", Username: "u"}); err != nil {
		t.Fatal(err)
	}
	if _, ok := cfg.Find("a"); ok {
		t.Error("a should be gone after rename")
	}
	if b, ok := cfg.Find("c"); !ok || b.URL != "h2" {
		t.Errorf("c = %+v", b)
	}
	if cfg.Bookmarks[0].Name != "c" {
		t.Error("rename should keep position")
	}
	if !cfg.Delete("c") || cfg.Delete("c") {
		t.Error("delete semantics wrong")
	}
	if err := cfg.Upsert("", Bookmark{Name: "x", URL: "", Username: "u"}); err == nil {
		t.Error("empty url should fail")
	}
	if err := cfg.Upsert("", Bookmark{Name: "x", URL: "h", Username: ""}); err == nil {
		t.Error("empty username should fail")
	}
}
