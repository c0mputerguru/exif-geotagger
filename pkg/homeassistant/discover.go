package homeassistant

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
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
func DiscoverDeviceTrackers(ctx context.Context, haURL, haToken string) ([]DeviceTracker, error) {
	if strings.HasSuffix(haURL, "/") {
		haURL = strings.TrimSuffix(haURL, "/")
	}
	url := fmt.Sprintf("%s/api/states", haURL)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
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

// SelectDeviceTrackersInteractive displays a list of device trackers and prompts
// the user to select which ones to include via comma-separated numbers.
// It returns the selected entity IDs.
func SelectDeviceTrackersInteractive(trackers []DeviceTracker) ([]string, error) {
	if len(trackers) == 0 {
		return nil, fmt.Errorf("no device trackers provided")
	}
	fmt.Println("Discovered device_tracker entities:")
	for i, t := range trackers {
		name := t.FriendlyName
		if name == "" {
			name = t.EntityID
		}
		lastSeen := t.LastSeen
		if lastSeen != "" {
			lastSeen = " (last seen: " + lastSeen + ")"
		}
		fmt.Printf("%d. %s%s (%s)\n", i+1, name, lastSeen, t.EntityID)
	}
	fmt.Print("Enter numbers (comma-separated) to include: ")
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		return nil, fmt.Errorf("failed to read selection")
	}
	input := scanner.Text()
	if strings.TrimSpace(input) == "" {
		return nil, fmt.Errorf("no devices selected")
	}
	idxStrs := strings.Split(input, ",")
	selected := []string{}
	for _, idxStr := range idxStrs {
		idxStr = strings.TrimSpace(idxStr)
		if idxStr == "" {
			continue
		}
		idx, err := strconv.Atoi(idxStr)
		if err != nil || idx < 1 || idx > len(trackers) {
			fmt.Fprintf(os.Stderr, "Invalid selection: %s\n", idxStr)
			continue
		}
		selected = append(selected, trackers[idx-1].EntityID)
	}
	if len(selected) == 0 {
		return nil, fmt.Errorf("no valid devices selected")
	}
	return selected, nil
}
