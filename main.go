package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/abpatel/exif-geotagger/pkg/database"
	"github.com/abpatel/exif-geotagger/pkg/processor"
)

func main() {
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
				fmt.Println("Error: -input directory is required when source is 'images'")
				buildDBCmd.Usage()
				os.Exit(1)
			}
		}

		if *source == "ha" {
			// Call BuildDBHA with HA parameters
			if err := processor.BuildDBHA(*outputDB, *haURL, *haToken, *haDevices, *haStart, *haEnd, *haDays); err != nil {
				fmt.Printf("Error building database from Home Assistant: %v\n", err)
				os.Exit(1)
			}
		} else {
<<<<<<< HEAD
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
					fmt.Printf("Error discovering devices: %v\n", err)
					os.Exit(1)
				}
				if len(devices) == 0 {
					fmt.Println("No devices with GPS data found in the input directory.")
					os.Exit(1)
				}
				// Prepare options for prompt
				type devInfo struct {
					model    string
					lastSeen time.Time
				}
				var options []devInfo
				for model, ts := range devices {
					options = append(options, devInfo{model: model, lastSeen: ts})
				}
				// Sort by lastSeen descending for better UX
				sort.Slice(options, func(i, j int) bool {
					return options[i].lastSeen.After(options[j].lastSeen)
				})
				var surveyOpts []survey.Option
				for _, opt := range options {
					display := fmt.Sprintf("%s (last seen: %s)", opt.model, opt.lastSeen.Format("2006-01-02 15:04:05"))
					surveyOpts = append(surveyOpts, survey.Option{
						Name:  display,
						Value: opt.model,
					})
				}
				var selected []string
				err = survey.AskOne(&survey.MultiSelect{
					Message: "Select devices to include in database:",
					Options: surveyOpts,
				}, &selected)
				if err != nil {
					fmt.Printf("Error during device selection: %v\n", err)
					os.Exit(1)
				}
				deviceFilter = selected
				if len(deviceFilter) == 0 {
					fmt.Println("No devices selected. Exiting.")
					os.Exit(0)
				}
			}

			if err := processor.BuildDB(*inputDir, *outputDB, deviceFilter); err != nil {
				fmt.Printf("Error building database: %v\n", err)
				os.Exit(1)
			}
=======
			// Interactive device discovery
			devices, err := processor.DiscoverDevices(*inputDir)
			if err != nil {
				fmt.Printf("Error discovering devices: %v\n", err)
				os.Exit(1)
			}
			if len(devices) == 0 {
				fmt.Println("No devices with GPS data found in the input directory.")
				os.Exit(1)
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
				fmt.Printf("Error during device selection: %v\n", err)
				os.Exit(1)
			}
			// Map selected displays back to device models
			deviceFilter = make([]string, len(selectedDisplays))
			for i, disp := range selectedDisplays {
				deviceFilter[i] = displayToModel[disp]
			}
			if len(deviceFilter) == 0 {
				fmt.Println("No devices selected. Exiting.")
				os.Exit(0)
			}
		}

		if err := processor.BuildDB(*inputDir, *outputDB, deviceFilter); err != nil {
			fmt.Printf("Error building database: %v\n", err)
			os.Exit(1)
>>>>>>> 414279f (fix: correct log.Printf args in TagImages and improve sorting)
		}

	case "print-db":
		printDbCmd := flag.NewFlagSet("print-db", flag.ExitOnError)
		dbPath := printDbCmd.String("db", "db.sqlite", "Path to SQLite database")

		printDbCmd.Parse(os.Args[2:])

		repo, err := database.Connect(*dbPath)
		if err != nil {
			fmt.Printf("Error connecting to database: %v\n", err)
			os.Exit(1)
		}
		defer repo.Close()

		entries, err := repo.GetAll()
		if err != nil {
			fmt.Printf("Error fetching entries: %v\n", err)
			os.Exit(1)
		}

		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(entries); err != nil {
			fmt.Printf("Error encoding JSON: %v\n", err)
			os.Exit(1)
		}

	case "tag-images":
		tagImagesCmd := flag.NewFlagSet("tag-images", flag.ExitOnError)
		rawDir := tagImagesCmd.String("raw-dir", "", "Directory containing raw images to tag")
		dbPath := tagImagesCmd.String("db", "db.sqlite", "Path to SQLite database")
		dryRun := tagImagesCmd.Bool("dry-run", false, "Preview changes without writing")
		priorityDevices := tagImagesCmd.String("priority-devices", "", "Comma-separated list of priority devices (e.g., 'iPhone,Pixel')")

		tagImagesCmd.Parse(os.Args[2:])

		if *rawDir == "" {
			fmt.Println("Error: -raw-dir directory is required")
			tagImagesCmd.Usage()
			os.Exit(1)
		}

		var devicesList []string
		if *priorityDevices != "" {
			devicesList = strings.Split(*priorityDevices, ",")
			for i, d := range devicesList {
				devicesList[i] = strings.TrimSpace(d)
			}
		}

		if err := processor.TagImages(*rawDir, *dbPath, *dryRun, devicesList); err != nil {
			fmt.Printf("Error tagging images: %v\n", err)
			os.Exit(1)
		}

	default:
		fmt.Printf("Unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
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
