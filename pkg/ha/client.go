package ha

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// HAEntry represents a location entry from the Health Assistant API.
type HAEntry struct {
	// RawTimestamp stores the original timestamp string from the API.
	RawTimestamp string   `json:"timestamp,omitempty"`
	Latitude     *float64 `json:"latitude,omitempty"`
	Longitude    *float64 `json:"longitude,omitempty"`
	Altitude     *float64 `json:"altitude,omitempty"`
	City         *string  `json:"city,omitempty"`
	State        *string  `json:"state,omitempty"`
	Country      *string  `json:"country,omitempty"`
	Timezone     *string  `json:"timezone,omitempty"`
}

// GetTimestamp parses the raw timestamp string using multiple known formats.
// Returns an error if no format matches or if the string is empty.
func (e *HAEntry) GetTimestamp() (time.Time, error) {
	if e.RawTimestamp == "" {
		return time.Time{}, fmt.Errorf("empty timestamp")
	}
	s := strings.TrimSpace(e.RawTimestamp)
	formats := []string{
		"2006:01:02 15:04:05",       // exiftool style
		"2006-01-02T15:04:05Z07:00", // ISO8601 with colon TZ
		"2006-01-02T15:04:05.999Z07:00",
		"2006-01-02 15:04:05.999 -07:00",
		"2006-01-02 15:04:05 -07:00",
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
	}
	for _, f := range formats {
		t, err := time.ParseInLocation(f, s, time.UTC)
		if err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unable to parse timestamp %q", s)
}

// HasLocation returns true if both latitude and longitude are present.
func (e *HAEntry) HasLocation() bool {
	return e.Latitude != nil && e.Longitude != nil
}

// Client is an HTTP client for the Health Assistant API.
type Client struct {
	baseURL    string
	apiToken   string
	httpClient *http.Client
}

// NewClient creates a new HA client with the given base URL and API token.
func NewClient(baseURL, apiToken string) *Client {
	return &Client{
		baseURL:    strings.TrimSuffix(baseURL, "/"),
		apiToken:   apiToken,
		httpClient: http.DefaultClient,
	}
}

// GetLocationHistory fetches the user's location history from the HA API.
// It returns a slice of HAEntry on success.
func (c *Client) GetLocationHistory(ctx context.Context) ([]HAEntry, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/api/location-history", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	// Add authentication header if token is provided.
	if c.apiToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiToken)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		var result struct {
			Locations []HAEntry `json:"locations"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return nil, fmt.Errorf("failed to decode response: %w", err)
		}
		return result.Locations, nil
	case http.StatusUnauthorized:
		return nil, fmt.Errorf("unauthorized: invalid or missing API token")
	case http.StatusTooManyRequests:
		return nil, fmt.Errorf("rate limited: too many requests")
	default:
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
}

// GetTimezone fetches the timezone for given latitude and longitude.
func (c *Client) GetTimezone(ctx context.Context, lat, lon float64) (string, error) {
	url := fmt.Sprintf("%s/api/timezone?lat=%f&lon=%f", c.baseURL, lat, lon)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	if c.apiToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiToken)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to get timezone: status %d", resp.StatusCode)
	}

	var payload struct {
		Timezone string `json:"timezone"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", fmt.Errorf("failed to decode timezone response: %w", err)
	}
	return payload.Timezone, nil
}
