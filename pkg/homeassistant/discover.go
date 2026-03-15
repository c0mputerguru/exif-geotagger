package homeassistant

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// DeviceTracker represents a device_tracker entity from Home Assistant.
type DeviceTracker struct {
	EntityID     string `json:"entity_id"`
	FriendlyName string `json:"friendly_name,omitempty"`
	LastSeen     string `json:"last_seen"`
}

// StateResponse represents the JSON response from /api/states.
type StateResponse struct {
	EntityID    string                 `json:"entity_id"`
	State       string                 `json:"state"`
	Attributes  map[string]interface{} `json:"attributes"`
	LastChanged string                 `json:"last_changed"`
	LastUpdated string                 `json:"last_updated"`
}

// DiscoverDeviceTrackers queries Home Assistant /api/states and returns all
// device_tracker entities with their entity_id and optional friendly_name.
//
// Parameters:
//   - haURL: Home Assistant base URL (e.g., "http://homeassistant:8123")
//   - haToken: Long-lived access token for authentication
//
// Returns:
//   - []DeviceTracker: list of discovered device trackers
//   - error: network or decoding errors
func DiscoverDeviceTrackers(haURL, haToken string) ([]DeviceTracker, error) {
	if strings.HasSuffix(haURL, "/") {
		haURL = strings.TrimSuffix(haURL, "/")
	}
	url := fmt.Sprintf("%s/api/states", haURL)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+haToken)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to query Home Assistant: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	var states []StateResponse
	if err := json.NewDecoder(resp.Body).Decode(&states); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	var trackers []DeviceTracker
	for _, state := range states {
		if strings.HasPrefix(state.EntityID, "device_tracker.") {
			tracker := DeviceTracker{
				EntityID: state.EntityID,
				LastSeen: state.LastUpdated,
			}
			if friendly, ok := state.Attributes["friendly_name"]; ok {
				if name, ok := friendly.(string); ok {
					tracker.FriendlyName = name
				}
			}
			trackers = append(trackers, tracker)
		}
	}

	return trackers, nil
}
