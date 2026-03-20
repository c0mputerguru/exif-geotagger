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
	"github.com/abpatel/exif-geotagger/pkg/matcher"
)

// exiftoolAvailable checks if the exiftool binary is available in PATH.
func exiftoolAvailable() bool {
	_, err := exec.LookPath("exiftool")
	return err == nil
}

// mockHAServer creates an httptest.Server that mocks the Home Assistant API endpoints.
func mockHAServer(t *testing.T) *httptest.Server {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check auth for protected endpoints
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-token" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		if r.URL.Path == "/api/config" {
			fmt.Fprint(w, `{"timezone":"UTC"}`)
			return
		}
		if r.URL.Path == "/api/states" {
			// Return mock states including a device_tracker entity
			states := []homeassistant.StateResponse{
				{
					EntityID:    "device_tracker.iphone",
					State:       "home",
					Attributes:  map[string]interface{}{"friendly_name": "iPhone"},
					LastChanged: "2023-10-01T12:00:00Z",
					LastUpdated: "2023-10-01T12:00:00Z",
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(states)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/history/period/") {
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

// createImageWithGPS creates a JPEG file with GPS metadata, timestamp, and device model.
func createImageWithGPS(t *testing.T, dir string, filename string, lat, lon float64, alt float64, ts time.Time, model string) string {
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
			img.Set(x, y, color.RGBA{0, 255, 0, 255})
		}
	}
	if err := jpeg.Encode(f, img, nil); err != nil {
		t.Fatalf("failed to encode JPEG: %v", err)
	}
	// Set GPS and other metadata using exiftool CLI
	dtStr := ts.Format("2006:01:02 15:04:05")
	latRef := "N"
	if lat < 0 {
		latRef = "S"
	}
	lonRef := "E"
	if lon < 0 {
		lonRef = "W"
	}
	altRef := "0"
	if alt < 0 {
		altRef = "1"
	}
	cmd := exec.Command("exiftool",
		"-DateTimeOriginal="+dtStr,
		"-GPSLatitude="+fmt.Sprintf("%f", lat),
		"-GPSLongitude="+fmt.Sprintf("%f", lon),
		"-GPSAltitude="+fmt.Sprintf("%f", alt),
		"-GPSLatitudeRef="+latRef,
		"-GPSLongitudeRef="+lonRef,
		"-GPSAltitudeRef="+altRef,
		"-Model="+model,
		"-overwrite_original",
		path,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("exiftool failed: %s, %v", out, err)
	}
	return path
}

func TestEndToEnd_HAtoTagImages(t *testing.T) {
	if !exiftoolAvailable() {
		t.Skip("exiftool binary not found in PATH, skipping integration test")
	}
	// Start mock HA server
	server := mockHAServer(t)
	defer server.Close()

	// Prepare directories
	rawDir := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "ha.db")

	// 1. Build DB from HA source using -source=ha equivalent
	// Call BuildDB with HA configuration
	err := BuildDB(context.Background(), BuildConfig{
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
	err = TagImages(context.Background(), rawDir, dbPath, false, nil, matcher.ProviderOptions{
		SearchWindow:       matcher.DefaultSearchWindow,
		TimeThreshold:      matcher.DefaultTimeThreshold,
		PriorityMultiplier: matcher.DefaultPriorityMultiplier,
	}, nil)
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

func TestEndToEnd_BuildDBFromImages(t *testing.T) {
	if !exiftoolAvailable() {
		t.Skip("exiftool binary not found in PATH, skipping integration test")
	}
	// Prepare directories
	imagesDir := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "images.db")

	// Create test images with GPS metadata from different devices
	// Image 1: iPhone at 2023-10-01 12:00:00 UTC
	img1Time := time.Date(2023, 10, 1, 12, 0, 0, 0, time.UTC)
	createImageWithGPS(t, imagesDir, "iphone_photo1.jpg", 37.7749, -122.4194, 15.2, img1Time, "iPhone 14 Pro")

	// Image 2: Pixel at 2023-10-01 13:00:00 UTC
	img2Time := time.Date(2023, 10, 1, 13, 0, 0, 0, time.UTC)
	createImageWithGPS(t, imagesDir, "pixel_photo1.jpg", 37.7750, -122.4195, 20.1, img2Time, "Pixel 8")

	// Image 3: Another iPhone photo at 2023-10-01 14:00:00 UTC
	img3Time := time.Date(2023, 10, 1, 14, 0, 0, 0, time.UTC)
	createImageWithGPS(t, imagesDir, "iphone_photo2.jpg", 37.7760, -122.4200, 12.5, img3Time, "iPhone 14 Pro")

	// Build database from images source
	err := BuildDB(context.Background(), BuildConfig{
		OutputDB: dbPath,
		Source:   "images",
		InputDir: imagesDir,
	})
	if err != nil {
		t.Fatalf("BuildDB from images failed: %v", err)
	}

	// Verify database entries
	repo, err := database.Connect(dbPath)
	if err != nil {
		t.Fatalf("connect DB: %v", err)
	}
	defer repo.Close()
	entries, err := repo.GetAll(context.Background())
	if err != nil {
		t.Fatalf("get all entries: %v", err)
	}

	// We expect 3 entries
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}

	// Count entries by model
	modelsCount := make(map[string]int)
	for _, e := range entries {
		modelsCount[e.DeviceModel]++
	}

	// Check we have 2 iPhone entries and 1 Pixel entry
	if modelsCount["iPhone 14 Pro"] != 2 {
		t.Errorf("expected 2 iPhone 14 Pro entries, got %d", modelsCount["iPhone 14 Pro"])
	}
	if modelsCount["Pixel 8"] != 1 {
		t.Errorf("expected 1 Pixel 8 entry, got %d", modelsCount["Pixel 8"])
	}

	// Verify specific entry values by finding them
	var iphone1, iphone2, pixelEntry *database.LocationEntry
	for i := range entries {
		e := entries[i]
		if e.DeviceModel == "iPhone 14 Pro" {
			if e.Timestamp.Equal(img1Time) {
				iphone1 = &e
			} else if e.Timestamp.Equal(img3Time) {
				iphone2 = &e
			}
		} else if e.DeviceModel == "Pixel 8" && e.Timestamp.Equal(img2Time) {
			pixelEntry = &e
		}
	}

	// Check iPhone 1
	if iphone1 == nil {
		t.Error("missing iPhone 14 Pro entry for first image")
	} else {
		if iphone1.Latitude != 37.7749 || iphone1.Longitude != -122.4194 {
			t.Errorf("iPhone 1 location mismatch: got (%f, %f)", iphone1.Latitude, iphone1.Longitude)
		}
		if *iphone1.Altitude != 15.2 {
			t.Errorf("iPhone 1 altitude mismatch: got %f", *iphone1.Altitude)
		}
	}

	// Check iPhone 2
	if iphone2 == nil {
		t.Error("missing iPhone 14 Pro entry for second image")
	} else {
		if iphone2.Latitude != 37.7760 || iphone2.Longitude != -122.4200 {
			t.Errorf("iPhone 2 location mismatch: got (%f, %f)", iphone2.Latitude, iphone2.Longitude)
		}
		if *iphone2.Altitude != 12.5 {
			t.Errorf("iPhone 2 altitude mismatch: got %f", *iphone2.Altitude)
		}
	}

	// Check Pixel
	if pixelEntry == nil {
		t.Error("missing Pixel 8 entry")
	} else {
		if pixelEntry.Latitude != 37.7750 || pixelEntry.Longitude != -122.4195 {
			t.Errorf("Pixel location mismatch: got (%f, %f)", pixelEntry.Latitude, pixelEntry.Longitude)
		}
		if *pixelEntry.Altitude != 20.1 {
			t.Errorf("Pixel altitude mismatch: got %f", *pixelEntry.Altitude)
		}
	}
}

func TestEndToEnd_BuildDBFromImages_WithFilter(t *testing.T) {
	if !exiftoolAvailable() {
		t.Skip("exiftool binary not found in PATH, skipping integration test")
	}
	// Test filtering by device models
	imagesDir := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "filtered.db")

	// Create images from two different models
	img1Time := time.Date(2023, 10, 1, 12, 0, 0, 0, time.UTC)
	createImageWithGPS(t, imagesDir, "iphone.jpg", 37.7749, -122.4194, 15.2, img1Time, "iPhone 14 Pro")

	img2Time := time.Date(2023, 10, 1, 13, 0, 0, 0, time.UTC)
	createImageWithGPS(t, imagesDir, "pixel.jpg", 37.7750, -122.4195, 20.1, img2Time, "Pixel 8")

	// Build database with filter for only iPhone
	err := BuildDB(context.Background(), BuildConfig{
		OutputDB:     dbPath,
		Source:       "images",
		InputDir:     imagesDir,
		FilterModels: []string{"iPhone 14 Pro"},
	})
	if err != nil {
		t.Fatalf("BuildDB with filter failed: %v", err)
	}

	// Verify only iPhone entries were added
	repo, err := database.Connect(dbPath)
	if err != nil {
		t.Fatalf("connect DB: %v", err)
	}
	defer repo.Close()
	entries, err := repo.GetAll(context.Background())
	if err != nil {
		t.Fatalf("get all entries: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("expected 1 entry after filter, got %d", len(entries))
	}
	if entries[0].DeviceModel != "iPhone 14 Pro" {
		t.Errorf("expected iPhone 14 Pro, got %s", entries[0].DeviceModel)
	}
}

// TestBuildDBFromImages_WithAllFlag tests that when FilterModels is empty (no filter),
// all device models from images are included (equivalent to -all flag behavior).
func TestBuildDBFromImages_WithAllFlag(t *testing.T) {
	if !exiftoolAvailable() {
		t.Skip("exiftool binary not found in PATH, skipping integration test")
	}
	imagesDir := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "all.db")

	// Create images from two different models
	img1Time := time.Date(2023, 10, 1, 12, 0, 0, 0, time.UTC)
	createImageWithGPS(t, imagesDir, "iphone.jpg", 37.7749, -122.4194, 15.2, img1Time, "iPhone 14 Pro")

	img2Time := time.Date(2023, 10, 1, 13, 0, 0, 0, time.UTC)
	createImageWithGPS(t, imagesDir, "pixel.jpg", 37.7750, -122.4195, 20.1, img2Time, "Pixel 8")

	// Build database without FilterModels (empty slice) - should include all
	err := BuildDB(context.Background(), BuildConfig{
		OutputDB: dbPath,
		Source:   "images",
		InputDir: imagesDir,
		// FilterModels: nil (empty) means include all
	})
	if err != nil {
		t.Fatalf("BuildDB failed: %v", err)
	}

	// Verify both entries were added
	repo, err := database.Connect(dbPath)
	if err != nil {
		t.Fatalf("connect DB: %v", err)
	}
	defer repo.Close()
	entries, err := repo.GetAll(context.Background())
	if err != nil {
		t.Fatalf("get all entries: %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("expected 2 entries (all devices), got %d", len(entries))
	}

	modelsCount := make(map[string]int)
	for _, e := range entries {
		modelsCount[e.DeviceModel]++
	}
	if modelsCount["iPhone 14 Pro"] != 1 {
		t.Errorf("expected 1 iPhone 14 Pro, got %d", modelsCount["iPhone 14 Pro"])
	}
	if modelsCount["Pixel 8"] != 1 {
		t.Errorf("expected 1 Pixel 8, got %d", modelsCount["Pixel 8"])
	}
}

// TestBuildDBFromImages_ErrorMissingInputDir tests that BuildDB returns an error
// when InputDir is empty for images source.
func TestBuildDBFromImages_ErrorMissingInputDir(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "error.db")

	err := BuildDB(context.Background(), BuildConfig{
		OutputDB: dbPath,
		Source:   "images",
		// InputDir is empty
	})
	if err == nil {
		t.Fatal("expected error for missing InputDir, got nil")
	}
	if !strings.Contains(err.Error(), "inputDir is required") {
		t.Errorf("error message should mention inputDir requirement, got: %v", err)
	}
}

// TestBuildDB_InvalidSource tests that BuildDB returns an error for an invalid source.
// Since non-ha source defaults to images, the error will be about missing InputDir.
func TestBuildDB_InvalidSource(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "invalid.db")

	err := BuildDB(context.Background(), BuildConfig{
		OutputDB: dbPath,
		Source:   "invalid",
	})
	if err == nil {
		t.Fatal("expected error for invalid source, got nil")
	}
	// The invalid source defaults to images, which requires InputDir
	if !strings.Contains(err.Error(), "inputDir is required") {
		t.Errorf("error should mention inputDir requirement, got: %v", err)
	}
}

// TestBuildDBFromHA_ErrorMissingCredentials tests that BuildDB returns an error
// when HAURL or HAToken is empty for HA source.
func TestBuildDBFromHA_ErrorMissingCredentials(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "ha.db")

	// Missing URL
	err := BuildDB(context.Background(), BuildConfig{
		OutputDB: dbPath,
		Source:   "ha",
		HAToken:  "token",
	})
	if err == nil {
		t.Fatal("expected error for missing HAURL, got nil")
	}

	// Missing token
	err = BuildDB(context.Background(), BuildConfig{
		OutputDB: dbPath,
		Source:   "ha",
		HAURL:    "http://ha",
	})
	if err == nil {
		t.Fatal("expected error for missing HAToken, got nil")
	}
}

// TestBuildDBFromHA_WithAllDevicesFlag tests HA source with HADevices empty and HAAll=true,
// which should discover all device_tracker entities.
func TestBuildDBFromHA_WithAllDevicesFlag(t *testing.T) {
	if !exiftoolAvailable() {
		t.Skip("exiftool binary not found in PATH, skipping integration test")
	}
	// Use mock HA server
	server := mockHAServer(t)
	defer server.Close()

	dbPath := filepath.Join(t.TempDir(), "ha_all.db")

	// Build with HAAll=true and no HADevices specified
	err := BuildDB(context.Background(), BuildConfig{
		OutputDB: dbPath,
		Source:   "ha",
		HAURL:    server.URL,
		HAToken:  "test-token",
		HAAll:    true,
	})
	if err != nil {
		t.Fatalf("BuildDB from HA with -all failed: %v", err)
	}

	// Verify DB has entries (the mock server returns 2 entries for device_tracker.iphone)
	repo, err := database.Connect(dbPath)
	if err != nil {
		t.Fatalf("connect DB: %v", err)
	}
	defer repo.Close()
	entries, err := repo.GetAll(context.Background())
	if err != nil {
		t.Fatalf("get all entries: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries from HA -all, got %d", len(entries))
	}
}

// TestBuildDBFromHA_WithDaysFlag tests HA source with -ha-days flag.
func TestBuildDBFromHA_WithDaysFlag(t *testing.T) {
	if !exiftoolAvailable() {
		t.Skip("exiftool binary not found in PATH, skipping integration test")
	}
	server := mockHAServer(t)
	defer server.Close()

	dbPath := filepath.Join(t.TempDir(), "ha_days.db")

	// Build with HADays=7 (last 7 days)
	err := BuildDB(context.Background(), BuildConfig{
		OutputDB:  dbPath,
		Source:    "ha",
		HAURL:     server.URL,
		HAToken:   "test-token",
		HADevices: "device_tracker.iphone",
		HADays:    7,
		// HAStart and HAEnd should be ignored when HADays is set
	})
	if err != nil {
		t.Fatalf("BuildDB from HA with -days failed: %v", err)
	}

	// Verify DB contains entries (mock server returns data regardless of dates, so we just check it succeeded)
	repo, err := database.Connect(dbPath)
	if err != nil {
		t.Fatalf("connect DB: %v", err)
	}
	defer repo.Close()
	entries, err := repo.GetAll(context.Background())
	if err != nil {
		t.Fatalf("get all entries: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries from HA with days flag, got %d", len(entries))
	}
}

// TestBuildDBFromHA_ErrorInvalidTimeRange tests that BuildDB returns an error
// when ha-start or ha-end have invalid RFC3339 format.
func TestBuildDBFromHA_ErrorInvalidTimeRange(t *testing.T) {
	server := mockHAServer(t)
	defer server.Close()

	dbPath := filepath.Join(t.TempDir(), "ha_invalid.db")

	// Invalid start format
	err := BuildDB(context.Background(), BuildConfig{
		OutputDB:  dbPath,
		Source:    "ha",
		HAURL:     server.URL,
		HAToken:   "test-token",
		HADevices: "device_tracker.iphone",
		HAStart:   "not-a-date",
		HAEnd:     "2023-10-02T00:00:00Z",
	})
	if err == nil {
		t.Fatal("expected error for invalid ha-start format, got nil")
	}
	if !strings.Contains(err.Error(), "invalid ha-start") {
		t.Errorf("error should mention invalid ha-start, got: %v", err)
	}

	// Invalid end format
	err = BuildDB(context.Background(), BuildConfig{
		OutputDB:  dbPath,
		Source:    "ha",
		HAURL:     server.URL,
		HAToken:   "test-token",
		HADevices: "device_tracker.iphone",
		HAStart:   "2023-10-01T00:00:00Z",
		HAEnd:     "invalid",
	})
	if err == nil {
		t.Fatal("expected error for invalid ha-end format, got nil")
	}
	if !strings.Contains(err.Error(), "invalid ha-end") {
		t.Errorf("error should mention invalid ha-end, got: %v", err)
	}
}

// TestBuildDB_UpsertSemantics tests that running BuildDB twice with overlapping
// data updates existing entries rather than creating duplicates.
func TestBuildDB_UpsertSemantics(t *testing.T) {
	if !exiftoolAvailable() {
		t.Skip("exiftool binary not found in PATH, skipping integration test")
	}
	imagesDir := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "upsert.db")

	// Create first image
	imgTime := time.Date(2023, 10, 1, 12, 0, 0, 0, time.UTC)
	createImageWithGPS(t, imagesDir, "iphone1.jpg", 37.7749, -122.4194, 15.2, imgTime, "iPhone 14 Pro")

	// First build
	err := BuildDB(context.Background(), BuildConfig{
		OutputDB: dbPath,
		Source:   "images",
		InputDir: imagesDir,
	})
	if err != nil {
		t.Fatalf("first BuildDB failed: %v", err)
	}

	// Create another image with same timestamp and device (should update) and a new one
	createImageWithGPS(t, imagesDir, "iphone1_updated.jpg", 37.7755, -122.4200, 18.3, imgTime, "iPhone 14 Pro")
	img2Time := time.Date(2023, 10, 1, 13, 0, 0, 0, time.UTC)
	createImageWithGPS(t, imagesDir, "iphone2.jpg", 37.7760, -122.4210, 22.1, img2Time, "iPhone 14 Pro")

	// Second build (should upsert)
	err = BuildDB(context.Background(), BuildConfig{
		OutputDB: dbPath,
		Source:   "images",
		InputDir: imagesDir,
	})
	if err != nil {
		t.Fatalf("second BuildDB failed: %v", err)
	}

	// Verify database: should have 2 unique timestamps, with first timestamp having updated location
	repo, err := database.Connect(dbPath)
	if err != nil {
		t.Fatalf("connect DB: %v", err)
	}
	defer repo.Close()
	entries, err := repo.GetAll(context.Background())
	if err != nil {
		t.Fatalf("get all entries: %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("expected 2 unique entries after upsert, got %d", len(entries))
	}

	// Find entry for imgTime
	var firstEntry *database.LocationEntry
	for i := range entries {
		e := entries[i]
		if e.Timestamp.Equal(imgTime) && e.DeviceModel == "iPhone 14 Pro" {
			firstEntry = &e
			break
		}
	}
	if firstEntry == nil {
		t.Fatal("missing entry for first timestamp after upsert")
	}
	// Should reflect updated coordinates from iphone1_updated.jpg
	if firstEntry.Latitude != 37.7755 || firstEntry.Longitude != -122.4200 {
		t.Errorf("after upsert, location should be updated: got (%f, %f), want (37.7755, -122.4200)",
			firstEntry.Latitude, firstEntry.Longitude)
	}
	if *firstEntry.Altitude != 18.3 {
		t.Errorf("after upsert, altitude should be updated: got %f, want 18.3", *firstEntry.Altitude)
	}
}

// TestTagImages_ScriptGeneration_EdgeCases tests script generation with filenames containing
// special characters, spaces, and UTF-8 to ensure proper shell escaping.
func TestTagImages_ScriptGeneration_EdgeCases(t *testing.T) {
	if !exiftoolAvailable() {
		t.Skip("exiftool binary not found")
	}
	// Build DB with a known location
	imagesDir := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "edge.db")
	imgTime := time.Date(2023, 10, 1, 12, 0, 0, 0, time.UTC)
	createImageWithGPS(t, imagesDir, "ref.jpg", 37.7749, -122.4194, 10.0, imgTime, "TestCam")
	err := BuildDB(context.Background(), BuildConfig{
		OutputDB: dbPath,
		Source:   "images",
		InputDir: imagesDir,
	})
	if err != nil {
		t.Fatalf("BuildDB failed: %v", err)
	}

	rawDir := t.TempDir()
	// Complex filename with spaces, dollar sign, backtick, single quote, double quote
	complexName := "my photo with spaces $dollar `backtick' and \"quote\".jpg"
	createRawImage(t, rawDir, complexName, imgTime.Add(30*time.Minute))
	// UTF-8 filename
	utfName := "世界地图.jpg"
	createRawImage(t, rawDir, utfName, imgTime.Add(31*time.Minute))

	// Generate script
	scriptPath := filepath.Join(t.TempDir(), "edge_script.sh")
	writer, err := NewFileScriptWriter(scriptPath)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()

	err = TagImages(context.Background(), rawDir, dbPath, false, nil, matcher.ProviderOptions{
		SearchWindow:       matcher.DefaultSearchWindow,
		TimeThreshold:      matcher.DefaultTimeThreshold,
		PriorityMultiplier: matcher.DefaultPriorityMultiplier,
	}, writer)
	if err != nil {
		t.Fatalf("TagImages: %v", err)
	}

	content, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatal(err)
	}
	script := string(content)

	// Verify complex filename appears escaped
	complexFullPath := filepath.Join(rawDir, complexName)
	if !strings.Contains(script, complexFullPath) {
		t.Errorf("script missing complex filename %s", complexFullPath)
	}
	// Check that internal single quote is escaped as '\'' in the script
	escapedSingleQuote := "'\\''"
	if !strings.Contains(script, escapedSingleQuote) {
		// The filename contains a single quote, so the escaped version should appear
		t.Error("script does not contain escaped single quote pattern, check escaping")
	}

	// Verify UTF-8 filename appears
	utfFullPath := filepath.Join(rawDir, utfName)
	if !strings.Contains(script, utfFullPath) {
		t.Errorf("script missing UTF-8 filename %s", utfFullPath)
	}

	// Footer
	if !strings.Contains(script, "# Total:") {
		t.Error("missing footer")
	}
}

// TestTagImages_ScriptGeneration tests that script generation produces a valid bash script
// with proper comments and exiftool commands, and correctly handles skip scenarios.
func TestTagImages_ScriptGeneration(t *testing.T) {
	if !exiftoolAvailable() {
		t.Skip("exiftool binary not found")
	}
	// Setup: create a DB with a single location entry.
	imagesDir := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "script.db")
	imgTime := time.Date(2023, 10, 1, 12, 0, 0, 0, time.UTC)
	createImageWithGPS(t, imagesDir, "ref.jpg", 37.7749, -122.4194, 10.0, imgTime, "TestCam")
	err := BuildDB(context.Background(), BuildConfig{
		OutputDB: dbPath,
		Source:   "images",
		InputDir: imagesDir,
	})
	if err != nil {
		t.Fatalf("BuildDB failed: %v", err)
	}

	// Prepare raw images directory
	rawDir := t.TempDir()
	// 1. Image that should be tagged
	rawTime := imgTime.Add(30 * time.Minute)
	createRawImage(t, rawDir, "tagme.jpg", rawTime)
	// 2. Image that already has GPS (skip)
	createImageWithGPS(t, rawDir, "skip_gps.jpg", 0, 0, 0, rawTime, "Other")
	// 3. Image with timestamp too far (skip)
	farTime := imgTime.Add(48 * time.Hour)
	createRawImage(t, rawDir, "skip_nomatch.jpg", farTime)

	// Script output
	scriptPath := filepath.Join(t.TempDir(), "script.sh")
	writer, err := NewFileScriptWriter(scriptPath)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()

	// TagImages with scriptWriter
	err = TagImages(context.Background(), rawDir, dbPath, false, nil, matcher.ProviderOptions{
		SearchWindow:       matcher.DefaultSearchWindow,
		TimeThreshold:      matcher.DefaultTimeThreshold,
		PriorityMultiplier: matcher.DefaultPriorityMultiplier,
	}, writer)
	if err != nil {
		t.Fatalf("TagImages error: %v", err)
	}

	// Verify script contents
	content, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatal(err)
	}
	script := string(content)

	// Shebang
	if !strings.HasPrefix(script, "#!/usr/bin/env bash") {
		t.Error("script missing shebang")
	}
	// Header comments
	if !strings.Contains(script, "# Generated by exif-geotagger") {
		t.Error("missing generation header")
	}
	// Command for tagme.jpg
	if !strings.Contains(script, "tagme.jpg") {
		t.Error("script missing tagged file")
	}
	if !strings.Contains(script, "-GPSLatitude=37.7749") {
		t.Error("missing latitude in command")
	}
	if !strings.Contains(script, "-GPSLongitude=-122.4194") {
		t.Error("missing longitude in command")
	}
	// Skip comment for skip_gps.jpg (full path will include directories)
	if !strings.Contains(script, "skip_gps.jpg") || !strings.Contains(script, "already has GPS data") {
		t.Error("missing skip comment for already has GPS")
	}
	// Skip comment for skip_nomatch.jpg
	if !strings.Contains(script, "skip_nomatch.jpg") || !strings.Contains(script, "no matching location") {
		t.Error("missing skip comment for no match")
	}
	// Footer
	if !strings.Contains(script, "# Total:") {
		t.Error("missing footer with totals")
	}
}

// TestTagImages_ScriptGeneration_NewlineInFilename tests that when a file path contains
// newline characters, the skip comments in the generated script have the newlines
// sanitized to spaces, ensuring the script remains valid.
func TestTagImages_ScriptGeneration_NewlineInFilename(t *testing.T) {
	if !exiftoolAvailable() {
		t.Skip("exiftool binary not found")
	}
	// Build DB with a single location
	imagesDir := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "newline.db")
	imgTime := time.Date(2023, 10, 1, 12, 0, 0, 0, time.UTC)
	createImageWithGPS(t, imagesDir, "ref.jpg", 37.7749, -122.4194, 10.0, imgTime, "TestCam")
	err := BuildDB(context.Background(), BuildConfig{
		OutputDB: dbPath,
		Source:   "images",
		InputDir: imagesDir,
	})
	if err != nil {
		t.Fatalf("BuildDB failed: %v", err)
	}

	rawDir := t.TempDir()
	// Create a raw image with newline in the filename that already has GPS (will be skipped)
	rawTime := imgTime.Add(30 * time.Minute)
	filenameWithNewline := "photo\nwith\nnewlines.jpg"
	// Create raw image, then add GPS and timestamp
	path := filepath.Join(rawDir, filenameWithNewline)
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("failed to create image: %v", err)
	}
	f.Close()
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	for x := 0; x < 10; x++ {
		for y := 0; y < 10; y++ {
			img.Set(x, y, color.RGBA{255, 0, 0, 255})
		}
	}
	f, err = os.Create(path)
	if err != nil {
		t.Fatalf("failed to create image: %v", err)
	}
	defer f.Close()
	if err := jpeg.Encode(f, img, nil); err != nil {
		t.Fatalf("failed to encode JPEG: %v", err)
	}
	// Set DateTimeOriginal
	dtStr := rawTime.Format("2006:01:02 15:04:05")
	cmd := exec.Command("exiftool", "-DateTimeOriginal="+dtStr, "-overwrite_original", path)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("exiftool set DateTime failed: %s, %v", out, err)
	}
	// Set GPS so it's considered as already having GPS
	latRef := "N"
	lonRef := "E"
	cmd = exec.Command("exiftool",
		"-GPSLatitude=37.7749",
		"-GPSLongitude=-122.4194",
		"-GPSLatitudeRef="+latRef,
		"-GPSLongitudeRef="+lonRef,
		"-overwrite_original",
		path,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("exiftool set GPS failed: %s, %v", out, err)
	}

	// Generate script
	scriptPath := filepath.Join(t.TempDir(), "newline_script.sh")
	writer, err := NewFileScriptWriter(scriptPath)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()

	err = TagImages(context.Background(), rawDir, dbPath, false, nil, matcher.ProviderOptions{
		SearchWindow:       matcher.DefaultSearchWindow,
		TimeThreshold:      matcher.DefaultTimeThreshold,
		PriorityMultiplier: matcher.DefaultPriorityMultiplier,
	}, writer)
	if err != nil {
		t.Fatalf("TagImages error: %v", err)
	}

	content, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatal(err)
	}
	script := string(content)

	// The file should be skipped with a comment. The path contains newlines, which should be sanitized to spaces.
	// The skip comment line should contain "SKIP:" and the sanitized path (with spaces instead of newlines).
	// Original filename with newlines: "photo\nwith\nnewlines.jpg"
	// After sanitization: "photo with newlines.jpg"
	sanitized := "photo with newlines.jpg"
	if !strings.Contains(script, "# SKIP: "+sanitized) {
		t.Errorf("script missing sanitized skip comment for filename with newlines.\nScript:\n%s", script)
	}
	// Ensure the raw newline does not appear as a literal newline in the script line (i.e., no extra line break within comment).
	lines := strings.Split(script, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "# SKIP:") {
			if strings.Contains(line, "\n") || strings.Contains(line, "\r") {
				t.Errorf("skip comment contains newline character: %q", line)
			}
		}
	}
}
