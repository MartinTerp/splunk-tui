package splunk

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type loginResponse struct {
	SessionKey string `json:"sessionKey"`
	Messages   []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"messages"`
}

type serverInfoResponse struct {
	Entry []struct {
		Content struct {
			ServerName string `json:"serverName"`
			Version    string `json:"version"`
			Build      string `json:"build"`
			OSName     string `json:"os_name"`
		} `json:"content"`
	} `json:"entry"`
}

// Login authenticates with Splunk and caches the session key.
// When token-based authentication is configured, Login succeeds immediately.
func (c *Client) Login(ctx context.Context) error {
	c.mu.RLock()
	user := c.username
	pass := c.password
	token := c.sessionKey
	isToken := c.isTokenAuth
	c.mu.RUnlock()

	if isToken {
		if token == "" {
			return fmt.Errorf("token is required for token authentication")
		}
		return nil
	}

	if user == "" || pass == "" {
		return fmt.Errorf("username and password are required for login")
	}

	form := url.Values{
		"username":    {user},
		"password":    {pass},
		"output_mode": {"json"},
	}

	reqURL := c.baseURL + "/services/auth/login"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("login request failed: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading login response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp loginResponse
		_ = json.Unmarshal(bodyBytes, &errResp)
		if len(errResp.Messages) > 0 {
			return fmt.Errorf("login error (%d): %s", resp.StatusCode, errResp.Messages[0].Text)
		}
		return fmt.Errorf("login failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var loginResp loginResponse
	if err := json.Unmarshal(bodyBytes, &loginResp); err != nil {
		return fmt.Errorf("parsing login json: %w", err)
	}

	if loginResp.SessionKey == "" {
		return fmt.Errorf("no sessionKey in login response")
	}

	c.SetSessionKey(loginResp.SessionKey)
	return nil
}

// GetServerInfo retrieves instance details like version and server name.
func (c *Client) GetServerInfo(ctx context.Context) (*ServerInfo, error) {
	resp, err := c.request(ctx, http.MethodGet, "/services/server/info", url.Values{"output_mode": {"json"}}, nil, "")
	if err != nil {
		return nil, fmt.Errorf("requesting server info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("server info returned %d: %s", resp.StatusCode, string(b))
	}

	var infoResp serverInfoResponse
	if err := json.NewDecoder(resp.Body).Decode(&infoResp); err != nil {
		return nil, fmt.Errorf("decoding server info: %w", err)
	}

	if len(infoResp.Entry) == 0 {
		return &ServerInfo{ServerName: "Splunk", Version: "Unknown"}, nil
	}

	content := infoResp.Entry[0].Content
	return &ServerInfo{
		ServerName: content.ServerName,
		Version:    content.Version,
		Build:      content.Build,
		OSName:     content.OSName,
	}, nil
}
