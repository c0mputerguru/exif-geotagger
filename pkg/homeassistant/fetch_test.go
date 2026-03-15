package homeassistant

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"
)

// mockClient is a test double for the Client interface.
type mockClient struct {
	body          io.ReadCloser
	timezone      string
	getErr        error
	getTimezoneFn func() (string, error)
}

func (m *mockClient) Get(ctx context.Context, url string) (io.ReadCloser, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	return m.body, nil
}

func (m *mockClient) GetTimezone(ctx context.Context) (string, error) {
	if m.getTimezoneFn != nil {
		return m.getTimezoneFn()
	}
	return m.timezone, nil
}

// newMockClient creates a mock client with the given JSON response.
func newMockClient(t *testing.T, resp interface{}) *mockClient {
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal mock response: %v", err)
	}
	return &mockClient{
		body: io.NopCloser(strings.NewReader(string(data))),
	}
}

// TestFetchLocationHistory_basic tests a successful fetch with valid data.
func TestFetchLocationHistory_basic(t *testing.T) {
	ctx := context.Background()

	// HA API returns a 2D array: one inner array per entity
	haResp := HistoryResponse{
		{ // First entity's states
			HAState{
				EntityID:   "device_tracker.iphone",
				State:      "home",
				Attributes: json.RawMessage(`{"latitude":37.7749,"longitude":-122.4194,"last_updated_iso":"2023-10-01T12:00:00Z"}`),
			},
			HAState{
				EntityID:   "device_tracker.iphone",
				State:      "home",
				Attributes: json.RawMessage(`{"latitude":37.7750,"longitude":-122.4195,"last_updated_iso":"2023-10-01T12:30:00Z"}`),
			},
		},
		{ // Second entity
			HAState{
				EntityID:   "device_tracker.pixel",
				State:      "home",
				Attributes: json.RawMessage(`{"latitude":37.7740,"longitude":-122.4180,"altitude":10.5,"last_updated_iso":"2023-10-01T13:00:00Z"}`),
			},
		},
	}

	client := newMockClient(t, haResp)
	start := time.Date(2023, 10, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2023, 10, 2, 0, 0, 0, 0, time.UTC)
	entityIDs := []string{"device_tracker.iphone", "device_tracker.pixel"}

	entries, err := FetchLocationHistory(ctx, client, start, end, entityIDs)
	if err != nil {
		t.Fatalf("FetchLocationHistory returned error: %v", err)
	}

	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}

	// Verify first entry
	if entries[0].DeviceModel != "device_tracker.iphone" {
		t.Errorf("expected device_model %s, got %s", "device_tracker.iphone", entries[0].DeviceModel)
	}
	if entries[0].Latitude != 37.7749 {
		t.Errorf("expected lat 37.7749, got %f", entries[0].Latitude)
	}
	if entries[0].Longitude != -122.4194 {
		t.Errorf("expected lon -122.4194, got %f", entries[0].Longitude)
	}
	expectedTS := time.Date(2023, 10, 1, 12, 0, 0, 0, time.UTC)
	if !entries[0].Timestamp.Equal(expectedTS) {
		t.Errorf("expected timestamp %v, got %v", expectedTS, entries[0].Timestamp)
	}

	// Third entry should have altitude
	if entries[2].Altitude == nil || *entries[2].Altitude != 10.5 {
		t.Errorf("expected altitude 10.5, got %v", entries[2].Altitude)
	}
}

// TestFetchLocationHistory_emptyResponse tests that empty response returns empty slice.
func TestFetchLocationHistory_emptyResponse(t *testing.T) {
	ctx := context.Background()
	haResp := HistoryResponse{}
	client := newMockClient(t, haResp)
	start := time.Now().Add(-24 * time.Hour)
	end := time.Now()
	entityIDs := []string{"device_tracker.phone"}

	entries, err := FetchLocationHistory(ctx, client, start, end, entityIDs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

// TestFetchLocationHistory_malformedEntry tests that malformed entries are skipped.
func TestFetchLocationHistory_malformedEntry(t *testing.T) {
	ctx := context.Background()
	haResp := HistoryResponse{
		{
			HAState{
				EntityID:   "device_tracker.phone",
				Attributes: json.RawMessage(`{"latitude":"not a number","longitude":-122.4194}`),
			},
			HAState{
				EntityID:   "device_tracker.phone2",
				Attributes: json.RawMessage(`{"latitude":37.7749}`), // missing longitude
			},
			HAState{
				EntityID:   "device_tracker.phone3",
				Attributes: json.RawMessage(`{"latitude":37.7749,"longitude":-122.4194,"last_updated_iso":"2023-10-01T12:00:00Z"}`),
			},
		},
	}
	client := newMockClient(t, haResp)
	start := time.Now().Add(-24 * time.Hour)
	end := time.Now()
	entityIDs := []string{"device_tracker.phone", "device_tracker.phone2", "device_tracker.phone3"}

	entries, err := FetchLocationHistory(ctx, client, start, end, entityIDs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Only the third entry is valid
	if len(entries) != 1 {
		t.Errorf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].DeviceModel != "device_tracker.phone3" {
		t.Errorf("expected device_model device_tracker.phone3, got %s", entries[0].DeviceModel)
	}
}

// TestFetchLocationHistory_missingCoordinates tests that states without lat/lon are skipped.
func TestFetchLocationHistory_missingCoordinates(t *testing.T) {
	ctx := context.Background()
	haResp := HistoryResponse{
		{
			HAState{
				EntityID:   "sensor.temperature",
				Attributes: json.RawMessage(`{"temperature":72.5}`),
			},
		},
	}
	client := newMockClient(t, haResp)
	start := time.Now().Add(-24 * time.Hour)
	end := time.Now()
	entityIDs := []string{"sensor.temperature"}

	entries, err := FetchLocationHistory(ctx, client, start, end, entityIDs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

// TestFetchLocationHistory_noEntityIDs tests error when no entity IDs provided.
func TestFetchLocationHistory_noEntityIDs(t *testing.T) {
	ctx := context.Background()
	client := newMockClient(t, HistoryResponse{})
	start := time.Now()
	end := time.Now()

	_, err := FetchLocationHistory(ctx, client, start, end, nil)
	if err == nil {
		t.Error("expected error for nil entityIDs")
	}
}

// TestFetchLocationHistory_clientError tests handling of client errors.
func TestFetchLocationHistory_clientError(t *testing.T) {
	ctx := context.Background()
	client := newMockClient(t, HistoryResponse{})
	client.getErr = fmt.Errorf("connection failed")
	start := time.Now().Add(-24 * time.Hour)
	end := time.Now()
	entityIDs := []string{"device_tracker.phone"}

	_, err := FetchLocationHistory(ctx, client, start, end, entityIDs)
	if err == nil {
		t.Error("expected error from client")
	}
}

// TestParseLocationFromState_variousTypes tests conversion of numeric types.
func TestParseLocationFromState_variousTypes(t *testing.T) {
	tests := []struct {
		name      string
		attrsJSON string
		wantLat   float64
		wantLon   float64
		wantAlt   *float64
		wantErr   bool
	}{
		{
			name:      "float64 values",
			attrsJSON: `{"latitude":37.7749,"longitude":-122.4194}`,
			wantLat:   37.7749,
			wantLon:   -122.4194,
			wantErr:   false,
		},
		{
			name:      "integer values converted to float64",
			attrsJSON: `{"latitude":37,"longitude":-122}`,
			wantLat:   37,
			wantLon:   -122,
			wantErr:   false,
		},
		{
			name:      "missing longitude",
			attrsJSON: `{"latitude":37.7749}`,
			wantErr:   true,
		},
		{
			name:      "latitude as string",
			attrsJSON: `{"latitude":"37.7749","longitude":"-122.4194"}`,
			wantLat:   0,
			wantLon:   0,
			wantErr:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			state := HAState{
				Attributes: json.RawMessage(tc.attrsJSON),
			}
			lat, lon, alt, _, err := parseLocationFromState(state)
			if tc.wantErr {
				if err == nil {
					t.Errorf("expected error, got none")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if lat != tc.wantLat {
				t.Errorf("expected lat %f, got %f", tc.wantLat, lat)
			}
			if lon != tc.wantLon {
				t.Errorf("expected lon %f, got %f", tc.wantLon, lon)
			}
			if tc.wantAlt != nil {
				if alt == nil || *alt != *tc.wantAlt {
					t.Errorf("expected alt %f, got %v", *tc.wantAlt, alt)
				}
			}
		})
	}
}
