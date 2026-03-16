package main

import (
    "flag"
    "fmt"
    "os"
    "sort"
    "strings"

    "github.com/AlecAivazis/survey/v2"
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
        // For HA, InputDir not needed
    } else {
        // Images source
        cfg.InputDir = *inputDir
        if *all {
            // FilterModels remains nil => all devices
        } else if *haDevices != "" {
            parts := strings.Split(*haDevices, ",")
            for _, p := range parts {
                cfg.FilterModels = append(cfg.FilterModels, strings.TrimSpace(p))
            }
        } else {
            // Interactive device discovery
            devices, err := processor.DiscoverDevices(*inputDir)
            if err != nil {
                logger.Error("Error discovering devices: %v", err)
                os.Exit(1)
            }
            if len(devices) == 0 {
                logger.Info("No devices with GPS data found in the input directory.")
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
                logger.Error("Error during device selection: %v", err)
                os.Exit(1)
            }
            // Map selected displays back to device models
            cfg.FilterModels = make([]string, len(selectedDisplays))
            for i, disp := range selectedDisplays {
                cfg.FilterModels[i] = displayToModel[disp]
            }
            if len(cfg.FilterModels) == 0 {
                logger.Info("No devices selected. Exiting.")
                os.Exit(0)
            }
        }
    }

    if err := processor.BuildDB(cfg); err != nil {
        logger.Error("Error building database: %v", err)
        os.Exit(1)
    }
}
