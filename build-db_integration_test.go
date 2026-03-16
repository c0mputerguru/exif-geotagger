package main

import (
	"database/sql"
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

	_ "github.com/mattn/go-sqlite3"
)

func mockHAServer(t *testing.T) *httptest.Server {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/config" {
			fmt.Fprint(w, `{"timezone":"UTC"}`)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/history/period/") {
			auth := r.Header.Get("Authorization")
			if auth != "Bearer test-token" {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			resp := [][]map[string]interface{}{
				{
					{
						"entity_id":        "device_tracker.iphone",
						"state":            "home",
						"attributes":       map[string]interface{}{"latitude": 37.7749, "longitude": -122.4194, "altitude": 15.2},
						"last_updated_iso": "2023-10-01T12:00:00Z",
					},
					{
						"entity_id":        "device_tracker.iphone",
						"state":            "home",
						"attributes":       map[string]interface{}{"latitude": 37.7750, "longitude": -122.4195, "altitude": 15.5},
						"last_updated_iso": "2023-10-01T13:00:00Z",
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

func createRawImage(t *testing.T, dir string, filename string, ts time.Time) string {
	path := filepath.Join(dir, filename)
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
	dtStr := ts.Format("2006:01:02 15:04:05")
	cmd := exec.Command("exiftool", "-DateTimeOriginal="+dtStr, "-overwrite_original", path)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("exiftool failed: %s, %v", out, err)
	}
	return path
}

func createImageWithGPS(t *testing.T, dir string, filename string, lat, lon float64, alt float64, ts time.Time, model string) string {
	path := filepath.Join(dir, filename)
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

func invokeBuildDBCmd(args ...string) error {
	cmdArgs := make([]string, 0, len(args)+2)
	cmdArgs = append(cmdArgs, "./exif-geotagger", "build-db")
	cmdArgs = append(cmdArgs, args...)
	cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

type expectedEntry struct {
	Timestamp   time.Time
	Latitude    float64
	Longitude   float64
	Altitude    float64
	DeviceModel string
}

func verifyDBContents(t *testing.T, dbPath string, expected []expectedEntry) {
	repo, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("connect DB: %v", err)
	}
	defer repo.Close()

	rows, err := repo.Query("SELECT timestamp, latitude, longitude, altitude, device_model FROM locations ORDER BY timestamp, device_model")
	if err != nil {
		t.Fatalf("query DB: %v", err)
	}
	defer rows.Close()

	var entries []expectedEntry
	for rows.Next() {
		var e struct {
			Timestamp   string
			Latitude    float64
			Longitude   float64
			Altitude    sql.NullFloat64
			DeviceModel string
		}
		if err := rows.Scan(&e.Timestamp, &e.Latitude, &e.Longitude, &e.Altitude, &e.DeviceModel); err != nil {
			t.Fatalf("scan row: %v", err)
		}
		ts, _ := time.Parse(time.RFC3339, e.Timestamp)
		alt := 0.0
		if e.Altitude.Valid {
			alt = e.Altitude.Float64
		}
		entries = append(entries, expectedEntry{
			Timestamp:   ts,
			Latitude:    e.Latitude,
			Longitude:   e.Longitude,
			Altitude:    alt,
			DeviceModel: e.DeviceModel,
		})
	}

	if len(entries) != len(expected) {
		t.Fatalf("expected %d entries, got %d", len(expected), len(entries))
	}

	for i, e := range entries {
		exp := expected[i]
		if !e.Timestamp.Equal(exp.Timestamp) {
			t.Errorf("entry %d: timestamp mismatch: got %v, want %v", i, e.Timestamp, exp.Timestamp)
		}
		if e.Latitude != exp.Latitude {
			t.Errorf("entry %d: latitude mismatch: got %f, want %f", i, e.Latitude, exp.Latitude)
		}
		if e.Longitude != exp.Longitude {
			t.Errorf("entry %d: longitude mismatch: got %f, want %f", i, e.Longitude, exp.Longitude)
		}
		if e.Altitude != exp.Altitude {
			t.Errorf("entry %d: altitude mismatch: got %f, want %f", i, e.Altitude, exp.Altitude)
		}
		if e.DeviceModel != exp.DeviceModel {
			t.Errorf("entry %d: device_model mismatch: got %s, want %s", i, e.DeviceModel, exp.DeviceModel)
		}
	}
}

func TestBuildDB_ImagesSource(t *testing.T) {
	if _, err := exec.LookPath("exiftool"); err != nil {
		t.Skip("exiftool not found in PATH")
	}

	imagesDir := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "test.db")

	img1Time := time.Date(2023, 10, 1, 12, 0, 0, 0, time.UTC)
	createImageWithGPS(t, imagesDir, "iphone1.jpg", 37.7749, -122.4194, 15.2, img1Time, "iPhone 14 Pro")

	img2Time := time.Date(2023, 10, 1, 13, 0, 0, 0, time.UTC)
	createImageWithGPS(t, imagesDir, "pixel1.jpg", 37.7750, -122.4195, 20.1, img2Time, "Pixel 8")

	if err := invokeBuildDBCmd("-input", imagesDir, "-output", dbPath, "-source", "images"); err != nil {
		t.Fatalf("build-db failed: %v", err)
	}

	verifyDBContents(t, dbPath, []expectedEntry{
		{img1Time, 37.7749, -122.4194, 15.2, "iPhone 14 Pro"},
		{img2Time, 37.7750, -122.4195, 20.1, "Pixel 8"},
	})
}

func TestBuildDB_ImagesSource_WithFilter(t *testing.T) {
	if _, err := exec.LookPath("exiftool"); err != nil {
		t.Skip("exiftool not found in PATH")
	}

	imagesDir := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "filtered.db")

	img1Time := time.Date(2023, 10, 1, 12, 0, 0, 0, time.UTC)
	createImageWithGPS(t, imagesDir, "iphone.jpg", 37.7749, -122.4194, 15.2, img1Time, "iPhone 14 Pro")

	img2Time := time.Date(2023, 10, 1, 13, 0, 0, 0, time.UTC)
	createImageWithGPS(t, imagesDir, "pixel.jpg", 37.7750, -122.4195, 20.1, img2Time, "Pixel 8")

	if err := invokeBuildDBCmd("-input", imagesDir, "-output", dbPath, "-source", "images", "-models", "iPhone 14 Pro"); err != nil {
		t.Fatalf("build-db with filter failed: %v", err)
	}

	repo, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("connect DB: %v", err)
	}
	defer repo.Close()

	rows, err := repo.Query("SELECT COUNT(*) FROM locations")
	if err != nil {
		t.Fatalf("query DB: %v", err)
	}
	var count int
	if rows.Next() {
		rows.Scan(&count)
	}
	rows.Close()

	if count != 1 {
		t.Fatalf("expected 1 entry after filter, got %d", count)
	}
}

func TestBuildDB_ImagesSource_WithAllFlag(t *testing.T) {
	if _, err := exec.LookPath("exiftool"); err != nil {
		t.Skip("exiftool not found in PATH")
	}

	imagesDir := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "all.db")

	img1Time := time.Date(2023, 10, 1, 12, 0, 0, 0, time.UTC)
	createImageWithGPS(t, imagesDir, "iphone.jpg", 37.7749, -122.4194, 15.2, img1Time, "iPhone 14 Pro")

	img2Time := time.Date(2023, 10, 1, 13, 0, 0, 0, time.UTC)
	createImageWithGPS(t, imagesDir, "pixel.jpg", 37.7750, -122.4195, 20.1, img2Time, "Pixel 8")

	if err := invokeBuildDBCmd("-input", imagesDir, "-output", dbPath, "-source", "images", "-all"); err != nil {
		t.Fatalf("build-db with -all failed: %v", err)
	}

	repo, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("connect DB: %v", err)
	}
	defer repo.Close()

	rows, err := repo.Query("SELECT COUNT(*) FROM locations")
	if err != nil {
		t.Fatalf("query DB: %v", err)
	}
	var count int
	if rows.Next() {
		rows.Scan(&count)
	}
	rows.Close()

	if count != 2 {
		t.Fatalf("expected 2 entries with -all flag, got %d", count)
	}
}

func TestBuildDB_HASource(t *testing.T) {
	if _, err := exec.LookPath("exiftool"); err != nil {
		t.Skip("exiftool not found in PATH")
	}

	server := mockHAServer(t)
	defer server.Close()

	dbPath := filepath.Join(t.TempDir(), "ha.db")

	args := []string{
		"-source", "ha",
		"-ha-url", server.URL,
		"-ha-token", "test-token",
		"-ha-devices", "device_tracker.iphone",
		"-ha-start", "2023-10-01T00:00:00Z",
		"-ha-end", "2023-10-02T00:00:00Z",
		"-output", dbPath,
	}
	if err := invokeBuildDBCmd(args...); err != nil {
		t.Fatalf("build-db HA source failed: %v", err)
	}

	repo, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("connect DB: %v", err)
	}
	defer repo.Close()

	rows, err := repo.Query("SELECT timestamp, latitude, longitude, altitude, device_model FROM locations")
	if err != nil {
		t.Fatalf("query DB: %v", err)
	}
	defer rows.Close()

	var entries []expectedEntry
	for rows.Next() {
		var e struct {
			Timestamp   string
			Latitude    float64
			Longitude   float64
			Altitude    sql.NullFloat64
			DeviceModel string
		}
		if err := rows.Scan(&e.Timestamp, &e.Latitude, &e.Longitude, &e.Altitude, &e.DeviceModel); err != nil {
			t.Fatalf("scan row: %v", err)
		}
		ts, _ := time.Parse(time.RFC3339, e.Timestamp)
		alt := 0.0
		if e.Altitude.Valid {
			alt = e.Altitude.Float64
		}
		entries = append(entries, expectedEntry{
			Timestamp:   ts,
			Latitude:    e.Latitude,
			Longitude:   e.Longitude,
			Altitude:    alt,
			DeviceModel: e.DeviceModel,
		})
	}

	if len(entries) != 2 {
		t.Fatalf("expected 2 entries from HA source, got %d", len(entries))
	}

	if entries[0].DeviceModel != "device_tracker.iphone" {
		t.Errorf("expected device_model 'device_tracker.iphone', got %s", entries[0].DeviceModel)
	}
}

func TestBuildDB_MissingInputFlag(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	err := invokeBuildDBCmd("-output", dbPath, "-source", "images")
	if err == nil {
		t.Fatal("expected error for missing -input flag, got nil")
	}
}

func TestBuildDB_InvalidInputDir(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	err := invokeBuildDBCmd("-input", "/nonexistent/dir", "-output", dbPath, "-source", "images")
	if err == nil {
		t.Fatal("expected error for invalid input directory, got nil")
	}
}

func TestBuildDB_InvalidSource(t *testing.T) {
	imagesDir := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "test.db")

	err := invokeBuildDBCmd("-input", imagesDir, "-output", dbPath, "-source", "invalid")
	if err == nil {
		t.Fatal("expected error for invalid source, got nil")
	}
}

func TestBuildDB_HAMissingRequiredFlags(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	err := invokeBuildDBCmd("-source", "ha", "-ha-token", "token", "-output", dbPath)
	if err == nil {
		t.Fatal("expected error for missing -ha-url, got nil")
	}

	err = invokeBuildDBCmd("-source", "ha", "-ha-url", "http://example.com", "-output", dbPath)
	if err == nil {
		t.Fatal("expected error for missing -ha-token, got nil")
	}
}
