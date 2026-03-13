package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

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

		buildDBCmd.Parse(os.Args[2:])

		if *inputDir == "" {
			fmt.Println("Error: -input directory is required")
			buildDBCmd.Usage()
			os.Exit(1)
		}

		if err := processor.BuildDB(*inputDir, *outputDB); err != nil {
			fmt.Printf("Error building database: %v\n", err)
			os.Exit(1)
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
