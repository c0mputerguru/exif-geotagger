package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/abpatel/exif-geotagger/pkg/logger"
	"github.com/abpatel/exif-geotagger/pkg/matcher"
	"github.com/abpatel/exif-geotagger/pkg/processor"
)

// TagImagesConfig holds the configuration for tag-images command.
type TagImagesConfig struct {
	RawDir             string
	DBPath             string
	DryRun             bool
	PriorityDevices    []string
	SearchWindow       time.Duration
	TimeThreshold      time.Duration
	PriorityMultiplier float64
}

// parseTagImagesArgs parses command line arguments and returns a TagImagesConfig.
// It handles all flag definitions, parsing, and validation.
func parseTagImagesArgs(args []string) (*TagImagesConfig, error) {
	tagImagesCmd := flag.NewFlagSet("tag-images", flag.ExitOnError)
	rawDir := tagImagesCmd.String("raw-dir", "", "Directory containing raw images to tag (required)")
	dbPath := tagImagesCmd.String("db", "db.sqlite", "Path to SQLite database")
	dryRun := tagImagesCmd.Bool("dry-run", false, "Preview changes without writing")
	priorityDevices := tagImagesCmd.String("priority-devices", "", "Comma-separated list of priority devices (e.g., 'iPhone,Pixel')")
	searchWindow := tagImagesCmd.String("search-window", "12h", "Search window duration (e.g., 12h, 30m)")
	timeThreshold := tagImagesCmd.String("time-threshold", "6h", "Maximum time difference threshold (e.g., 6h, 30m)")
	priorityMultiplier := tagImagesCmd.Float64("priority-multiplier", 5.0, "Score multiplier for priority devices")

	if err := tagImagesCmd.Parse(args); err != nil {
		return nil, err
	}

	// Validate required -raw-dir
	if *rawDir == "" {
		return nil, fmt.Errorf("-raw-dir directory is required")
	}

	// Parse priority devices
	var priorityDevicesList []string
	if *priorityDevices != "" {
		parts := strings.Split(*priorityDevices, ",")
		for _, p := range parts {
			trimmed := strings.TrimSpace(p)
			if trimmed != "" {
				priorityDevicesList = append(priorityDevicesList, trimmed)
			}
		}
	}

	// Parse durations
	searchWindowDur, err := time.ParseDuration(*searchWindow)
	if err != nil {
		return nil, fmt.Errorf("invalid search-window duration: %w", err)
	}
	timeThresholdDur, err := time.ParseDuration(*timeThreshold)
	if err != nil {
		return nil, fmt.Errorf("invalid time-threshold duration: %w", err)
	}

	return &TagImagesConfig{
		RawDir:             *rawDir,
		DBPath:             *dbPath,
		DryRun:             *dryRun,
		PriorityDevices:    priorityDevicesList,
		SearchWindow:       searchWindowDur,
		TimeThreshold:      timeThresholdDur,
		PriorityMultiplier: *priorityMultiplier,
	}, nil
}

func runTagImages() {
	cfg, err := parseTagImagesArgs(os.Args[2:])
	if err != nil {
		logger.Error("Error: %v", err)
		os.Exit(1)
	}

	opts := matcher.ProviderOptions{
		SearchWindow:       cfg.SearchWindow,
		TimeThreshold:      cfg.TimeThreshold,
		PriorityMultiplier: cfg.PriorityMultiplier,
	}

	if err := processor.TagImages(context.Background(), cfg.RawDir, cfg.DBPath, cfg.DryRun, cfg.PriorityDevices, opts); err != nil {
		logger.Error("Error tagging images: %v", err)
		os.Exit(1)
	}
}
