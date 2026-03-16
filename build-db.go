package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/abpatel/exif-geotagger/pkg/logger"
	"github.com/abpatel/exif-geotagger/pkg/processor"
)

// parseBuildDBArgs parses command line arguments and returns a BuildConfig.
// It handles all flag definitions, parsing, and validation.
func parseBuildDBArgs(args []string) (*processor.BuildConfig, error) {
	buildDBCmd := flag.NewFlagSet("build-db", flag.ExitOnError)
	inputDir := buildDBCmd.String("input", "", "Directory containing reference images with GPS data")
	outputDB := buildDBCmd.String("output", "db.sqlite", "Path to output SQLite database")
	source := buildDBCmd.String("source", "images", "Data source: 'images' or 'ha'")
	haURL := buildDBCmd.String("ha-url", "", "Home Assistant URL")
	haToken := buildDBCmd.String("ha-token", "", "Home Assistant long-lived access token")
	haDevices := buildDBCmd.String("ha-devices", "", "Comma-separated list of device entity IDs (for HA source)")
	haStart := buildDBCmd.String("ha-start", "", "Start time for HA history (RFC3339)")
	haEnd := buildDBCmd.String("ha-end", "", "End time for HA history (RFC3339)")
	haDays := buildDBCmd.Int("ha-days", 0, "Number of days of history (alternative to start/end)")
	all := buildDBCmd.Bool("all", false, "Select all discovered devices (images source only)")
	models := buildDBCmd.String("models", "", "Comma-separated list of device models to include (for images source)")

	if err := buildDBCmd.Parse(args); err != nil {
		return nil, err
	}

	// Build config
	cfg := &processor.BuildConfig{
		OutputDB: *outputDB,
		Source:   *source,
		// HA config
		HAURL:     *haURL,
		HAToken:   *haToken,
		HADevices: *haDevices,
		HAStart:   *haStart,
		HAEnd:     *haEnd,
		HADays:    *haDays,
		HAAll:     *all,
	}

	// Validation
	if *source == "ha" {
		// For HA source, validation will happen in buildDBFromHA
		// But we need at least URL and token for basic validation
		if *haURL == "" {
			return nil, fmt.Errorf("-ha-url is required for HA source")
		}
		if *haToken == "" {
			return nil, fmt.Errorf("-ha-token is required for HA source")
		}
	} else if *source == "images" {
		// Images source: require -input
		if *inputDir == "" {
			return nil, fmt.Errorf("-input directory is required when source is 'images'")
		}
		cfg.InputDir = *inputDir
		if *all {
			// -all: FilterModels stays nil => include all devices
		} else if *models != "" {
			// Use -models flag to filter specific device models
			parts := strings.Split(*models, ",")
			for _, p := range parts {
				trimmed := strings.TrimSpace(p)
				if trimmed != "" {
					cfg.FilterModels = append(cfg.FilterModels, trimmed)
				}
			}
		}
		// If neither -all nor -models provided, FilterModels stays nil => all devices
	} else {
		return nil, fmt.Errorf("invalid source '%s'. Must be 'images' or 'ha'", *source)
	}

	return cfg, nil
}

func runBuildDB() {
	cfg, err := parseBuildDBArgs(os.Args[2:])
	if err != nil {
		logger.Error("Error: %v", err)
		os.Exit(1)
	}

	if err := processor.BuildDB(*cfg); err != nil {
		logger.Error("Error building database: %v", err)
		os.Exit(1)
	}
}
