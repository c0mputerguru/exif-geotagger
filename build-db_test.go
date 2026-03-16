package main

import (
	"testing"
)

func TestParseBuildDBArgs_ImagesSource(t *testing.T) {
	args := []string{
		"-input", "/path/to/images",
		"-output", "test.db",
		"-source", "images",
	}
	cfg, err := parseBuildDBArgs(args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Source != "images" {
		t.Errorf("expected source 'images', got '%s'", cfg.Source)
	}
	if cfg.InputDir != "/path/to/images" {
		t.Errorf("expected InputDir '/path/to/images', got '%s'", cfg.InputDir)
	}
	if cfg.OutputDB != "test.db" {
		t.Errorf("expected OutputDB 'test.db', got '%s'", cfg.OutputDB)
	}
	if cfg.FilterModels != nil {
		t.Errorf("expected nil FilterModels, got %v", cfg.FilterModels)
	}
}

func TestParseBuildDBArgs_ImagesSource_WithModels(t *testing.T) {
	args := []string{
		"-input", "/path/to/images",
		"-output", "test.db",
		"-source", "images",
		"-models", "iPhone 14 Pro, Pixel 8, Nikon D850",
	}
	cfg, err := parseBuildDBArgs(args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.FilterModels) != 3 {
		t.Fatalf("expected 3 filter models, got %d", len(cfg.FilterModels))
	}
	expected := []string{"iPhone 14 Pro", "Pixel 8", "Nikon D850"}
	for i, exp := range expected {
		if cfg.FilterModels[i] != exp {
			t.Errorf("FilterModels[%d]: expected '%s', got '%s'", i, exp, cfg.FilterModels[i])
		}
	}
}

func TestParseBuildDBArgs_ImagesSource_WithAll(t *testing.T) {
	args := []string{
		"-input", "/path/to/images",
		"-output", "test.db",
		"-source", "images",
		"-all",
	}
	cfg, err := parseBuildDBArgs(args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.FilterModels != nil {
		t.Errorf("expected nil FilterModels with -all, got %v", cfg.FilterModels)
	}
}

func TestParseBuildDBArgs_ImagesSource_MissingInput(t *testing.T) {
	args := []string{
		"-output", "test.db",
		"-source", "images",
	}
	cfg, err := parseBuildDBArgs(args)
	if cfg != nil || err == nil {
		t.Fatal("expected error for missing -input, got nil")
	}
	expectedErr := "-input directory is required when source is 'images'"
	if err.Error() != expectedErr {
		t.Errorf("expected error '%s', got '%s'", expectedErr, err.Error())
	}
}

func TestParseBuildDBArgs_ImagesSource_DefaultSource(t *testing.T) {
	args := []string{
		"-input", "/path/to/images",
		"-output", "test.db",
	}
	cfg, err := parseBuildDBArgs(args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Source != "images" {
		t.Errorf("expected default source 'images', got '%s'", cfg.Source)
	}
	if cfg.InputDir != "/path/to/images" {
		t.Errorf("expected InputDir '/path/to/images', got '%s'", cfg.InputDir)
	}
}

func TestParseBuildDBArgs_InvalidSource(t *testing.T) {
	args := []string{
		"-input", "/path/to/images",
		"-output", "test.db",
		"-source", "invalid",
	}
	cfg, err := parseBuildDBArgs(args)
	if cfg != nil || err == nil {
		t.Fatal("expected error for invalid source, got nil")
	}
	expectedErr := "invalid source 'invalid'. Must be 'images' or 'ha'"
	if err.Error() != expectedErr {
		t.Errorf("expected error '%s', got '%s'", expectedErr, err.Error())
	}
}

func TestParseBuildDBArgs_HASource(t *testing.T) {
	args := []string{
		"-source", "ha",
		"-ha-url", "http://homeassistant.local:8123",
		"-ha-token", "abc123token",
		"-output", "test.db",
	}
	cfg, err := parseBuildDBArgs(args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Source != "ha" {
		t.Errorf("expected source 'ha', got '%s'", cfg.Source)
	}
	if cfg.HAURL != "http://homeassistant.local:8123" {
		t.Errorf("expected HAURL '%s', got '%s'", "http://homeassistant.local:8123", cfg.HAURL)
	}
	if cfg.HAToken != "abc123token" {
		t.Errorf("expected HAToken '%s', got '%s'", "abc123token", cfg.HAToken)
	}
}

func TestParseBuildDBArgs_HASource_MissingURL(t *testing.T) {
	args := []string{
		"-source", "ha",
		"-ha-token", "abc123token",
		"-output", "test.db",
	}
	cfg, err := parseBuildDBArgs(args)
	if cfg != nil || err == nil {
		t.Fatal("expected error for missing -ha-url, got nil")
	}
	expectedErr := "-ha-url is required for HA source"
	if err.Error() != expectedErr {
		t.Errorf("expected error '%s', got '%s'", expectedErr, err.Error())
	}
}

func TestParseBuildDBArgs_HASource_MissingToken(t *testing.T) {
	args := []string{
		"-source", "ha",
		"-ha-url", "http://homeassistant.local:8123",
		"-output", "test.db",
	}
	cfg, err := parseBuildDBArgs(args)
	if cfg != nil || err == nil {
		t.Fatal("expected error for missing -ha-token, got nil")
	}
	expectedErr := "-ha-token is required for HA source"
	if err.Error() != expectedErr {
		t.Errorf("expected error '%s', got '%s'", expectedErr, err.Error())
	}
}

func TestParseBuildDBArgs_HASource_WithOptionalFlags(t *testing.T) {
	args := []string{
		"-source", "ha",
		"-ha-url", "http://homeassistant.local:8123",
		"-ha-token", "abc123token",
		"-output", "test.db",
		"-ha-devices", "device_tracker.iphone, device_tracker.pixel",
		"-ha-start", "2023-10-01T00:00:00Z",
		"-ha-end", "2023-10-02T00:00:00Z",
		"-ha-days", "7",
		"-all",
	}
	cfg, err := parseBuildDBArgs(args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.HADevices != "device_tracker.iphone, device_tracker.pixel" {
		t.Errorf("expected HADevices 'device_tracker.iphone, device_tracker.pixel', got '%s'", cfg.HADevices)
	}
	if cfg.HAStart != "2023-10-01T00:00:00Z" {
		t.Errorf("expected HAStart '2023-10-01T00:00:00Z', got '%s'", cfg.HAStart)
	}
	if cfg.HAEnd != "2023-10-02T00:00:00Z" {
		t.Errorf("expected HAEnd '2023-10-02T00:00:00Z', got '%s'", cfg.HAEnd)
	}
	if cfg.HADays != 7 {
		t.Errorf("expected HADays 7, got %d", cfg.HADays)
	}
	if !cfg.HAAll {
		t.Error("expected HAAll to be true")
	}
}

func TestParseBuildDBArgs_Models_EmptyAndSpaces(t *testing.T) {
	args := []string{
		"-input", "/path",
		"-output", "test.db",
		"-source", "images",
		"-models", ", , iPhone  , , Pixel,",
	}
	cfg, err := parseBuildDBArgs(args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.FilterModels) != 2 {
		t.Fatalf("expected 2 valid models after trimming, got %d", len(cfg.FilterModels))
	}
	if cfg.FilterModels[0] != "iPhone" {
		t.Errorf("expected first model 'iPhone', got '%s'", cfg.FilterModels[0])
	}
	if cfg.FilterModels[1] != "Pixel" {
		t.Errorf("expected second model 'Pixel', got '%s'", cfg.FilterModels[1])
	}
}
