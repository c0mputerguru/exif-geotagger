package processor

import (
	"testing"
	"time"

	"github.com/abpatel/exif-geotagger/pkg/database"
)

func TestHasExtension(t *testing.T) {
	tests := []struct {
		name       string
		ext        string
		extensions []string
		want       bool
	}{
		{
			name:       "empty list",
			ext:        ".jpg",
			extensions: []string{},
			want:       false,
		},
		{
			name:       "single match",
			ext:        ".jpg",
			extensions: []string{".jpg", ".jpeg"},
			want:       true,
		},
		{
			name:       "multiple matches",
			ext:        ".png",
			extensions: []string{".jpg", ".jpeg", ".png", ".heic"},
			want:       true,
		},
		{
			name:       "no match",
			ext:        ".gif",
			extensions: []string{".jpg", ".jpeg", ".png"},
			want:       false,
		},
		{
			name:       "case sensitive match",
			ext:        ".JPG",
			extensions: []string{".JPG", ".PNG"},
			want:       true,
		},
		{
			name:       "case sensitive no match",
			ext:        ".jpg",
			extensions: []string{".JPG", ".PNG"},
			want:       false,
		},
		{
			name:       "image file extensions list",
			ext:        ".heic",
			extensions: ImageFileExtensions,
			want:       true,
		},
		{
			name:       "raw file extensions list",
			ext:        ".cr2",
			extensions: RawFileExtensions,
			want:       true,
		},
		{
			name:       "jpeg in raw list",
			ext:        ".jpg",
			extensions: RawFileExtensions,
			want:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasExtension(tt.ext, tt.extensions)
			if got != tt.want {
				t.Errorf("hasExtension(%q, %v) = %v, want %v", tt.ext, tt.extensions, got, tt.want)
			}
		})
	}
}

func TestCopyLocationEntry(t *testing.T) {
	t.Run("DeepCopySemantics", func(t *testing.T) {
		// Create original entry with pointer fields
		altVal := 100.5
		cityVal := "San Francisco"
		stateVal := "CA"
		countryVal := "USA"

		entry := database.LocationEntry{
			Timestamp:   time.Now(),
			Latitude:    37.7749,
			Longitude:   -122.4194,
			Altitude:    &altVal,
			City:        &cityVal,
			State:       &stateVal,
			Country:     &countryVal,
			DeviceModel: "iPhone 14 Pro",
		}

		// Create copy
		copyMeta := copyLocationEntry(entry)

		// Verify copy has same values
		if copyMeta.GPSLatitude == nil || *copyMeta.GPSLatitude != 37.7749 {
			t.Errorf("GPSLatitude mismatch: got %v, want 37.7749", copyMeta.GPSLatitude)
		}
		if copyMeta.GPSLongitude == nil || *copyMeta.GPSLongitude != -122.4194 {
			t.Errorf("GPSLongitude mismatch: got %v, want -122.4194", copyMeta.GPSLongitude)
		}
		if copyMeta.GPSAltitude == nil || *copyMeta.GPSAltitude != 100.5 {
			t.Errorf("GPSAltitude mismatch: got %v, want 100.5", copyMeta.GPSAltitude)
		}
		if copyMeta.City == nil || *copyMeta.City != "San Francisco" {
			t.Errorf("City mismatch: got %v, want San Francisco", copyMeta.City)
		}
		if copyMeta.State == nil || *copyMeta.State != "CA" {
			t.Errorf("State mismatch: got %v, want CA", copyMeta.State)
		}
		if copyMeta.Country == nil || *copyMeta.Country != "USA" {
			t.Errorf("Country mismatch: got %v, want USA", copyMeta.Country)
		}

		// Verify deep copy: modifying original's pointer fields doesn't affect copy
		newAltVal := 200.5
		entry.Altitude = &newAltVal
		if copyMeta.GPSAltitude == nil || *copyMeta.GPSAltitude == 200.5 {
			t.Errorf("GPSAltitude should be unaffected by original modification, got %v", copyMeta.GPSAltitude)
		}

		newCityVal := "Los Angeles"
		entry.City = &newCityVal
		if copyMeta.City == nil || *copyMeta.City == "Los Angeles" {
			t.Errorf("City should be unaffected by original modification, got %v", copyMeta.City)
		}

		newStateVal := "NY"
		entry.State = &newStateVal
		if copyMeta.State == nil || *copyMeta.State == "NY" {
			t.Errorf("State should be unaffected by original modification, got %v", copyMeta.State)
		}

		newCountryVal := "Canada"
		entry.Country = &newCountryVal
		if copyMeta.Country == nil || *copyMeta.Country == "Canada" {
			t.Errorf("Country should be unaffected by original modification, got %v", copyMeta.Country)
		}
	})

	t.Run("NilPointers", func(t *testing.T) {
		// Create entry with nil pointer fields
		entry := database.LocationEntry{
			Timestamp:   time.Now(),
			Latitude:    37.7749,
			Longitude:   -122.4194,
			Altitude:    nil,
			City:        nil,
			State:       nil,
			Country:     nil,
			DeviceModel: "iPhone 14 Pro",
		}

		copyMeta := copyLocationEntry(entry)

		// Non-pointer fields are always copied
		if copyMeta.GPSLatitude == nil || *copyMeta.GPSLatitude != 37.7749 {
			t.Errorf("GPSLatitude should be set, got %v", copyMeta.GPSLatitude)
		}
		if copyMeta.GPSLongitude == nil || *copyMeta.GPSLongitude != -122.4194 {
			t.Errorf("GPSLongitude should be set, got %v", copyMeta.GPSLongitude)
		}

		// Nil pointers should remain nil
		if copyMeta.GPSAltitude != nil {
			t.Errorf("GPSAltitude should be nil, got %v", copyMeta.GPSAltitude)
		}
		if copyMeta.City != nil {
			t.Errorf("City should be nil, got %v", copyMeta.City)
		}
		if copyMeta.State != nil {
			t.Errorf("State should be nil, got %v", copyMeta.State)
		}
		if copyMeta.Country != nil {
			t.Errorf("Country should be nil, got %v", copyMeta.Country)
		}
	})

	t.Run("MixedPointers", func(t *testing.T) {
		// Some pointers set, some nil
		altVal := 50.0
		cityVal := "Seattle"

		entry := database.LocationEntry{
			Timestamp:   time.Now(),
			Latitude:    47.6062,
			Longitude:   -122.3321,
			Altitude:    &altVal,
			City:        &cityVal,
			State:       nil,
			Country:     nil,
			DeviceModel: "Pixel 8",
		}

		copyMeta := copyLocationEntry(entry)

		// Check non-nil pointers
		if copyMeta.GPSAltitude == nil || *copyMeta.GPSAltitude != 50.0 {
			t.Errorf("GPSAltitude mismatch: got %v, want 50.0", copyMeta.GPSAltitude)
		}
		if copyMeta.City == nil || *copyMeta.City != "Seattle" {
			t.Errorf("City mismatch: got %v, want Seattle", copyMeta.City)
		}

		// Check nil pointers
		if copyMeta.State != nil {
			t.Errorf("State should be nil, got %v", copyMeta.State)
		}
		if copyMeta.Country != nil {
			t.Errorf("Country should be nil, got %v", copyMeta.Country)
		}
	})
}
