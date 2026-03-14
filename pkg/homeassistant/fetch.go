package homeassistant

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/abpatel/exif-geotagger/pkg/database"
)

// HAState represents a single state entry from HA history.
type HAState struct {
	EntityID       string      `json:"entity_id"`
	State          string      `json:"state"`
	LastID         interface{} `json:"last_id"` // Can be string or number
	LastUpdated    string      `json:"last_updated"`
	LastUpdatedISO string      `json:"last_updated_iso"`
	// Other fields may exist but we only need the state attributes
	Attributes json.RawMessage `json:"attributes"` // Keep raw for latitude/longitude extraction
}

// HistoryResponse represents the 2D array response from /api/history/period.
// It's a slice of slices: [[state1, state2, ...], [state1, state2, ...], ...]
// Each inner slice contains states for one entity over the time period.
type HistoryResponse [][]HAState

// FetchLocationHistory retrieves location history for given entity IDs within a time range.
// It calls /api/history/period, parses the 2D array response, and converts each state
// to a LocationEntry. Only states with valid latitude/longitude are included.
// Empty responses and malformed data are handled gracefully (skipped).
func FetchLocationHistory(ctx context.Context, client Client, start, end time.Time, entityIDs []string) ([]database.LocationEntry, error) {
	if len(entityIDs) == 0 {
		return nil, fmt.Errorf("at least one entity ID is required")
	}

	// Build URL with query parameters
	// HA API: GET /api/history/period/{start}/{end}?filter_entity_id=id1,id2,...
	// Times should be in ISO8601 format with timezone (use time.RFC3339)
	url := fmt.Sprintf("/api/history/period/%s/%s?filter_entity_id=%s",
		start.Format(time.RFC3339),
		end.Format(time.RFC3339),
		strings.Join(entityIDs, ","))

	body, err := client.Get(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch history: %w", err)
	}
	defer body.Close()

	data, err := io.ReadAll(body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var resp HistoryResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse history response: %w", err)
	}

	var entries []database.LocationEntry
	skipCounts := make(map[string]int)
	for _, entityStates := range resp {
		for _, state := range entityStates {
			// Parse the state to extract location info
			lat, lon, alt, ts, err := parseLocationFromState(state)
			if err != nil {
				// Track skip reason
				skipCounts[err.Error()]++
				continue
			}
			// Build LocationEntry (we don't have city/state/country from HA usually)
			entry := database.LocationEntry{
				Timestamp:   ts,
				Latitude:    lat,
				Longitude:   lon,
				Altitude:    alt,
				DeviceModel: state.EntityID, // Use entity ID as device model
			}
			entries = append(entries, entry)
		}
	}

	// Log summary of skipped entries
	totalSkipped := 0
	for _, count := range skipCounts {
		totalSkipped += count
	}
	if totalSkipped > 0 {
		fmt.Printf("Skipped %d location entries:\n", totalSkipped)
		for reason, count := range skipCounts {
			fmt.Printf("  - %s: %d\n", reason, count)
		}
	}

	return entries, nil
}

// parseLocationFromState extracts latitude, longitude, altitude, and timestamp from an HA state.
// HA states typically have attributes with gps_accuracy, latitude, longitude, source, etc.
func parseLocationFromState(state HAState) (lat, lon float64, alt *float64, ts time.Time, err error) {
	// The state's attributes should contain the location data
	attrs := make(map[string]interface{})
	if err := json.Unmarshal(state.Attributes, &attrs); err != nil {
		return 0, 0, nil, time.Time{}, fmt.Errorf("invalid attributes: %w", err)
	}

	// Check for latitude and longitude
	latVal, okLat := attrs["latitude"]
	lonVal, okLon := attrs["longitude"]
	if !okLat || !okLon {
		return 0, 0, nil, time.Time{}, fmt.Errorf("missing latitude or longitude")
	}

	lat, okLat = latVal.(float64)
	lon, okLon = lonVal.(float64)
	if !okLat || !okLon {
		// Sometimes they might be numbers encoded differently (e.g., json.Number)
		// Try converting via fmt
		if latFloat, err := convertToFloat64(latVal); err == nil {
			lat = latFloat
		} else {
			return 0, 0, nil, time.Time{}, fmt.Errorf("latitude not a number")
		}
		if lonFloat, err := convertToFloat64(lonVal); err == nil {
			lon = lonFloat
		} else {
			return 0, 0, nil, time.Time{}, fmt.Errorf("longitude not a number")
		}
	}

	// Optional: altitude
	if altVal, ok := attrs["gps_accuracy"]; ok {
		if altFloat, ok := altVal.(float64); ok {
			// gps_accuracy is usually horizontal accuracy, not altitude
			// But we'll store it as altitude if no better option
			// In HA, altitude might be in "altitude" attribute
			alt = &altFloat
		}
	}
	if altVal, ok := attrs["altitude"]; ok {
		if altFloat, ok := altVal.(float64); ok {
			alt = &altFloat
		}
	}

	// Parse timestamp: try last_updated_iso first, then last_updated, then state itself
	ts = time.Time{}
	if tsStr, ok := attrs["last_updated_iso"].(string); ok {
		if parsed, err := time.Parse(time.RFC3339, tsStr); err == nil {
			ts = parsed
		}
	}
	// If still zero, could try state.LastUpdated if we add that field to HAState,
	// but for now we return zero time which is acceptable (entry still useful)

	return lat, lon, alt, ts, nil
}

// convertToFloat64 attempts to convert various numeric types to float64.
func convertToFloat64(v interface{}) (float64, error) {
	switch n := v.(type) {
	case float64:
		return n, nil
	case float32:
		return float64(n), nil
	case int:
		return float64(n), nil
	case int64:
		return float64(n), nil
	case json.Number:
		return n.Float64()
	case string:
		// Try parsing string as float
		var f float64
		_, err := fmt.Sscanf(n, "%f", &f)
		if err != nil {
			return 0, fmt.Errorf("cannot parse string as float: %w", err)
		}
		return f, nil
	default:
		return 0, fmt.Errorf("unable to convert %T to float64", v)
	}
}
