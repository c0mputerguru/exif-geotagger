package main

import (
	"flag"
	"os"
	"strings"

	"github.com/abpatel/exif-geotagger/pkg/logger"
	"github.com/abpatel/exif-geotagger/pkg/processor"
)

func runTagImages() {
	tagImagesCmd := flag.NewFlagSet("tag-images", flag.ExitOnError)
	rawDir := tagImagesCmd.String("raw-dir", "", "Directory containing raw images to tag")
	dbPath := tagImagesCmd.String("db", "db.sqlite", "Path to SQLite database")
	dryRun := tagImagesCmd.Bool("dry-run", false, "Preview changes without writing")
	priorityDevices := tagImagesCmd.String("priority-devices", "", "Comma-separated list of priority devices (e.g., 'iPhone,Pixel')")

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

		if err := processor.TagImages(*rawDir, *dbPath, *dryRun, priorityDevicesList); err != nil {
			logger.Error("Error tagging images: %v", err)
			os.Exit(1)
		}
	}
}
