package splunk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
)

// ConfigStanza represents a configuration section with key-value settings.
type ConfigStanza struct {
	Name       string            `json:"name"`
	Properties map[string]string `json:"content"`
}

var defaultConfigFiles = []string{
	"server",
	"inputs",
	"props",
	"transforms",
	"indexes",
	"savedsearches",
	"limits",
	"outputs",
	"authentication",
	"authorize",
	"distsearch",
	"alert_actions",
	"fields",
	"tags",
	"eventtypes",
	"web",
}

// GetConfigFiles lists available Splunk configuration files (.conf).
func (c *Client) GetConfigFiles(ctx context.Context) ([]string, error) {
	resp, err := c.request(ctx, http.MethodGet, "/services/properties", url.Values{"output_mode": {"json"}, "count": {"0"}}, nil, "")
	if err != nil {
		return defaultConfigFiles, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return defaultConfigFiles, nil
	}

	var res struct {
		Entry []struct {
			Name string `json:"name"`
		} `json:"entry"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil || len(res.Entry) == 0 {
		return defaultConfigFiles, nil
	}

	seen := make(map[string]bool)
	var files []string
	for _, e := range res.Entry {
		name := strings.TrimSpace(e.Name)
		name = strings.TrimSuffix(name, ".conf")
		if name != "" && !seen[name] {
			seen[name] = true
			files = append(files, name)
		}
	}

	// Add any missing standard files
	for _, f := range defaultConfigFiles {
		if !seen[f] {
			seen[f] = true
			files = append(files, f)
		}
	}

	sort.Strings(files)
	return files, nil
}

// GetConfigStanzas retrieves all stanzas and properties for a given conf file.
func (c *Client) GetConfigStanzas(ctx context.Context, confFile string) ([]ConfigStanza, error) {
	if confFile == "" {
		return nil, fmt.Errorf("confFile is required")
	}

	cleanName := strings.TrimSuffix(strings.TrimSpace(confFile), ".conf")
	path := "/services/configs/conf-" + url.PathEscape(cleanName)

	resp, err := c.request(ctx, http.MethodGet, path, url.Values{"output_mode": {"json"}, "count": {"0"}}, nil, "")
	if err != nil {
		return nil, fmt.Errorf("retrieving config %s: %w", confFile, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("retrieving config %s: HTTP %d", confFile, resp.StatusCode)
	}

	var res struct {
		Entry []struct {
			Name    string                 `json:"name"`
			Content map[string]interface{} `json:"content"`
		} `json:"entry"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, fmt.Errorf("decoding config %s: %w", confFile, err)
	}

	var stanzas []ConfigStanza
	for _, e := range res.Entry {
		props := make(map[string]string)
		for k, v := range e.Content {
			if v != nil {
				props[k] = fmt.Sprintf("%v", v)
			}
		}
		stanzas = append(stanzas, ConfigStanza{
			Name:       e.Name,
			Properties: props,
		})
	}

	sort.Slice(stanzas, func(i, j int) bool {
		return stanzas[i].Name < stanzas[j].Name
	})

	return stanzas, nil
}
