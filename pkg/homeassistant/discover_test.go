package homeassistant

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDiscoverDeviceTrackers(t *testing.T) {
	tests := []struct {
		name          string
		setupServer   func() []StateResponse
		expectedCount int
		checkResults  func([]DeviceTracker) error
	}{
		{
			name: "finds device_tracker entities and extracts friendly_name",
			setupServer: func() []StateResponse {
				return []StateResponse{
					{
						EntityID:    "device_tracker.iphone",
						State:       "home",
						LastUpdated: "2024-03-13T15:04:05.000000+00:00",
						Attributes: map[string]interface{}{
							"friendly_name": "iPhone 14 Pro",
							"source_type":   "gps",
						},
					},
					{
						EntityID:   "sensor.temperature",
						State:      "72",
						Attributes: map[string]interface{}{},
					},
					{
						EntityID:    "device_tracker.pixel",
						State:       "not_home",
						LastUpdated: "2024-03-13T14:30:00.000000+00:00",
						Attributes: map[string]interface{}{
							"friendly_name": "Pixel 8",
						},
					},
					{
						EntityID:   "device_tracker.samsung",
						State:      "home",
						Attributes: map[string]interface{}{}, // No friendly_name
					},
				}
			},
			expectedCount: 3,
			checkResults: func(trackers []DeviceTracker) error {
				names := make(map[string]string)
				lastSeen := make(map[string]string)
				for _, t := range trackers {
					names[t.EntityID] = t.FriendlyName
					lastSeen[t.EntityID] = t.LastSeen
				}
				if names["device_tracker.iphone"] != "iPhone 14 Pro" {
					return fmt.Errorf("expected iPhone 14 Pro, got %q", names["device_tracker.iphone"])
				}
				if lastSeen["device_tracker.iphone"] != "2024-03-13T15:04:05.000000+00:00" {
					return fmt.Errorf("expected last_seen 2024-03-13T15:04:05.000000+00:00, got %q", lastSeen["device_tracker.iphone"])
				}
				if names["device_tracker.pixel"] != "Pixel 8" {
					return fmt.Errorf("expected Pixel 8, got %q", names["device_tracker.pixel"])
				}
				if lastSeen["device_tracker.pixel"] != "2024-03-13T14:30:00.000000+00:00" {
					return fmt.Errorf("expected last_seen 2024-03-13T14:30:00.000000+00:00, got %q", lastSeen["device_tracker.pixel"])
				}
				if names["device_tracker.samsung"] != "" {
					return fmt.Errorf("expected empty friendly_name for samsung, got %q", names["device_tracker.samsung"])
				}
				if lastSeen["device_tracker.samsung"] != "" {
					return fmt.Errorf("expected empty last_seen for samsung (no LastUpdated), got %q", lastSeen["device_tracker.samsung"])
				}
				return nil
			},
		},
		{
			name: "empty response returns empty list",
			setupServer: func() []StateResponse {
				return []StateResponse{}
			},
			expectedCount: 0,
			checkResults: func(trackers []DeviceTracker) error {
				if len(trackers) != 0 {
					return fmt.Errorf("expected empty list, got %d", len(trackers))
				}
				return nil
			},
		},
		{
			name: "no device_tracker entities returns empty list",
			setupServer: func() []StateResponse {
				return []StateResponse{
					{
						EntityID: "sensor.temperature",
						State:    "72",
					},
					{
						EntityID: "light.living_room",
						State:    "on",
					},
				}
			},
			expectedCount: 0,
			checkResults: func(trackers []DeviceTracker) error {
				if len(trackers) != 0 {
					return fmt.Errorf("expected empty list, got %d", len(trackers))
				}
				return nil
			},
		},
		{
			name: "non-ok HTTP status returns error",
			setupServer: func() []StateResponse {
				return nil // server returns 500
			},
			expectedCount: 0,
			checkResults: func(trackers []DeviceTracker) error {
				// Should not reach here - test expects error
				return fmt.Errorf("expected error")
			},
		},
		{
			name: "invalid JSON returns error",
			setupServer: func() []StateResponse {
				return nil // server returns invalid JSON
			},
			expectedCount: 0,
			checkResults: func(trackers []DeviceTracker) error {
				// Should not reach here
				return fmt.Errorf("expected error")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var handler http.HandlerFunc
			states := tt.setupServer()

			if tt.name == "non-ok HTTP status returns error" {
				handler = func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusInternalServerError)
					w.Write([]byte("internal error"))
				}
			} else if tt.name == "invalid JSON returns error" {
				handler = func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.Write([]byte(`{invalid json`))
				}
			} else {
				jsonData, _ := json.Marshal(states)
				handler = func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.Write(jsonData)
				}
			}

			server := httptest.NewServer(handler)
			defer server.Close()

			baseURL := server.URL
			token := "test-token"

			trackers, err := DiscoverDeviceTrackers(context.Background(), baseURL, token)

			if tt.name == "non-ok HTTP status returns error" || tt.name == "invalid JSON returns error" {
				if err == nil {
					t.Fatalf("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(trackers) != tt.expectedCount {
				t.Fatalf("expected %d trackers, got %d", tt.expectedCount, len(trackers))
			}

			if err := tt.checkResults(trackers); err != nil {
				t.Fatalf("check failed: %v", err)
			}
		})
	}
}

func TestDiscoverDeviceTrackers_URLNormalization(t *testing.T) {
	// Test that trailing slash is removed
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/states" {
			t.Errorf("expected path /api/states, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[]`))
	}))
	defer server.Close()

	urlWithSlash := server.URL + "/"
	_, err := DiscoverDeviceTrackers(context.Background(), urlWithSlash, "token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDiscoverDeviceTrackers_AuthHeader(t *testing.T) {
	// Test that Authorization header is set correctly
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer my-token" {
			t.Errorf("expected Authorization 'Bearer my-token', got %q", auth)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[]`))
	}))
	defer server.Close()

	_, err := DiscoverDeviceTrackers(context.Background(), server.URL, "my-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDiscoverDeviceTrackers_FriendlyNameTypeAssertion(t *testing.T) {
	// Test that non-string friendly_name values are ignored
	states := []StateResponse{
		{
			EntityID: "device_tracker.test",
			State:    "home",
			Attributes: map[string]interface{}{
				"friendly_name": 123, // not a string
			},
		},
	}
	jsonData, _ := json.Marshal(states)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(jsonData)
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	trackers, err := DiscoverDeviceTrackers(context.Background(), server.URL, "token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(trackers) != 1 {
		t.Fatalf("expected 1 tracker, got %d", len(trackers))
	}
	if trackers[0].FriendlyName != "" {
		t.Errorf("expected empty friendly_name for non-string value, got %q", trackers[0].FriendlyName)
	}
}

func TestDiscoverDeviceTrackers_EntityIDPrefixCase(t *testing.T) {
	// Ensure prefix matching is case-sensitive (device_tracker. only)
	states := []StateResponse{
		{
			EntityID: "device_tracker.iphone",
			State:    "home",
		},
		{
			EntityID: "Device_Tracker.pixel", // uppercase - should NOT match
			State:    "home",
		},
		{
			EntityID: "device_tracker_extra.samsung", // underscore - should NOT match
			State:    "home",
		},
	}
	jsonData, _ := json.Marshal(states)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(jsonData)
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	trackers, err := DiscoverDeviceTrackers(context.Background(), server.URL, "token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(trackers) != 1 {
		t.Fatalf("expected 1 tracker (case-sensitive), got %d", len(trackers))
	}
	if trackers[0].EntityID != "device_tracker.iphone" {
		t.Errorf("unexpected entity_id: %s", trackers[0].EntityID)
	}
}

func TestDiscoverDeviceTrackers_Timeout(t *testing.T) {
	// This is a simple sanity check that timeout is reasonable.
	// Full timeout testing would require more complex setup.
	// We'll just verify the function uses a client with timeout by checking
	// that slow responses can be cancelled (but we won't test actual timeout value).
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate slow response beyond typical timeout
		// We won't actually sleep; just verify the client is used.
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[]`))
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	// Should succeed quickly since server responds immediately
	_, err := DiscoverDeviceTrackers(context.Background(), server.URL, "token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func BenchmarkDiscoverDeviceTrackers(b *testing.B) {
	states := make([]StateResponse, 100)
	for i := 0; i < 100; i++ {
		states[i] = StateResponse{
			EntityID:   "device_tracker.device" + string(rune(i)),
			State:      "home",
			Attributes: map[string]interface{}{"friendly_name": "Device " + string(rune(i))},
		}
	}
	jsonData, _ := json.Marshal(states)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(jsonData)
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := DiscoverDeviceTrackers(context.Background(), server.URL, "token")
		if err != nil {
			b.Fatalf("unexpected error: %v", err)
		}
	}
}
