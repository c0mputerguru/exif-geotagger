package ha

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestParseHAEntry(t *testing.T) {
	fixturePath := "../../testdata/ha_history.json"
	data, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}

	type FixtureEntry struct {
		Description string `json:"description"`
		HAEntry
	}
	var fixtures []FixtureEntry
	if err := json.Unmarshal(data, &fixtures); err != nil {
		t.Fatalf("failed to unmarshal fixture: %v", err)
	}

	// Expected values per description
	exp := map[string]struct {
		hasLocation bool
		lat         float64
		lon         float64
		alt         *float64
		city        *string
		hasTZ       bool
		tz          string
		parseErr    bool
	}{
		"complete": {
			hasLocation: true, lat: 37.7749, lon: -122.4194,
			alt: floatPtr(15.2), city: strPtr("San Francisco"),
			hasTZ: true, tz: "America/Los_Angeles", parseErr: false,
		},
		"missing altitude": {
			hasLocation: true, lat: 34.0522, lon: -118.2437,
			alt: nil, city: strPtr("Los Angeles"),
			hasTZ: false, parseErr: false,
		},
		"missing GPS": {
			hasLocation: false, lat: 0, lon: 0,
			alt: nil, city: strPtr("Nowhere"),
			hasTZ: false, parseErr: false,
		},
		"missing timestamp": {
			hasLocation: true, lat: 40.7128, lon: -74.0060,
			alt: nil, city: strPtr("New York"),
			hasTZ: false, parseErr: true,
		},
		"timestamp with colon format": {
			hasLocation: true, lat: 41.8781, lon: -87.6298,
			alt: nil, city: strPtr("Chicago"),
			hasTZ: false, parseErr: false,
		},
		"invalid timestamp": {
			hasLocation: true, lat: 51.5074, lon: -0.1278,
			alt: nil, city: strPtr("London"),
			hasTZ: false, parseErr: true,
		},
	}

	for _, f := range fixtures {
		t.Run(f.Description, func(t *testing.T) {
			expCase := exp[f.Description]

			// Check location presence
			if got := f.HasLocation(); got != expCase.hasLocation {
				t.Errorf("HasLocation() = %v, want %v", got, expCase.hasLocation)
			}

			// Check latitude if present
			if expCase.lat != 0 {
				if f.Latitude == nil || *f.Latitude != expCase.lat {
					t.Errorf("Latitude = %v, want %f", f.Latitude, expCase.lat)
				}
			} else if f.Latitude != nil {
				t.Errorf("Latitude unexpectedly set to %v", *f.Latitude)
			}

			// Check longitude if present
			if expCase.lon != 0 {
				if f.Longitude == nil || *f.Longitude != expCase.lon {
					t.Errorf("Longitude = %v, want %f", f.Longitude, expCase.lon)
				}
			} else if f.Longitude != nil {
				t.Errorf("Longitude unexpectedly set to %v", *f.Longitude)
			}

			// Check altitude
			if expCase.alt == nil {
				if f.Altitude != nil {
					t.Errorf("Altitude unexpectedly set to %v", *f.Altitude)
				}
			} else if f.Altitude == nil || *f.Altitude != *expCase.alt {
				t.Errorf("Altitude = %v, want %f", f.Altitude, *expCase.alt)
			}

			// Check city
			if expCase.city == nil {
				if f.City != nil {
					t.Errorf("City unexpectedly set to %q", *f.City)
				}
			} else if f.City == nil || *f.City != *expCase.city {
				t.Errorf("City = %v, want %q", f.City, *expCase.city)
			}

			// Check timezone presence
			if expCase.hasTZ {
				if f.Timezone == nil || *f.Timezone != expCase.tz {
					t.Errorf("Timezone = %v, want %q", f.Timezone, expCase.tz)
				}
			} else if f.Timezone != nil {
				t.Errorf("Timezone unexpectedly set to %q", *f.Timezone)
			}

			// Check timestamp parsing
			ts, err := f.GetTimestamp()
			if expCase.parseErr {
				if err == nil {
					t.Errorf("GetTimestamp() expected error, got none")
				}
			} else {
				if err != nil {
					t.Errorf("GetTimestamp() unexpected error: %v", err)
				} else {
					// For complete case, check approximate time matches (ignore zone differences)
					if f.Description == "complete" {
						expTime := time.Date(2023, 10, 1, 12, 0, 0, 0, time.UTC)
						if !ts.Equal(expTime) {
							t.Errorf("GetTimestamp() = %v, want %v", ts, expTime)
						}
					}
					// For colon format case, expect Oct 4, 2023 at 15:00 UTC
					if f.Description == "timestamp with colon format" {
						expTime := time.Date(2023, 10, 4, 15, 0, 0, 0, time.UTC)
						if !ts.Equal(expTime) {
							t.Errorf("GetTimestamp() = %v, want %v", ts, expTime)
						}
					}
				}
			}
		})
	}
}

func TestClient_GetLocationHistory(t *testing.T) {
	// Test success
	t.Run("success", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				t.Errorf("method = %v, want GET", r.Method)
			}
			// Check auth header if token set
			auth := r.Header.Get("Authorization")
			if got, want := auth, "Bearer token123"; got != want {
				t.Errorf("Authorization header = %v, want %v", got, want)
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"locations":[{"timestamp":"2023-10-01T12:00:00Z","latitude":37.7749,"longitude":-122.4194}]}`)
		}))
		defer server.Close()

		client := NewClient(server.URL, "token123")
		entries, err := client.GetLocationHistory(context.Background())
		if err != nil {
			t.Fatalf("GetLocationHistory() error = %v", err)
		}
		if len(entries) != 1 {
			t.Fatalf("got %d entries, want 1", len(entries))
		}
		if entries[0].Latitude == nil || *entries[0].Latitude != 37.7749 {
			t.Errorf("latitude = %v, want 37.7749", entries[0].Latitude)
		}
		if entries[0].Longitude == nil || *entries[0].Longitude != -122.4194 {
			t.Errorf("longitude = %v, want -122.4194", entries[0].Longitude)
		}
	})

	// Test 401 Unauthorized
	t.Run("unauthorized", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		}))
		defer server.Close()

		client := NewClient(server.URL, "token123")
		_, err := client.GetLocationHistory(context.Background())
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "unauthorized") {
			t.Errorf("error message = %v, want contains 'unauthorized'", err.Error())
		}
	})

	// Test 429 Rate Limited
	t.Run("rate limited", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusTooManyRequests)
		}))
		defer server.Close()

		client := NewClient(server.URL, "token123")
		_, err := client.GetLocationHistory(context.Background())
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "rate limited") {
			t.Errorf("error message = %v, want contains 'rate limited'", err.Error())
		}
	})
}

func TestClient_GetTimezone(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				t.Errorf("method = %v, want GET", r.Method)
			}
			expectedLat := "lat=51.5074"
			expectedLon := "lon=-0.1278"
			if !strings.Contains(r.URL.RawQuery, expectedLat) || !strings.Contains(r.URL.RawQuery, expectedLon) {
				t.Errorf("query = %v, want to contain %s and %s", r.URL.RawQuery, expectedLat, expectedLon)
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"timezone":"Europe/London"}`)
		}))
		defer server.Close()

		client := NewClient(server.URL, "")
		tz, err := client.GetTimezone(context.Background(), 51.5074, -0.1278)
		if err != nil {
			t.Fatalf("GetTimezone() error = %v", err)
		}
		if tz != "Europe/London" {
			t.Errorf("timezone = %v, want Europe/London", tz)
		}
	})

	t.Run("error non-200", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		client := NewClient(server.URL, "")
		_, err := client.GetTimezone(context.Background(), 51.5074, -0.1278)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

// Helper functions for pointer values
func strPtr(s string) *string     { return &s }
func floatPtr(f float64) *float64 { return &f }
