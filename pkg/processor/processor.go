package processor

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"path/filepath"
	"strings"
	"time"

	"github.com/abpatel/exif-geotagger/pkg/database"
	"github.com/abpatel/exif-geotagger/pkg/exiftool"
	"github.com/abpatel/exif-geotagger/pkg/matcher"
)

// HAClient fetches location history from Home Assistant
type HAClient interface {
	FetchLocationHistory(ctx context.Context, start, end time.Time, entityIDs []string) ([]database.LocationEntry, error)
}

// BuildDB builds a location database from either images or Home Assistant.
// Parameters:
//   - inputDir: directory of images (used when source="images")
//   - outputDB: path to output SQLite database
//   - source: "images" or "ha"
//   - haURL, haToken, haDevices, haStart, haEnd, haDays: HA parameters (used when source="ha")
func BuildDB(inputDir, outputDB, source, haURL, haToken, haDevices, haStart, haEnd string, haDays int) error {
	repo, err := database.Connect(outputDB)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer repo.Close()

	switch source {
	case "images":
		return buildFromImages(inputDir, outputDB, repo)
	case "ha":
		return buildFromHA(repo, haURL, haToken, haDevices, haStart, haEnd, haDays)
	default:
		return fmt.Errorf("invalid source: %s (must be 'images' or 'ha')", source)
	}
}

func buildFromImages(inputDir, outputDB string, repo *database.Repository) error {
	count := 0
	skipped := 0
	err := filepath.WalkDir(inputDir, func(path string, d fs.DirEntry, err error) error {
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
			log.Printf("Skipping %s: failed to read metadata: %v\n", path, err)
			skipped++
			return nil
		}

		if meta.GPSLatitude == nil || meta.GPSLongitude == nil {
			// Skip if no GPS
			skipped++
			return nil
		}

		ts, err := meta.GetTimestamp()
		if err != nil {
			log.Printf("Skipping %s: no valid timestamp\n", path)
			skipped++
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
			skipped++
		} else {
			count++
		}

		return nil
	})

	if err != nil {
		return err
	}
	fmt.Printf("Successfully built database at %s with %d entries (skipped %d).\n", outputDB, count, skipped)
	return nil
}

func buildFromHA(repo *database.Repository, url, token, devices, start, end string, days int) error {
	// TODO: Replace with actual HA client implementation (dependency: ge-h87)
	// Expected function: FetchLocationHistory(ctx, start, end, entityIDs) ([]LocationEntry, error)
	return fmt.Errorf("HA source not yet implemented: depends on ge-h87 (Fetch Location History) and ge-0tz (Interactive Device Selection)")
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
			log.Printf("Skipping %s: failed to read metadata: %v\n", path, err)
			skipped++
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
			log.Printf("Skipping %s: no valid timestamp\n", path)
			skipped++
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
