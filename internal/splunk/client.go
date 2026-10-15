package splunk

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// ClientConfig holds settings for communicating with a Splunk instance.
type ClientConfig struct {
	BaseURL   string
	Username  string
	Password  string
	Token     string
	AuthType  string // "password" or "token"
	VerifySSL bool
	Timeout   time.Duration
}

// Client interacts with the Splunk REST API.
type Client struct {
	baseURL     string
	username    string
	password    string
	isTokenAuth bool
	verifySSL   bool
	sessionKey  string
	httpClient  *http.Client
	mu          sync.RWMutex
}

// NewClient creates a new Splunk REST API client.
func NewClient(cfg ClientConfig) (*Client, error) {
	if cfg.BaseURL == "" {
		return nil, fmt.Errorf("baseURL is required")
	}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	tr := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: !cfg.VerifySSL,
		},
		ResponseHeaderTimeout: timeout,
	}

	httpClient := &http.Client{
		Transport: tr,
		Timeout:   timeout,
	}

	isToken := strings.EqualFold(strings.TrimSpace(cfg.AuthType), "token") || (cfg.Token != "" && cfg.Password == "")
	token := strings.TrimSpace(cfg.Token)
	var sessionKey string
	if isToken && token != "" {
		sessionKey = token
	}

	return &Client{
		baseURL:     strings.TrimRight(cfg.BaseURL, "/"),
		username:    cfg.Username,
		password:    cfg.Password,
		isTokenAuth: isToken,
		sessionKey:  sessionKey,
		verifySSL:   cfg.VerifySSL,
		httpClient:  httpClient,
	}, nil
}

// BaseURL returns the configured base URL.
func (c *Client) BaseURL() string {
	return c.baseURL
}

// Username returns the configured username.
func (c *Client) Username() string {
	return c.username
}

// SetPassword updates the in-memory password for re-authentication.
func (c *Client) SetPassword(pw string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.password = pw
	c.sessionKey = "" // Invalidate session
}

// SetSessionKey sets an explicit Splunk session key.
func (c *Client) SetSessionKey(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sessionKey = key
}

// SessionKey returns the active session key or token.
func (c *Client) SessionKey() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.sessionKey
}

// IsTokenAuth returns whether the client is configured with token authentication.
func (c *Client) IsTokenAuth() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.isTokenAuth
}

// SetToken updates the in-memory token and switches to token authentication.
func (c *Client) SetToken(t string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.isTokenAuth = true
	c.sessionKey = strings.TrimSpace(t)
}

func formatAuthHeader(token string, isToken bool) string {
	t := strings.TrimSpace(token)
	if strings.HasPrefix(t, "Bearer ") || strings.HasPrefix(t, "Splunk ") {
		return t
	}
	if isToken {
		return "Bearer " + t
	}
	return "Splunk " + t
}

func (c *Client) request(ctx context.Context, method, path string, query url.Values, body io.Reader, contentType string) (*http.Response, error) {
	reqURL := c.baseURL + path
	if len(query) > 0 {
		reqURL += "?" + query.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, method, reqURL, body)
	if err != nil {
		return nil, err
	}

	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	c.mu.RLock()
	token := c.sessionKey
	isToken := c.isTokenAuth
	c.mu.RUnlock()

	if token != "" {
		req.Header.Set("Authorization", formatAuthHeader(token, isToken))
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	// If 401 Unauthorized and we have password credentials (not token auth), try logging in once and retry
	if resp.StatusCode == http.StatusUnauthorized && !c.isTokenAuth && c.username != "" && c.password != "" {
		_ = resp.Body.Close()
		if err := c.Login(ctx); err != nil {
			return nil, fmt.Errorf("re-authenticating after 401: %w", err)
		}

		// Re-create request for retry
		c.mu.RLock()
		token = c.sessionKey
		isToken = c.isTokenAuth
		c.mu.RUnlock()

		reqRetry, err := http.NewRequestWithContext(ctx, method, reqURL, body)
		if err != nil {
			return nil, err
		}
		if contentType != "" {
			reqRetry.Header.Set("Content-Type", contentType)
		}
		if token != "" {
			reqRetry.Header.Set("Authorization", formatAuthHeader(token, isToken))
		}
		return c.httpClient.Do(reqRetry)
	}

	return resp, nil
}
