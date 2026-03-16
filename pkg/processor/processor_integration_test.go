package processor

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"net/http"
	"net/http/httptest"

	"github.com/abpatel/exif-geotagger/pkg/database"
	"github.com/abpatel/exif-geotagger/pkg/exiftool"
	"github.com/abpatel/exif-geotagger/pkg/homeassistant"
)

// mockHAServer creates an httptest.Server that mocks the Home Assistant API endpoints.
func mockHAServer(t *testing.T) *httptest.Server {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Only handle history endpoint; config not needed
		if r.URL.Path == "/api/config" {
			fmt.Fprint(w, `{"timezone":"UTC"}`)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/history/period/") {
			// Check auth
			auth := r.Header.Get("Authorization")
			if auth != "Bearer test-token" {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			// Return mock history: two entries for device_tracker.iphone
			resp := [][]homeassistant.HAState{
				{
					{
						EntityID:   "device_tracker.iphone",
						State:      "home",
						Attributes: json.RawMessage(`{"latitude":37.7749,"longitude":-122.4194,"altitude":15.2,"last_updated_iso":"2023-10-01T12:00:00Z"}`),
					},
					{
						EntityID:   "device_tracker.iphone",
						State:      "home",
						Attributes: json.RawMessage(`{"latitude":37.7750,"longitude":-122.4195,"altitude":15.5,"last_updated_iso":"2023-10-01T13:00:00Z"}`),
					},
				},
			}
			data, _ := json.Marshal(resp)
			w.Header().Set("Content-Type", "application/json")
			w.Write(data)
			return
		}
		http.NotFound(w, r)
	})
	return httptest.NewServer(handler)
}

// createRawImage creates a JPEG file with only a DateTimeOriginal EXIF tag (no GPS).
func createRawImage(t *testing.T, dir string, filename string, ts time.Time) string {
	path := filepath.Join(dir, filename)
	// Create a simple JPEG
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("failed to create image: %v", err)
	}
	defer f.Close()
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	for x := 0; x < 10; x++ {
		for y := 0; y < 10; y++ {
			img.Set(x, y, color.RGBA{255, 0, 0, 255})
		}
	}
	if err := jpeg.Encode(f, img, nil); err != nil {
		t.Fatalf("failed to encode JPEG: %v", err)
	}
	// Set DateTimeOriginal using exiftool CLI
	dtStr := ts.Format("2006:01:02 15:04:05")
	cmd := exec.Command("exiftool", "-DateTimeOriginal="+dtStr, "-overwrite_original", path)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("exiftool failed: %s, %v", out, err)
	}
	return path
}

func TestEndToEnd_HAtoTagImages(t *testing.T) {
	// Start mock HA server
	server := mockHAServer(t)
	defer server.Close()

	// Prepare directories
	rawDir := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "ha.db")

	// 1. Build DB from HA source using -source=ha equivalent
	// Call BuildDB with HA configuration
	err := BuildDB(BuildConfig{
		OutputDB:  dbPath,
		Source:    "ha",
		HAURL:     server.URL,
		HAToken:   "test-token",
		HADevices: "device_tracker.iphone",
		HAStart:   "2023-10-01T00:00:00Z",
		HAEnd:     "2023-10-02T00:00:00Z",
		HADays:    0,
		HAAll:     false,
	})
	if err != nil {
		t.Fatalf("BuildDBHA failed: %v", err)
	}

	// Verify DB entries
	repo, err := database.Connect(dbPath)
	if err != nil {
		t.Fatalf("connect DB: %v", err)
	}
	defer repo.Close()
	entries, err := repo.GetAll(context.Background())
	if err != nil {
		t.Fatalf("get all entries: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected at least one DB entry")
	}
	// We expect two HA entries, both from iphone
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	// 2. Create a raw image with a timestamp that matches one of the HA entries
	// We want the image timestamp to match the first HA entry (12:00 UTC).
	// GetTimestamp now interprets naive EXIF timestamps as UTC (not local).
	// So we write the timestamp directly as UTC.
	imgTime := time.Date(2023, 10, 1, 12, 0, 0, 0, time.UTC)
	rawImg := createRawImage(t, rawDir, "photo.jpg", imgTime)

	// 3. Run tag-images
	err = TagImages(rawDir, dbPath, false, nil)
	if err != nil {
		t.Fatalf("TagImages error: %v", err)
	}

	// 4. Verify GPS tags were written
	meta, err := exiftool.ReadMetadata(rawImg)
	if err != nil {
		t.Fatalf("ReadMetadata failed: %v", err)
	}
	if meta.GPSLatitude == nil || *meta.GPSLatitude != 37.7749 {
		t.Errorf("GPSLatitude = %v, want ~37.7749", meta.GPSLatitude)
	}
	if meta.GPSLongitude == nil || *meta.GPSLongitude != -122.4194 {
		t.Errorf("GPSLongitude = %v, want ~-122.4194", meta.GPSLongitude)
	}
	if meta.GPSAltitude == nil || *meta.GPSAltitude != 15.2 {
		t.Errorf("GPSAltitude = %v, want ~15.2", meta.GPSAltitude)
	}
}
