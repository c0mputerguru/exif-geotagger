package processor

import (
	"fmt"
	"io/fs"
	"log"
	"path/filepath"
	"strings"

	"github.com/abpatel/exif-geotagger/pkg/database"
	"github.com/abpatel/exif-geotagger/pkg/exiftool"
	"github.com/abpatel/exif-geotagger/pkg/matcher"
)

func BuildDB(inputDir string, outputDB string) error {
	repo, err := database.Connect(outputDB)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer repo.Close()

	count := 0
	err = filepath.WalkDir(inputDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		// Simple filter for common image types
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".jpg" && ext != ".jpeg" && ext != ".heic" && ext != ".png" {
			return nil
		}

		meta, err := exiftool.ReadMetadata(path)
		if err != nil {
			// Skip files with no/failed metadata
			return nil
		}

		if meta.GPSLatitude == nil || meta.GPSLongitude == nil {
			// Skip if no GPS
			return nil
		}

		ts, err := meta.GetTimestamp()
		if err != nil {
			return nil
		}

		model := "Unknown"
		if meta.Model != nil {
			model = *meta.Model
		}

		entry := database.LocationEntry{
			Timestamp:   ts,
			Latitude:    *meta.GPSLatitude,
			Longitude:   *meta.GPSLongitude,
			Altitude:    meta.GPSAltitude,
			City:        meta.City,
			State:       meta.State,
			Country:     meta.Country,
			DeviceModel: model,
		}

		if err := repo.Insert(entry); err != nil {
			log.Printf("Warning: failed to insert location for %s: %v", path, err)
		} else {
			count++
		}

		return nil
	})

	if err != nil {
		return err
	}
	fmt.Printf("Successfully built database at %s with %d entries.\n", outputDB, count)
	return nil
}

func TagImages(rawDir string, dbPath string, dryRun bool, priorityDevices []string) error {
	repo, err := database.Connect(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer repo.Close()

	provider := matcher.NewSQLiteLocationProvider(repo)

	count := 0
	skipped := 0

	err = filepath.WalkDir(rawDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		// Support typical raw formats: .CR2, .CR3, .NEF, .ARW, .DNG, etc.
		if ext != ".cr2" && ext != ".cr3" && ext != ".nef" && ext != ".arw" && ext != ".dng" && ext != ".jpg" {
			return nil
		}

		meta, err := exiftool.ReadMetadata(path)
		if err != nil {
			log.Printf("Failed to read metadata for %s: %v", path, err)
			return nil
		}

		// Skip if it already has GPS tags
		if meta.GPSLatitude != nil && meta.GPSLongitude != nil {
			fmt.Printf("Skipping %s (already has GPS data)\n", path)
			skipped++
			return nil
		}

		ts, err := meta.GetTimestamp()
		if err != nil {
			log.Printf("Warning: No valid timestamp for %s\n", path)
			return nil
		}

		match, err := provider.FindBestMatch(ts, priorityDevices)
		if err != nil {
			log.Printf("No match found for %s (time: %s): %v\n", path, ts, err)
			skipped++
			return nil
		}

		// Prepare new metadata block
		newMeta := exiftool.Metadata{
			GPSLatitude:  &match.Latitude,
			GPSLongitude: &match.Longitude,
			GPSAltitude:  match.Altitude,
			City:         match.City,
			State:        match.State,
			Country:      match.Country,
		}

		if err := exiftool.WriteMetadata(path, newMeta, dryRun); err != nil {
			log.Printf("Failed to write metadata to %s: %v", path, err)
		} else {
			if !dryRun {
				fmt.Printf("Successfully tagged %s with location from %s (time diff: %v)\n", path, match.DeviceModel, match.Timestamp.Sub(ts))
			}
			count++
		}

		return nil
	})

	if err != nil {
		return err
	}

	if dryRun {
		fmt.Printf("Dry run complete. Would have tagged %d images (skipped %d)\n", count, skipped)
	} else {
		fmt.Printf("Tagging complete. Successfully tagged %d images (skipped %d)\n", count, skipped)
	}
	return nil
}
