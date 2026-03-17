package database

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// TestConnect_InvalidPath tests database connection with an invalid path
func TestConnect_InvalidPath(t *testing.T) {
	// Use an invalid database path that cannot be opened
	invalidPath := "/nonexistent/path/to/invalid.db"

	_, err := Connect(invalidPath)
	if err == nil {
		t.Fatal("expected error for invalid database path, got nil")
	}
	// The error should indicate inability to open database
	if !strings.Contains(err.Error(), "sqlite3") && !strings.Contains(err.Error(), "unable to open") && !strings.Contains(err.Error(), "no such file") {
		t.Logf("error message: %v", err)
	}
}

// TestConnect_PermissionDenied tests database connection with a file we don't have read/write permissions
func TestConnect_PermissionDenied(t *testing.T) {
	// Skip on Windows as permission handling differs
	if os.PathSeparator == '\\' {
		t.Skip("Skipping permission test on Windows")
	}

	// Create a temp file with no permissions
	tmpFile, err := os.CreateTemp("", "test-db-perm-*.sqlite")
	if err != nil {
		t.Fatalf("failed to create temp db: %v", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()

	// Remove all permissions
	err = os.Chmod(tmpPath, 0000)
	if err != nil {
		t.Fatalf("failed to chmod: %v", err)
	}

	// Ensure cleanup
	defer os.Remove(tmpPath)

	_, err = Connect(tmpPath)
	if err == nil {
		t.Fatal("expected error for permission denied, got nil")
	}
	// Should indicate permission error
	if !strings.Contains(err.Error(), "permission") && !strings.Contains(err.Error(), "denied") && !strings.Contains(err.Error(), "access") {
		t.Logf("error message: %v", err)
	}
}

// TestConnect_CorruptDatabase tests connection to a corrupt SQLite database
func TestConnect_CorruptDatabase(t *testing.T) {
	// Create a temp file with garbage data (corrupt database)
	tmpFile, err := os.CreateTemp("", "test-db-corrupt-*.sqlite")
	if err != nil {
		t.Fatalf("failed to create temp db: %v", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()

	// Write corrupt data to the file
	err = os.WriteFile(tmpPath, []byte("corrupt garbage data not a database"), 0600)
	if err != nil {
		t.Fatalf("failed to write corrupt data: %v", err)
	}

	// Ensure cleanup
	defer os.Remove(tmpPath)

	_, err = Connect(tmpPath)
	if err == nil {
		t.Fatal("expected error for corrupt database, got nil")
	}
	// Should indicate file is not a database or malformed
	if !strings.Contains(err.Error(), "malformed") && !strings.Contains(err.Error(), "database") && !strings.Contains(err.Error(), "file is not a database") {
		t.Logf("error message: %v", err)
	}
}

// TestInsert_ClosedDatabase tests insert on a closed repository
func TestInsert_ClosedDatabase(t *testing.T) {
	// Create a valid in-memory database by connecting to a temp file
	tmpFile, err := os.CreateTemp("", "test-db-closed-*.sqlite")
	if err != nil {
		t.Fatalf("failed to create temp db: %v", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	repo, err := Connect(tmpPath)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}

	// Close the repository
	err = repo.Close()
	if err != nil {
		t.Fatalf("failed to close repo: %v", err)
	}

	// Try to insert after closing
	entry := LocationEntry{
		Timestamp:   time.Now(),
		Latitude:    37.7749,
		Longitude:   -122.4194,
		DeviceModel: "Test",
	}
	err = repo.Insert(context.Background(), entry)
	if err == nil {
		t.Fatal("expected error for insert on closed database, got nil")
	}
	// Should indicate database is closed
	if !strings.Contains(err.Error(), "closed") && !strings.Contains(err.Error(), "already closed") {
		t.Logf("error message: %v", err)
	}
}

// TestGetAll_DatabaseClosed tests GetAll on a closed database
func TestGetAll_DatabaseClosed(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "test-db-getall-closed-*.sqlite")
	if err != nil {
		t.Fatalf("failed to create temp db: %v", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	repo, err := Connect(tmpPath)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}

	err = repo.Close()
	if err != nil {
		t.Fatalf("failed to close repo: %v", err)
	}

	_, err = repo.GetAll(context.Background())
	if err == nil {
		t.Fatal("expected error for GetAll on closed database, got nil")
	}
}

// TestFindClosest_DatabaseClosed tests FindClosest on a closed database
func TestFindClosest_DatabaseClosed(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "test-db-find-closed-*.sqlite")
	if err != nil {
		t.Fatalf("failed to create temp db: %v", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	repo, err := Connect(tmpPath)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}

	err = repo.Close()
	if err != nil {
		t.Fatalf("failed to close repo: %v", err)
	}

	target := time.Now()
	window := time.Hour
	_, err = repo.FindClosest(context.Background(), target, window)
	if err == nil {
		t.Fatal("expected error for FindClosest on closed database, got nil")
	}
}

// TestInsert_ContextCancelled tests insert with cancelled context
func TestInsert_ContextCancelled(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "test-db-ctx-*.sqlite")
	if err != nil {
		t.Fatalf("failed to create temp db: %v", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	repo, err := Connect(tmpPath)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer repo.Close()

	// Create a cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	entry := LocationEntry{
		Timestamp:   time.Now(),
		Latitude:    37.7749,
		Longitude:   -122.4194,
		DeviceModel: "Test",
	}
	err = repo.Insert(ctx, entry)
	// Should fail because context is cancelled
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
	if !strings.Contains(err.Error(), "canceled") && !strings.Contains(err.Error(), "cancelling") {
		t.Logf("error message: %v", err)
	}
}

// TestFindClosest_InvalidQuery tests FindClosest with a bad query (simulated by corrupting schema)
func TestFindClosest_InvalidQuery(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "test-db-invalid-query-*.sqlite")
	if err != nil {
		t.Fatalf("failed to create temp db: %v", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	repo, err := Connect(tmpPath)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer repo.Close()

	// Corrupt the schema by dropping the table (using raw SQL)
	_, err = repo.db.Exec("DROP TABLE locations")
	if err != nil {
		t.Fatalf("failed to drop table: %v", err)
	}

	target := time.Now()
	window := time.Hour
	_, err = repo.FindClosest(context.Background(), target, window)
	if err == nil {
		t.Fatal("expected error for query on missing table, got nil")
	}
}

// TestGetAll_InvalidQuery tests GetAll with a bad query (missing table)
func TestGetAll_InvalidQuery(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "test-db-getall-invalid-*.sqlite")
	if err != nil {
		t.Fatalf("failed to create temp db: %v", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	repo, err := Connect(tmpPath)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer repo.Close()

	_, err = repo.GetAll(context.Background())
	if err == nil {
		t.Fatal("expected error for query on empty/missing table, got nil")
	}
}

// TestInsert_ConstraintViolation tests insert with constraint violation (duplicate timestamp+device_model)
func TestInsert_ConstraintViolation(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "test-db-constraint-*.sqlite")
	if err != nil {
		t.Fatalf("failed to create temp db: %v", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	repo, err := Connect(tmpPath)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer repo.Close()

	now := time.Now()
	entry := LocationEntry{
		Timestamp:   now,
		Latitude:    37.7749,
		Longitude:   -122.4194,
		DeviceModel: "Test",
	}

	// First insert should succeed
	err = repo.Insert(context.Background(), entry)
	if err != nil {
		t.Fatalf("first insert failed: %v", err)
	}

	// Second insert with same timestamp and device model should fail (UNIQUE constraint violation)
	err = repo.Insert(context.Background(), entry)
	if err == nil {
		t.Fatal("expected error for duplicate entry, got nil")
	}
	// Should indicate constraint violation
	if !strings.Contains(err.Error(), "UNIQUE") && !strings.Contains(err.Error(), "constraint") && !strings.Contains(err.Error(), "duplicate") {
		t.Logf("error message: %v", err)
	}
}

// TestInsert_NilAltitude tests that nil altitude is handled correctly
func TestInsert_NilAltitude(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "test-db-nil-alt-*.sqlite")
	if err != nil {
		t.Fatalf("failed to create temp db: %v", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	repo, err := Connect(tmpPath)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer repo.Close()

	entry := LocationEntry{
		Timestamp:   time.Now(),
		Latitude:    37.7749,
		Longitude:   -122.4194,
		Altitude:    nil,
		City:        nil,
		State:       nil,
		Country:     nil,
		DeviceModel: "Test",
	}

	err = repo.Insert(context.Background(), entry)
	if err != nil {
		t.Fatalf("insert with nil altitude failed: %v", err)
	}

	entries, err := repo.GetAll(context.Background())
	if err != nil {
		t.Fatalf("getAll failed: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Altitude != nil {
		t.Errorf("expected nil altitude, got %v", entries[0].Altitude)
	}
}

// TestClose_CloseTwice tests that calling Close twice does not panic
func TestClose_CloseTwice(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "test-db-close-twice-*.sqlite")
	if err != nil {
		t.Fatalf("failed to create temp db: %v", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	repo, err := Connect(tmpPath)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}

	err = repo.Close()
	if err != nil {
		t.Fatalf("first close failed: %v", err)
	}

	// Second close should not panic, may return an error but that's also acceptable
	err = repo.Close()
	// Some implementations might return error on second close, some might not
	// Just ensure it doesn't panic
}
