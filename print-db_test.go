package main

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/abpatel/exif-geotagger/pkg/database"
)

// helper creates a temporary SQLite database file and returns its path.
// If entries is non-nil, they will be inserted into the database.
func helperCreateTestDB(t *testing.T, entries []database.LocationEntry) string {
	t.Helper()

	tmpFile, err := os.CreateTemp("", "test-db-*.sqlite")
	if err != nil {
		t.Fatalf("failed to create temp db: %v", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()

	repo, err := database.Connect(tmpPath)
	if err != nil {
		t.Fatalf("failed to connect to test db: %v", err)
	}
	defer repo.Close()

	for _, entry := range entries {
		if err := repo.Insert(context.Background(), entry); err != nil {
			t.Fatalf("failed to insert test entry: %v", err)
		}
	}

	return tmpPath
}

func TestPrintDatabase_DBConnectionError(t *testing.T) {
	// Use an invalid database path that cannot be opened
	invalidPath := "/nonexistent/path/to/invalid.db"

	err := printDatabase(context.Background(), invalidPath)
	if err == nil {
		t.Fatal("expected error for invalid database path, got nil")
	}
	if !strings.Contains(err.Error(), "error connecting to database") {
		t.Errorf("expected error message to contain 'error connecting to database', got: %v", err)
	}
}

func TestPrintDatabase_EmptyDatabase(t *testing.T) {
	// Create an empty database (no entries)
	dbPath := helperCreateTestDB(t, nil)

	// Capture stdout
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	os.Stdout = w

	// Call printDatabase
	err = printDatabase(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("printDatabase returned unexpected error: %v", err)
	}

	// Close writer and restore stdout
	w.Close()
	os.Stdout = oldStdout

	// Read captured output
	outBytes, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("failed to read captured output: %v", err)
	}
	output := string(outBytes)

	// Empty database should produce null JSON (nil slice) with newline
	expected := "null\n"
	if output != expected {
		t.Errorf("expected empty JSON array with newline, got: %q", output)
	}
}

func TestPrintDatabase_SuccessfulOutput(t *testing.T) {
	// Create a database with sample entries
	now := time.Now()
	entries := []database.LocationEntry{
		{
			Timestamp:   now,
			Latitude:    37.7749,
			Longitude:   -122.4194,
			Altitude:    float64Ptr(15.0),
			City:        stringPtr("San Francisco"),
			State:       stringPtr("CA"),
			Country:     stringPtr("USA"),
			DeviceModel: "iPhone 14 Pro",
		},
		{
			Timestamp:   now.Add(-1 * time.Hour),
			Latitude:    34.0522,
			Longitude:   -118.2437,
			Altitude:    nil,
			City:        stringPtr("Los Angeles"),
			State:       stringPtr("CA"),
			Country:     stringPtr("USA"),
			DeviceModel: "Pixel 8",
		},
	}
	dbPath := helperCreateTestDB(t, entries)

	// Capture stdout
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	os.Stdout = w

	// Call printDatabase
	err = printDatabase(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("printDatabase returned unexpected error: %v", err)
	}

	// Close writer and restore stdout
	w.Close()
	os.Stdout = oldStdout

	// Read captured output
	outBytes, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("failed to read captured output: %v", err)
	}

	// Parse the JSON output
	var result []database.LocationEntry
	if err := json.Unmarshal(outBytes, &result); err != nil {
		t.Fatalf("failed to parse JSON output: %v", err)
	}

	// Verify we got both entries (ordered by timestamp ASC, so older first)
	if len(result) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(result))
	}

	// First entry should be the older one (Pixel 8)
	if result[0].DeviceModel != entries[1].DeviceModel {
		t.Errorf("first entry DeviceModel: expected %s, got %s", entries[1].DeviceModel, result[0].DeviceModel)
	}
	if result[0].Latitude != entries[1].Latitude {
		t.Errorf("first entry Latitude: expected %f, got %f", entries[1].Latitude, result[0].Latitude)
	}
	if result[0].Longitude != entries[1].Longitude {
		t.Errorf("first entry Longitude: expected %f, got %f", entries[1].Longitude, result[0].Longitude)
	}
	if result[0].Altitude != nil {
		t.Errorf("first entry Altitude should be nil, got %v", result[0].Altitude)
	}
	if (*result[0].City != *entries[1].City) || (*result[0].State != *entries[1].State) || (*result[0].Country != *entries[1].Country) {
		t.Errorf("first entry location fields mismatch")
	}

	// Second entry should be the newer one (iPhone 14 Pro)
	if result[1].DeviceModel != entries[0].DeviceModel {
		t.Errorf("second entry DeviceModel: expected %s, got %s", entries[0].DeviceModel, result[1].DeviceModel)
	}
	if result[1].Latitude != entries[0].Latitude {
		t.Errorf("second entry Latitude: expected %f, got %f", entries[0].Latitude, result[1].Latitude)
	}
	if result[1].Longitude != entries[0].Longitude {
		t.Errorf("second entry Longitude: expected %f, got %f", entries[0].Longitude, result[1].Longitude)
	}
	if result[1].Altitude == nil || *result[1].Altitude != *entries[0].Altitude {
		t.Errorf("second entry Altitude: expected %f, got %v", *entries[0].Altitude, result[1].Altitude)
	}
	if (*result[1].City != *entries[0].City) || (*result[1].State != *entries[0].State) || (*result[1].Country != *entries[0].Country) {
		t.Errorf("second entry location fields mismatch")
	}
}

// Helper functions for pointers
func stringPtr(s string) *string    { return &s }
func float64Ptr(f float64) *float64 { return &f }
