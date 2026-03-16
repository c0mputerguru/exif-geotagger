package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/abpatel/exif-geotagger/pkg/database"
	"github.com/abpatel/exif-geotagger/pkg/logger"
	"github.com/abpatel/exif-geotagger/pkg/processor"
)

func printDatabase(dbPath string) error {
	repo, err := database.Connect(dbPath)
	if err != nil {
		return fmt.Errorf("error connecting to database: %w", err)
	}
	defer repo.Close()

	entries, err := repo.GetAll(context.Background())
	if err != nil {
		return fmt.Errorf("error fetching entries: %w", err)
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(entries); err != nil {
		return fmt.Errorf("error encoding JSON: %w", err)
	}
	return nil
}

func main() {
	exitCode := 0

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "build-db":
		buildDBCmd := flag.NewFlagSet("build-db", flag.ExitOnError)
		inputDir := buildDBCmd.String("input", "", "Directory containing reference images with GPS data")
		outputDB := buildDBCmd.String("output", "db.sqlite", "Path to output SQLite database")
		source := buildDBCmd.String("source", "images", "Data source: 'images' or 'ha'")
		haURL := buildDBCmd.String("ha-url", "", "Home Assistant URL")
		haToken := buildDBCmd.String("ha-token", "", "Home Assistant long-lived access token")
		haDevices := buildDBCmd.String("ha-devices", "", "Comma-separated list of device entity IDs")
		haStart := buildDBCmd.String("ha-start", "", "Start time for HA history (RFC3339)")
		haEnd := buildDBCmd.String("ha-end", "", "End time for HA history (RFC3339)")
		haDays := buildDBCmd.Int("ha-days", 0, "Number of days of history (alternative to start/end)")
		all := buildDBCmd.Bool("all", false, "Select all discovered devices")

		buildDBCmd.Parse(os.Args[2:])

		// Backward compatibility: if -source is not provided or is "images", require -input
		if *source == "images" || *source == "" {
			if *inputDir == "" {
				logger.Error("Error: -input directory is required when source is 'images'")
				buildDBCmd.Usage()
				exitCode = 1
				break
			}
		}

		if *source == "ha" {
			// Call BuildDBHA with HA parameters
			if err := processor.BuildDBHA(*outputDB, *haURL, *haToken, *haDevices, *haStart, *haEnd, *haDays, *all); err != nil {
				logger.Error("Error building database from Home Assistant: %v", err)
				exitCode = 1
				break
			}
		} else {
			// Images source: determine device filter from flags or interactive discovery
			var deviceFilter []string
			if *all {
				// deviceFilter remains nil => all devices
			} else if *haDevices != "" {
				parts := strings.Split(*haDevices, ",")
				for _, p := range parts {
					deviceFilter = append(deviceFilter, strings.TrimSpace(p))
				}
			} else {
				// Interactive device discovery
				devices, err := processor.DiscoverDevices(*inputDir)
				if err != nil {
					logger.Error("Error discovering devices: %v", err)
					exitCode = 1
					break
				}
				if len(devices) == 0 {
					logger.Info("No devices with GPS data found in the input directory.")
					exitCode = 1
					break
				}
				// Prepare options for prompt: map display string -> model, and sort by timestamp descending
				displayToModel := make(map[string]string)
				type option struct {
					display string
					model   string
				}
				var options []option
				for model, ts := range devices {
					display := fmt.Sprintf("%s (last seen: %s)", model, ts.Format("2006-01-02 15:04:05"))
					displayToModel[display] = model
					options = append(options, option{display: display, model: model})
				}
				// Sort by timestamp descending
				sort.Slice(options, func(i, j int) bool {
					return devices[options[i].model].After(devices[options[j].model])
				})
				// Build sorted display list
				optionList := make([]string, len(options))
				for i, opt := range options {
					optionList[i] = opt.display
				}
				var selectedDisplays []string
				err = survey.AskOne(&survey.MultiSelect{
					Message: "Select devices to include in database:",
					Options: optionList,
				}, &selectedDisplays)
				if err != nil {
					logger.Error("Error during device selection: %v", err)
					exitCode = 1
					break
				}
				// Map selected displays back to device models
				deviceFilter = make([]string, len(selectedDisplays))
				for i, disp := range selectedDisplays {
					deviceFilter[i] = displayToModel[disp]
				}
				if len(deviceFilter) == 0 {
					logger.Info("No devices selected. Exiting.")
					exitCode = 0
					break
				}
			}

			if exitCode == 0 { // Only proceed if no error occurred
				if err := processor.BuildDB(*inputDir, *outputDB, deviceFilter); err != nil {
					logger.Error("Error building database: %v", err)
					exitCode = 1
				}
			}
		}

	case "print-db":
		printDbCmd := flag.NewFlagSet("print-db", flag.ExitOnError)
		dbPath := printDbCmd.String("db", "db.sqlite", "Path to SQLite database")

		printDbCmd.Parse(os.Args[2:])

		if err := printDatabase(*dbPath); err != nil {
			logger.Error("Error printing database: %v", err)
			exitCode = 1
		}

	case "tag-images":
		tagImagesCmd := flag.NewFlagSet("tag-images", flag.ExitOnError)
		rawDir := tagImagesCmd.String("raw-dir", "", "Directory containing raw images to tag")
		dbPath := tagImagesCmd.String("db", "db.sqlite", "Path to SQLite database")
		dryRun := tagImagesCmd.Bool("dry-run", false, "Preview changes without writing")
		priorityDevices := tagImagesCmd.String("priority-devices", "", "Comma-separated list of priority devices (e.g., 'iPhone,Pixel')")

		tagImagesCmd.Parse(os.Args[2:])

		if *rawDir == "" {
			logger.Error("Error: -raw-dir directory is required")
			tagImagesCmd.Usage()
			exitCode = 1
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
				exitCode = 1
			}
		}

	default:
		logger.Error("Unknown command: %s", os.Args[1])
		printUsage()
		exitCode = 1
	}

	os.Exit(exitCode)
}

func printUsage() {
	fmt.Println("Usage: exif-geotagger <command> [options]")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  build-db      Extract GPS data from reference images and build database")
	fmt.Println("  print-db      Print database contents as JSON")
	fmt.Println("  tag-images    Tag raw images with GPS data from database")
	fmt.Println()
	fmt.Println("Run 'exif-geotagger <command> -h' for more information on a command.")
}
