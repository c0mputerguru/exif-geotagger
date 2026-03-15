package homeassistant

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client defines the interface for Home Assistant API interactions.
// This will be implemented by the HA Client Core (ge-d1z).
type Client interface {
	// Get performs an authenticated GET request and returns the response body.
	// Caller should close the body.
	Get(ctx context.Context, url string) (io.ReadCloser, error)

	// GetTimezone returns the Home Assistant timezone string.
	GetTimezone(ctx context.Context) (string, error)
}

// haClient is a concrete implementation of Client using HTTP.
type haClient struct {
	baseURL string
	token   string
	client  *http.Client
}

// NewClient creates a new Home Assistant client with the given base URL and token.
func NewClient(baseURL, token string) Client {
	return &haClient{
		baseURL: baseURL,
		token:   token,
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *haClient) Get(ctx context.Context, url string) (io.ReadCloser, error) {
	fullURL := c.baseURL + url
	req, err := http.NewRequestWithContext(ctx, "GET", fullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read error response: %w", err)
		}
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}
	return resp.Body, nil
}

func (c *haClient) GetTimezone(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/api/config", nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to get timezone: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	var cfg struct {
		Timezone string `json:"timezone"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&cfg); err != nil {
		return "", fmt.Errorf("failed to decode config: %w", err)
	}
	return cfg.Timezone, nil
}
