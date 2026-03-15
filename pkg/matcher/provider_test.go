package matcher

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/abpatel/exif-geotagger/pkg/database"
)

func setupTestDB(t *testing.T) (*database.Repository, func()) {
	// Create a temporary database
	f, err := os.CreateTemp("", "testdb_*.sqlite")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	f.Close()

	repo, err := database.Connect(f.Name())
	if err != nil {
		t.Fatalf("failed to connect to test db: %v", err)
	}

	cleanup := func() {
		repo.Close()
		os.Remove(f.Name())
	}

	return repo, cleanup
}

func TestFindBestMatch(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()

	baseTime := time.Date(2023, 10, 1, 12, 0, 0, 0, time.UTC)

	// Populate DB
	entries := []database.LocationEntry{
		{Timestamp: baseTime.Add(-5 * time.Hour), Latitude: 10.0, Longitude: 10.0, DeviceModel: "Old Camera"},
		{Timestamp: baseTime.Add(-1 * time.Hour), Latitude: 12.0, Longitude: 12.0, DeviceModel: "iPhone 15 Pro"},
		{Timestamp: baseTime.Add(30 * time.Minute), Latitude: 14.0, Longitude: 14.0, DeviceModel: "Pixel 8"},
		{Timestamp: baseTime.Add(2 * time.Hour), Latitude: 16.0, Longitude: 16.0, DeviceModel: "Some tablet"},
	}

	for _, e := range entries {
		if err := repo.Insert(context.Background(), e); err != nil {
			t.Fatalf("failed to insert: %v", err)
		}
	}

	provider := NewSQLiteLocationProvider(repo)

	tests := []struct {
		name          string
		targetTime    time.Time
		priorities    []string
		expectedLat   float64
		expectedError bool
	}{
		{
			name:        "Closest by time, no priorities",
			targetTime:  baseTime.Add(15 * time.Minute), // Only 15 mins away from Pixel 8
			priorities:  []string{},
			expectedLat: 14.0, // Pixel 8 is closest in time
		},
		{
			name:        "Priority device override within window",
			targetTime:  baseTime.Add(15 * time.Minute), // Closer to Pixel 8 but iPhone is priority
			priorities:  []string{"iphone"},
			expectedLat: 12.0, // iPhone 15 Pro is heavily boosted
		},
		{
			name:        "Multiple priorities, closest priority wins",
			targetTime:  baseTime.Add(15 * time.Minute),
			priorities:  []string{"iphone", "pixel"},
			expectedLat: 14.0, // Pixel 8 is closer and both are priority
		},
		{
			name:          "Out of time threshold (threshold is 6h default)",
			targetTime:    baseTime.Add(10 * time.Hour),
			priorities:    []string{"iphone"},
			expectedError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			match, err := provider.FindBestMatch(context.Background(), tc.targetTime, tc.priorities)
			if tc.expectedError {
				if err == nil {
					t.Errorf("expected error, got none")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if match.Latitude != tc.expectedLat {
				t.Errorf("expected latitude %v, got %v", tc.expectedLat, match.Latitude)
			}
		})
	}
}
