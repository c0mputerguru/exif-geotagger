package main

import (
	"context"
	"flag"
	"os"
	"strings"
	"time"

	"github.com/abpatel/exif-geotagger/pkg/logger"
	"github.com/abpatel/exif-geotagger/pkg/matcher"
	"github.com/abpatel/exif-geotagger/pkg/processor"
)

func runTagImages() {
	tagImagesCmd := flag.NewFlagSet("tag-images", flag.ExitOnError)
	rawDir := tagImagesCmd.String("raw-dir", "", "Directory containing raw images to tag")
	dbPath := tagImagesCmd.String("db", "db.sqlite", "Path to SQLite database")
	dryRun := tagImagesCmd.Bool("dry-run", false, "Preview changes without writing")
	priorityDevices := tagImagesCmd.String("priority-devices", "", "Comma-separated list of priority devices (e.g., 'iPhone,Pixel')")
	searchWindow := tagImagesCmd.String("search-window", "12h", "Search window duration (e.g., 12h, 30m)")
	timeThreshold := tagImagesCmd.String("time-threshold", "6h", "Maximum time difference threshold (e.g., 6h, 30m)")
	priorityMultiplier := tagImagesCmd.Float64("priority-multiplier", 5.0, "Score multiplier for priority devices")

	tagImagesCmd.Parse(os.Args[2:])

	if *rawDir == "" {
		logger.Error("Error: -raw-dir directory is required")
		tagImagesCmd.Usage()
		os.Exit(1)
	} else {
		var priorityDevicesList []string
		if *priorityDevices != "" {
			priorityDevicesList = strings.Split(*priorityDevices, ",")
			for i, d := range priorityDevicesList {
				priorityDevicesList[i] = strings.TrimSpace(d)
			}
		}

		// Parse provider options
		searchWindowDur, err := time.ParseDuration(*searchWindow)
		if err != nil {
			logger.Error("Invalid search-window duration: %v", err)
			os.Exit(1)
		}
		timeThresholdDur, err := time.ParseDuration(*timeThreshold)
		if err != nil {
			logger.Error("Invalid time-threshold duration: %v", err)
			os.Exit(1)
		}

		opts := matcher.ProviderOptions{
			SearchWindow:       searchWindowDur,
			TimeThreshold:      timeThresholdDur,
			PriorityMultiplier: *priorityMultiplier,
		}

		if err := processor.TagImages(context.Background(), *rawDir, *dbPath, *dryRun, priorityDevicesList, opts); err != nil {
			logger.Error("Error tagging images: %v", err)
			os.Exit(1)
		}
	}
}
