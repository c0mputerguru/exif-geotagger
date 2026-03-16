package processor

import (
	"context"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"time"

	"github.com/abpatel/exif-geotagger/pkg/database"
	"github.com/abpatel/exif-geotagger/pkg/exiftool"
	"github.com/abpatel/exif-geotagger/pkg/homeassistant"
	"github.com/abpatel/exif-geotagger/pkg/logger"
	"github.com/abpatel/exif-geotagger/pkg/matcher"
)

// Supported file extensions
var (
	// ImageFileExtensions are extensions for standard images (JPG, JPEG, HEIC, PNG)
	ImageFileExtensions = []string{".jpg", ".jpeg", ".heic", ".png"}
	// RawFileExtensions are extensions for raw camera formats plus JPEG
	RawFileExtensions = []string{".cr2", ".cr3", ".nef", ".arw", ".dng", ".jpg"}
)

// copyLocationEntry creates a new exiftool.Metadata from a database.LocationEntry,
// performing deep copies of pointer fields to avoid data races when the entry is reused.
func copyLocationEntry(entry database.LocationEntry) exiftool.Metadata {
	return exiftool.Metadata{
		GPSLatitude:  &entry.Latitude,
		GPSLongitude: &entry.Longitude,
		GPSAltitude:  ptrCopy(entry.Altitude),
		City:         ptrCopy(entry.City),
		State:        ptrCopy(entry.State),
		Country:      ptrCopy(entry.Country),
	}
}

// ptrCopy returns a pointer to a copy of the value if src is non-nil, otherwise returns nil.
func ptrCopy[T any](src *T) *T {
	if src == nil {
		return nil
	}
	v := *src
	return &v
}

// DiscoverDevices scans the input directory for images with GPS metadata and returns
// a map of device models to the latest timestamp seen for each device.
func DiscoverDevices(inputDir string) (map[string]time.Time, error) {
	devices := make(map[string]time.Time)

	err := filepath.WalkDir(inputDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".jpg" && ext != ".jpeg" && ext != ".heic" && ext != ".png" {
			return nil
		}
		meta, err := exiftool.ReadMetadata(path)
		if err != nil {
			return nil
		}
		if meta.GPSLatitude == nil || meta.GPSLongitude == nil {
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
		// Update with most recent timestamp for this model
		if existing, ok := devices[model]; !ok || ts.After(existing) {
			devices[model] = ts
		}
		return nil
	})

	return devices, err
}

// BuildDB builds the database from images in inputDir, optionally filtering by device models.
// If filterModels is nil or empty, all devices are included.
func BuildDB(inputDir string, outputDB string, filterModels []string) error {
	repo, err := database.Connect(outputDB)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer repo.Close()

	// Prepare filter set if needed
	filterSet := make(map[string]struct{})
	if len(filterModels) > 0 {
		for _, m := range filterModels {
			filterSet[m] = struct{}{}
		}
	}

	count := 0
	skipped := 0
	err = filepath.WalkDir(inputDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".jpg" && ext != ".jpeg" && ext != ".heic" && ext != ".png" {
			return nil
		}
		meta, err := exiftool.ReadMetadata(path)
		if err != nil {
			logger.Error("Failed to read metadata for %s: %v", path, err)
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
			logger.Warn("No valid timestamp for %s", path)
			skipped++
			return nil
		}
		model := "Unknown"
		if meta.Model != nil {
			model = *meta.Model
		}
		// Apply filter if set
		if len(filterSet) > 0 {
			if _, ok := filterSet[model]; !ok {
				return nil // skip this device
			}
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
		if err := repo.Insert(context.Background(), entry); err != nil {
			logger.Warn("Warning: failed to insert location for %s: %v", path, err)
			skipped++
		} else {
			count++
		}
		return nil
	})
	if err != nil {
		return err
	}
	logger.Info("Successfully built database at %s with %d entries (skipped %d).", outputDB, count, skipped)
	return nil
}

// BuildDBHA builds a location database from Home Assistant.
func BuildDBHA(outputDB, url, token, devices, startStr, endStr string, days int, all bool) error {
	// Create context for cancellation
	ctx := context.Background()

	// Trim trailing slash if present
	url = strings.TrimSuffix(url, "/")

	// 1. Determine entity IDs
	var entityIDs []string
	if devices != "" {
		parts := strings.Split(devices, ",")
		for _, p := range parts {
			if id := strings.TrimSpace(p); id != "" {
				entityIDs = append(entityIDs, id)
			}
		}
		if len(entityIDs) == 0 {
			return fmt.Errorf("no valid entity IDs provided")
		}
	} else if all {
		// Discover all devices automatically without prompting
		trackers, err := homeassistant.DiscoverDeviceTrackers(ctx, url, token, nil)
		if err != nil {
			return fmt.Errorf("failed to discover device trackers: %w", err)
		}
		if len(trackers) == 0 {
			return fmt.Errorf("no device_tracker entities found")
		}
		logger.Info("Discovering all %d device_tracker entities...", len(trackers))
		entityIDs = make([]string, len(trackers))
		for i, t := range trackers {
			entityIDs[i] = t.EntityID
		}
	} else {
		// Discover devices interactively
		trackers, err := homeassistant.DiscoverDeviceTrackers(ctx, url, token, nil)
		if err != nil {
			return fmt.Errorf("failed to discover device trackers: %w", err)
		}
		if len(trackers) == 0 {
			return fmt.Errorf("no device_tracker entities found")
		}
		selected, err := homeassistant.SelectDeviceTrackersInteractive(trackers)
		if err != nil {
			return fmt.Errorf("failed to select devices: %w", err)
		}
		entityIDs = selected
	}

	// 2. Determine time range
	var start, end time.Time
	var err error
	if days > 0 {
		end = time.Now()
		start = end.Add(-time.Duration(days) * 24 * time.Hour)
	} else if startStr != "" && endStr != "" {
		start, err = time.Parse(time.RFC3339, startStr)
		if err != nil {
			return fmt.Errorf("invalid ha-start: %w", err)
		}
		end, err = time.Parse(time.RFC3339, endStr)
		if err != nil {
			return fmt.Errorf("invalid ha-end: %w", err)
		}
	} else {
		// Default: last 365 days
		end = time.Now()
		start = end.Add(-365 * 24 * time.Hour)
	}

	// 3. Create HA client
	client := homeassistant.NewClient(url, token)

	// 4. Fetch location history
	entries, err := homeassistant.FetchLocationHistory(ctx, client, start, end, entityIDs)
	if err != nil {
		return fmt.Errorf("failed to fetch location history: %w", err)
	}

	// 5. Insert into database
	count := 0
	repo, err := database.Connect(outputDB)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer repo.Close()
	for _, e := range entries {
		if err := repo.Insert(ctx, e); err != nil {
			logger.Warn("Warning: failed to insert location for %s: %v", e.DeviceModel, err)
		} else {
			count++
		}
	}

	logger.Info("Successfully built database at %s with %d entries from Home Assistant.", outputDB, count)
	return nil
}

// TagImages tags raw images with GPS data from the database.
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
			logger.Error("Failed to read metadata for %s: %v", path, err)
			skipped++
			return nil
		}

		// Skip if it already has GPS tags
		if meta.GPSLatitude != nil && meta.GPSLongitude != nil {
			logger.Info("Skipping %s (already has GPS data)", path)
			skipped++
			return nil
		}

		ts, err := meta.GetTimestamp()
		if err != nil {
			logger.Warn("No valid timestamp for %s", path)
			skipped++
			return nil
		}

		match, err := provider.FindBestMatch(context.Background(), ts, priorityDevices)
		if err != nil {
			logger.Warn("No match found for %s (time: %s): %v", path, ts, err)
			skipped++
			return nil
		}

		// Use helper to copy match data and avoid data race (loop variable reuse)
		newMeta := copyLocationEntry(match)

		if err := exiftool.WriteMetadata(path, newMeta, dryRun); err != nil {
			logger.Error("Failed to write metadata to %s: %v", path, err)
		} else {
			if !dryRun {
				logger.Info("Successfully tagged %s with location from %s (time diff: %v)", path, match.DeviceModel, match.Timestamp.Sub(ts))
			}
			count++
		}

		return nil
	})

	if err != nil {
		return err
	}

	if dryRun {
		logger.Info("Dry run complete. Would have tagged %d images (skipped %d)", count, skipped)
	} else {
		logger.Info("Tagging complete. Successfully tagged %d images (skipped %d)", count, skipped)
	}
	return nil
}
