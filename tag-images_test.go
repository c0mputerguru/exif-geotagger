package main

import (
	"strings"
	"testing"
	"time"
)

func TestParseTagImagesArgs_RequiredRawDir(t *testing.T) {
	args := []string{
		"-raw-dir", "/path/to/raw",
	}
	cfg, err := parseTagImagesArgs(args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.RawDir != "/path/to/raw" {
		t.Errorf("expected RawDir '/path/to/raw', got '%s'", cfg.RawDir)
	}
	if cfg.DBPath != "db.sqlite" {
		t.Errorf("expected default DBPath 'db.sqlite', got '%s'", cfg.DBPath)
	}
	if cfg.DryRun != false {
		t.Errorf("expected default DryRun false, got %v", cfg.DryRun)
	}
	if cfg.PriorityDevices != nil {
		t.Errorf("expected nil PriorityDevices, got %v", cfg.PriorityDevices)
	}
	if cfg.SearchWindow != 12*time.Hour {
		t.Errorf("expected default SearchWindow 12h, got %v", cfg.SearchWindow)
	}
	if cfg.TimeThreshold != 6*time.Hour {
		t.Errorf("expected default TimeThreshold 6h, got %v", cfg.TimeThreshold)
	}
	if cfg.PriorityMultiplier != 5.0 {
		t.Errorf("expected default PriorityMultiplier 5.0, got %f", cfg.PriorityMultiplier)
	}
}

func TestParseTagImagesArgs_AllFlags(t *testing.T) {
	args := []string{
		"-raw-dir", "/path/to/raw",
		"-db", "custom.db",
		"-dry-run",
		"-priority-devices", "iPhone 14 Pro, Pixel 8, Nikon D850",
		"-search-window", "24h",
		"-time-threshold", "12h",
		"-priority-multiplier", "10.5",
	}
	cfg, err := parseTagImagesArgs(args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.RawDir != "/path/to/raw" {
		t.Errorf("expected RawDir '/path/to/raw', got '%s'", cfg.RawDir)
	}
	if cfg.DBPath != "custom.db" {
		t.Errorf("expected DBPath 'custom.db', got '%s'", cfg.DBPath)
	}
	if cfg.DryRun != true {
		t.Errorf("expected DryRun true, got %v", cfg.DryRun)
	}
	if len(cfg.PriorityDevices) != 3 {
		t.Fatalf("expected 3 priority devices, got %d", len(cfg.PriorityDevices))
	}
	expected := []string{"iPhone 14 Pro", "Pixel 8", "Nikon D850"}
	for i, exp := range expected {
		if cfg.PriorityDevices[i] != exp {
			t.Errorf("PriorityDevices[%d]: expected '%s', got '%s'", i, exp, cfg.PriorityDevices[i])
		}
	}
	if cfg.SearchWindow != 24*time.Hour {
		t.Errorf("expected SearchWindow 24h, got %v", cfg.SearchWindow)
	}
	if cfg.TimeThreshold != 12*time.Hour {
		t.Errorf("expected TimeThreshold 12h, got %v", cfg.TimeThreshold)
	}
	if cfg.PriorityMultiplier != 10.5 {
		t.Errorf("expected PriorityMultiplier 10.5, got %f", cfg.PriorityMultiplier)
	}
}

func TestParseTagImagesArgs_MissingRawDir(t *testing.T) {
	args := []string{
		"-db", "test.db",
	}
	cfg, err := parseTagImagesArgs(args)
	if cfg != nil || err == nil {
		t.Fatal("expected error for missing -raw-dir, got nil")
	}
	expectedErr := "-raw-dir directory is required"
	if err.Error() != expectedErr {
		t.Errorf("expected error '%s', got '%s'", expectedErr, err.Error())
	}
}

func TestParseTagImagesArgs_InvalidSearchWindow(t *testing.T) {
	args := []string{
		"-raw-dir", "/path/to/raw",
		"-search-window", "invalid",
	}
	cfg, err := parseTagImagesArgs(args)
	if cfg != nil || err == nil {
		t.Fatal("expected error for invalid search-window, got nil")
	}
	if !strings.Contains(err.Error(), "invalid search-window duration") {
		t.Errorf("expected error containing 'invalid search-window duration', got '%s'", err.Error())
	}
}

func TestParseTagImagesArgs_InvalidTimeThreshold(t *testing.T) {
	args := []string{
		"-raw-dir", "/path/to/raw",
		"-time-threshold", "invalid",
	}
	cfg, err := parseTagImagesArgs(args)
	if cfg != nil || err == nil {
		t.Fatal("expected error for invalid time-threshold, got nil")
	}
	if !strings.Contains(err.Error(), "invalid time-threshold duration") {
		t.Errorf("expected error containing 'invalid time-threshold duration', got '%s'", err.Error())
	}
}

func TestParseTagImagesArgs_PriorityDevices_EmptyAndSpaces(t *testing.T) {
	args := []string{
		"-raw-dir", "/path/to/raw",
		"-priority-devices", ", , iPhone  , , Pixel,",
	}
	cfg, err := parseTagImagesArgs(args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.PriorityDevices) != 2 {
		t.Fatalf("expected 2 valid devices after trimming, got %d", len(cfg.PriorityDevices))
	}
	if cfg.PriorityDevices[0] != "iPhone" {
		t.Errorf("expected first device 'iPhone', got '%s'", cfg.PriorityDevices[0])
	}
	if cfg.PriorityDevices[1] != "Pixel" {
		t.Errorf("expected second device 'Pixel', got '%s'", cfg.PriorityDevices[1])
	}
}

func TestParseTagImagesArgs_DurationFormats(t *testing.T) {
	testCases := []struct {
		name     string
		arg      string
		expected time.Duration
	}{
		{"hours", "12h", 12 * time.Hour},
		{"minutes", "30m", 30 * time.Minute},
		{"seconds", "45s", 45 * time.Second},
		{"complex", "1h30m", 90 * time.Minute},
		{"milliseconds", "500ms", 500 * time.Millisecond},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			args := []string{
				"-raw-dir", "/path/to/raw",
				"-search-window", tc.arg,
				"-time-threshold", tc.arg,
			}
			cfg, err := parseTagImagesArgs(args)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg.SearchWindow != tc.expected {
				t.Errorf("expected SearchWindow %v, got %v", tc.expected, cfg.SearchWindow)
			}
			if cfg.TimeThreshold != tc.expected {
				t.Errorf("expected TimeThreshold %v, got %v", tc.expected, cfg.TimeThreshold)
			}
		})
	}
}

func TestParseTagImagesArgs_PriorityMultiplierEdgeCases(t *testing.T) {
	testCases := []struct {
		name     string
		arg      string
		expected float64
	}{
		{"integer", "10", 10.0},
		{"float", "2.5", 2.5},
		{"negative", "-1.0", -1.0},
		{"zero", "0", 0.0},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			args := []string{
				"-raw-dir", "/path/to/raw",
				"-priority-multiplier", tc.arg,
			}
			cfg, err := parseTagImagesArgs(args)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg.PriorityMultiplier != tc.expected {
				t.Errorf("expected PriorityMultiplier %f, got %f", tc.expected, cfg.PriorityMultiplier)
			}
		})
	}
}

func TestParseTagImagesArgs_DefaultValues(t *testing.T) {
	args := []string{
		"-raw-dir", "/path/to/raw",
	}
	cfg, err := parseTagImagesArgs(args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Verify defaults from tag-images.go
	if cfg.DBPath != "db.sqlite" {
		t.Errorf("expected default DBPath 'db.sqlite', got '%s'", cfg.DBPath)
	}
	if cfg.DryRun != false {
		t.Errorf("expected default DryRun false, got %v", cfg.DryRun)
	}
	if len(cfg.PriorityDevices) != 0 {
		t.Errorf("expected empty PriorityDevices, got %v", cfg.PriorityDevices)
	}
	if cfg.SearchWindow != 12*time.Hour {
		t.Errorf("expected default SearchWindow 12h, got %v", cfg.SearchWindow)
	}
	if cfg.TimeThreshold != 6*time.Hour {
		t.Errorf("expected default TimeThreshold 6h, got %v", cfg.TimeThreshold)
	}
	if cfg.PriorityMultiplier != 5.0 {
		t.Errorf("expected default PriorityMultiplier 5.0, got %f", cfg.PriorityMultiplier)
	}
}
