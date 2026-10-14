// Package config loads and saves splunk-tui settings and instance bookmarks.
//
// Passwords are never stored here; see package secrets.
package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	// DefaultPort is used when a bookmark URL does not specify a port.
	DefaultPort = "8089"
	// DefaultPageSize is the number of events/results shown per table page.
	DefaultPageSize = 50
	// DefaultTimeRange is the earliest time used for new searches.
	DefaultTimeRange = "-24h"
	// DefaultKeyringBackend lets the secrets package pick the best backend.
	DefaultKeyringBackend = "auto"

	appDirName = "splunk-tui"
)

// Bookmark describes a known Splunk instance.
type Bookmark struct {
	Name      string `yaml:"name"`
	URL       string `yaml:"url"`
	Username  string `yaml:"username"`
	VerifySSL bool   `yaml:"verify_ssl"`
}

// NormalizedURL returns the bookmark URL with defaults applied (see NormalizeURL).
func (b Bookmark) NormalizedURL() (string, error) {
	return NormalizeURL(b.URL)
}

// Validate checks that the bookmark is usable.
func (b Bookmark) Validate() error {
	if strings.TrimSpace(b.Name) == "" {
		return errors.New("name is required")
	}
	if b.Name != strings.TrimSpace(b.Name) {
		return errors.New("name must not start or end with spaces")
	}
	if _, err := NormalizeURL(b.URL); err != nil {
		return err
	}
	if strings.TrimSpace(b.Username) == "" {
		return errors.New("username is required")
	}
	return nil
}

// Config is the on-disk configuration file.
type Config struct {
	PageSize         int        `yaml:"page_size"`
	DefaultTimeRange string     `yaml:"default_time_range"`
	KeyringBackend   string     `yaml:"keyring_backend"`
	Bookmarks        []Bookmark `yaml:"bookmarks"`

	path string
}

// DefaultPath returns the default config file location,
// e.g. ~/.config/splunk-tui/config.yaml on Linux.
func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locating user config dir: %w", err)
	}
	return filepath.Join(dir, appDirName, "config.yaml"), nil
}

// Load reads the config file at path. A missing file yields a default config.
func Load(path string) (*Config, error) {
	cfg := &Config{}
	data, err := os.ReadFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		// first run: start empty
	case err != nil:
		return nil, fmt.Errorf("reading %s: %w", path, err)
	default:
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("parsing %s: %w", path, err)
		}
	}
	cfg.path = path
	cfg.applyDefaults()
	return cfg, nil
}

func (c *Config) applyDefaults() {
	if c.PageSize <= 0 {
		c.PageSize = DefaultPageSize
	}
	if c.PageSize > 1000 {
		c.PageSize = 1000
	}
	if strings.TrimSpace(c.DefaultTimeRange) == "" {
		c.DefaultTimeRange = DefaultTimeRange
	}
	if strings.TrimSpace(c.KeyringBackend) == "" {
		c.KeyringBackend = DefaultKeyringBackend
	}
}

// Path returns the file path the config was loaded from.
func (c *Config) Path() string { return c.path }

// Dir returns the directory containing the config file.
func (c *Config) Dir() string { return filepath.Dir(c.path) }

// Save atomically writes the config file with 0600 permissions.
func (c *Config) Save() error {
	if c.path == "" {
		return errors.New("config path not set")
	}
	if err := os.MkdirAll(filepath.Dir(c.path), 0o700); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("encoding config: %w", err)
	}
	tmp := c.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}
	if err := os.Rename(tmp, c.path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("replacing config: %w", err)
	}
	_ = os.Chmod(c.path, 0o600)
	return nil
}

func (c *Config) index(name string) int {
	for i, b := range c.Bookmarks {
		if b.Name == name {
			return i
		}
	}
	return -1
}

// Find returns the bookmark with the given name.
func (c *Config) Find(name string) (Bookmark, bool) {
	if i := c.index(name); i >= 0 {
		return c.Bookmarks[i], true
	}
	return Bookmark{}, false
}

// Names returns all bookmark names in order.
func (c *Config) Names() []string {
	names := make([]string, 0, len(c.Bookmarks))
	for _, b := range c.Bookmarks {
		names = append(names, b.Name)
	}
	return names
}

// Upsert adds a bookmark, or replaces the bookmark named oldName.
// Pass an empty oldName to add a new bookmark.
func (c *Config) Upsert(oldName string, b Bookmark) error {
	b.Name = strings.TrimSpace(b.Name)
	b.URL = strings.TrimSpace(b.URL)
	b.Username = strings.TrimSpace(b.Username)
	if err := b.Validate(); err != nil {
		return err
	}
	if i := c.index(b.Name); i >= 0 && b.Name != oldName {
		return fmt.Errorf("a bookmark named %q already exists", b.Name)
	}
	if oldName != "" {
		if i := c.index(oldName); i >= 0 {
			c.Bookmarks[i] = b
			return nil
		}
	}
	c.Bookmarks = append(c.Bookmarks, b)
	return nil
}

// Delete removes the named bookmark and reports whether it existed.
func (c *Config) Delete(name string) bool {
	i := c.index(name)
	if i < 0 {
		return false
	}
	c.Bookmarks = append(c.Bookmarks[:i], c.Bookmarks[i+1:]...)
	return true
}

// NormalizeURL turns user input into a base URL for the Splunk management API.
//
//   - missing scheme -> https
//   - missing port   -> 8089 (even for https)
//   - paths, queries, credentials and trailing slashes are dropped
func NormalizeURL(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", errors.New("url is required")
	}
	if !strings.Contains(s, "://") {
		s = "https://" + s
	}
	u, err := url.Parse(s)
	if err != nil {
		return "", fmt.Errorf("invalid url %q: %w", raw, err)
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", fmt.Errorf("invalid url %q: scheme must be http or https", raw)
	}
	host := u.Hostname()
	if host == "" {
		return "", fmt.Errorf("invalid url %q: missing host", raw)
	}
	port := u.Port()
	if port == "" {
		port = DefaultPort
	} else {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return "", fmt.Errorf("invalid url %q: bad port %q", raw, port)
		}
	}
	return scheme + "://" + net.JoinHostPort(host, port), nil
}
