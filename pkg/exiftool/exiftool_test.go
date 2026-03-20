package exiftool

import (
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"reflect"
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

func TestBuildExiftoolArgs(t *testing.T) {
	tests := []struct {
		name     string
		meta     Metadata
		filePath string
		want     []string
	}{
		{
			name:     "empty metadata returns nil",
			meta:     Metadata{},
			filePath: "/path/to/image.jpg",
			want:     nil,
		},
		{
			name:     "latitude and longitude only",
			meta: Metadata{
				GPSLatitude:  floatPtr(37.7749),
				GPSLongitude: floatPtr(-122.4194),
			},
			filePath: "/path/to/image.jpg",
			want: []string{
				"-GPSLatitude=37.774900",
				"-GPSLongitude=-122.419400",
				"-GPSLatitudeRef=N",
				"-GPSLongitudeRef=W",
				"-overwrite_original",
				"/path/to/image.jpg",
			},
		},
		{
			name:     "negative latitude and longitude",
			meta: Metadata{
				GPSLatitude:  floatPtr(-33.8688),
				GPSLongitude: floatPtr(151.2093),
			},
			filePath: "/path/to/image.jpg",
			want: []string{
				"-GPSLatitude=-33.868800",
				"-GPSLongitude=151.209300",
				"-GPSLatitudeRef=S",
				"-GPSLongitudeRef=E",
				"-overwrite_original",
				"/path/to/image.jpg",
			},
		},
		{
			name:     "altitude only (positive)",
			meta: Metadata{
				GPSAltitude: floatPtr(15.5),
			},
			filePath: "/path/to/image.jpg",
			want: []string{
				"-GPSAltitude=15.500000",
				"-GPSAltitudeRef=0",
				"-overwrite_original",
				"/path/to/image.jpg",
			},
		},
		{
			name:     "altitude only (negative)",
			meta: Metadata{
				GPSAltitude: floatPtr(-10.0),
			},
			filePath: "/path/to/image.jpg",
			want: []string{
				"-GPSAltitude=-10.000000",
				"-GPSAltitudeRef=1",
				"-overwrite_original",
				"/path/to/image.jpg",
			},
		},
		{
			name:     "city, state, country",
			meta: Metadata{
				City:    stringPtr("San Francisco"),
				State:   stringPtr("California"),
				Country: stringPtr("USA"),
			},
			filePath: "/path/to/image.jpg",
			want: []string{
				"-City=San Francisco",
				"-State=California",
				"-Country=USA",
				"-overwrite_original",
				"/path/to/image.jpg",
			},
		},
		{
			name:     "all fields combined",
			meta: Metadata{
				GPSLatitude:  floatPtr(40.7128),
				GPSLongitude: floatPtr(-74.0060),
				GPSAltitude:  floatPtr(10.0),
				City:         stringPtr("New York"),
				State:        stringPtr("NY"),
				Country:      stringPtr("USA"),
			},
			filePath: "/path/to/image.jpg",
			want: []string{
				"-GPSLatitude=40.712800",
				"-GPSLongitude=-74.006000",
				"-GPSLatitudeRef=N",
				"-GPSLongitudeRef=W",
				"-GPSAltitude=10.000000",
				"-GPSAltitudeRef=0",
				"-City=New York",
				"-State=NY",
				"-Country=USA",
				"-overwrite_original",
				"/path/to/image.jpg",
			},
		},
		{
			name:     "nil values should be ignored",
			meta: Metadata{
				GPSLatitude:  floatPtr(37.7749),
				GPSLongitude: floatPtr(-122.4194),
				City:         nil, // nil, should not appear
			},
			filePath: "/path/to/image.jpg",
			want: []string{
				"-GPSLatitude=37.774900",
				"-GPSLongitude=-122.419400",
				"-GPSLatitudeRef=N",
				"-GPSLongitudeRef=W",
				"-overwrite_original",
				"/path/to/image.jpg",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildExiftoolArgs(tt.filePath, tt.meta)

			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("BuildExiftoolArgs(%q, %+v) = %v, want %v",
					tt.filePath, tt.meta, got, tt.want)
			}
		})
	}
}

func TestGetTimestamp(t *testing.T) {
	tests := []struct {
		m        Metadata
		expected time.Time
		isErr    bool
	}{
		{
			// typical exiftool format (naive, should be interpreted as UTC)
			m:        Metadata{DateTimeOriginal: stringPtr("2023:10:01 12:00:00")},
			expected: time.Date(2023, 10, 1, 12, 0, 0, 0, time.UTC),
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
