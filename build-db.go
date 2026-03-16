package main

import (
	"flag"
	"os"
	"strings"

	"github.com/abpatel/exif-geotagger/pkg/logger"
	"github.com/abpatel/exif-geotagger/pkg/processor"
)

func runBuildDB() {
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

	buildDBCmd.Parse(os.Args[2:])

	// Backward compatibility: if -source is not provided or is "images", require -input
	if *source == "images" || *source == "" {
		if *inputDir == "" {
			logger.Error("Error: -input directory is required when source is 'images'")
			buildDBCmd.Usage()
			os.Exit(1)
		}
	}

	// Build config
	cfg := processor.BuildConfig{
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

	if *source == "ha" {
		// For HA source, validation happens in buildDBFromHA
		// (requires HAURL and HAToken)
	} else if *source == "images" {
		// Images source: only pull device filters from command line (FilterModels)
		cfg.InputDir = *inputDir
		if *all {
			// -all: FilterModels stays nil => include all devices
		} else if *models != "" {
			// Use -models flag to filter specific device models
			parts := strings.Split(*models, ",")
			for _, p := range parts {
				cfg.FilterModels = append(cfg.FilterModels, strings.TrimSpace(p))
			}
		}
		// If neither -all nor -models provided, FilterModels stays nil => all devices
	} else {
		logger.Error("Error: invalid source '%s'. Must be 'images' or 'ha'", *source)
		buildDBCmd.Usage()
		os.Exit(1)
	}

	if err := processor.BuildDB(cfg); err != nil {
		logger.Error("Error building database: %v", err)
		os.Exit(1)
	}
}
