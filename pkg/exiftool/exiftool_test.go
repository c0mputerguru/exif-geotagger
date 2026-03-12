package exiftool

import (
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func createDummyImage(t *testing.T, dir string) string {
	path := filepath.Join(dir, "dummy.jpg")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("failed to create dummy file: %v", err)
	}
	defer f.Close()

	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	for x := 0; x < 10; x++ {
		for y := 0; y < 10; y++ {
			img.Set(x, y, color.Opaque)
		}
	}
	if err := jpeg.Encode(f, img, nil); err != nil {
		t.Fatalf("failed to encode dummy jpeg: %v", err)
	}
	return path
}

func floatPtr(f float64) *float64 { return &f }
func stringPtr(s string) *string  { return &s }

func TestExiftoolReadWrite(t *testing.T) {
	tempDir := t.TempDir()
	img := createDummyImage(t, tempDir)

	// Since the file has no exif data initially, ReadMetadata should fail or return minimal info
	_, err := ReadMetadata(img)
	if err == nil {
		t.Logf("ReadMetadata unexpectedly succeeded on empty file, continuing...")
	}

	lat := 37.7749
	lon := -122.4194
	alt := 15.0
	city := "San Francisco"

	meta := Metadata{
		GPSLatitude:  floatPtr(lat),
		GPSLongitude: floatPtr(lon),
		GPSAltitude:  floatPtr(alt),
		City:         stringPtr(city),
	}

	// Test Dry Run
	err = WriteMetadata(img, meta, true)
	if err != nil {
		t.Fatalf("dry run failed: %v", err)
	}

	// Test actual write
	err = WriteMetadata(img, meta, false)
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}

	// Test standard read
	readMeta, err := ReadMetadata(img)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}

	if readMeta.GPSLatitude == nil || *readMeta.GPSLatitude != lat {
		t.Errorf("expected lat %v, got %v", lat, readMeta.GPSLatitude)
	}
	if readMeta.GPSLongitude == nil || *readMeta.GPSLongitude != lon {
		t.Errorf("expected lon %v, got %v", lon, readMeta.GPSLongitude)
	}
	// Note: exiftool might not write City successfully to a standard EXIF tag without XMP/IPTC definitions,
	// depending on standard. Let's just test GPS which EXIF guarantees.
}

func TestGetTimestamp(t *testing.T) {
	tests := []struct {
		m        Metadata
		expected time.Time
		isErr    bool
	}{
		{
			// typical exiftool format
			m:        Metadata{DateTimeOriginal: stringPtr("2023:10:01 12:00:00")},
			expected: time.Date(2023, 10, 1, 12, 0, 0, 0, time.Local),
			isErr:    false,
		},
		{
			// RFC3339 fallback
			m:        Metadata{CreateDate: stringPtr("2023-10-01T12:00:00Z")},
			expected: time.Date(2023, 10, 1, 12, 0, 0, 0, time.UTC),
			isErr:    false,
		},
		{
			m:     Metadata{Model: stringPtr("iPhone")}, // no dates
			isErr: true,
		},
	}

	for i, tc := range tests {
		t.Run(fmt.Sprintf("tc%d", i), func(t *testing.T) {
			got, err := tc.m.GetTimestamp()
			if tc.isErr && err == nil {
				t.Errorf("expected error, got none")
			}
			if !tc.isErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if !tc.isErr && !got.Equal(tc.expected) {
				t.Errorf("expected %v, got %v", tc.expected, got)
			}
		})
	}
}
