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
	"github.com/abpatel/exif-geotagger/pkg/homeassistant"
	"github.com/abpatel/exif-geotagger/pkg/matcher"
	"github.com/abpatel/exif-geotagger/pkg/urlutil"
)

// Supported file extensions
var (
	// ImageFileExtensions are extensions for standard images (JPG, JPEG, HEIC, PNG)
	ImageFileExtensions = []string{".jpg", ".jpeg", ".heic", ".png"}
	// RawFileExtensions are extensions for raw camera formats plus JPEG
	RawFileExtensions = []string{".cr2", ".cr3", ".nef", ".arw", ".dng", ".jpg"}
)

// hasExtension checks if the given extension exists in the list
func hasExtension(ext string, extensions []string) bool {
	for _, e := range extensions {
		if e == ext {
			return true
		}
	}
	return false
}

// copyLocationEntry creates a new exiftool.Metadata from a database.LocationEntry,
// performing deep copies of pointer fields to avoid data races when the entry is reused.
func copyLocationEntry(entry database.LocationEntry) exiftool.Metadata {
	lat := entry.Latitude
	lon := entry.Longitude

	var alt *float64
	if entry.Altitude != nil {
		a := *entry.Altitude
		alt = &a
	}

	var city *string
	if entry.City != nil {
		c := *entry.City
		city = &c
	}

	var state *string
	if entry.State != nil {
		s := *entry.State
		state = &s
	}

	var country *string
	if entry.Country != nil {
		co := *entry.Country
		country = &co
	}

	return exiftool.Metadata{
		GPSLatitude:  &lat,
		GPSLongitude: &lon,
		GPSAltitude:  alt,
		City:         city,
		State:        state,
		Country:      country,
	}
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
		if !hasExtension(ext, ImageFileExtensions) {
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

// BuildConfig configures the BuildDB function.
type BuildConfig struct {
	OutputDB string // Path to output SQLite database
	Source   string // "images" or "ha"

	// For images source
	InputDir     string   // Directory of images with GPS data
	FilterModels []string // Device models to include (empty = all)

	// For HA source
	HAURL     string // Home Assistant URL
	HAToken   string // Home Assistant long-lived access token
	HADevices string // Comma-separated entity IDs
	HAStart   string // Start time (RFC3339)
	HAEnd     string // End time (RFC3339)
	HADays    int    // Number of days (alternative to start/end)
	HAAll     bool   // Select all discovered devices
}

// BuildDB builds a location database from either reference images or Home Assistant.
func BuildDB(ctx context.Context, cfg BuildConfig) error {
	if cfg.Source == "ha" {
		return buildDBFromHA(ctx, cfg)
	}
	// Default to images source
	return buildDBFromImages(ctx, cfg)
}

// buildDBFromImages builds the database from images in inputDir, optionally filtering by device models.
func buildDBFromImages(ctx context.Context, cfg BuildConfig) error {
	if cfg.InputDir == "" {
		return fmt.Errorf("inputDir is required when source is 'images'")
	}

	repo, err := database.Connect(cfg.OutputDB)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer repo.Close()

	// Prepare filter set if needed
	filterSet := make(map[string]struct{})
	if len(cfg.FilterModels) > 0 {
		for _, m := range cfg.FilterModels {
			filterSet[m] = struct{}{}
		}
	}

	count := 0
	skipped := 0
	err = filepath.WalkDir(cfg.InputDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if !hasExtension(ext, ImageFileExtensions) {
			return nil
		}
		meta, err := exiftool.ReadMetadata(path)
		if err != nil {
			log.Printf("Failed to read metadata for %s: %v\n", path, err)
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
			log.Printf("Warning: No valid timestamp for %s\n", path)
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
		if err := repo.Insert(ctx, entry); err != nil {
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
	fmt.Printf("Successfully built database at %s with %d entries (skipped %d).\n", cfg.OutputDB, count, skipped)
	return nil
}

// buildDBFromHA builds a location database from Home Assistant.
func buildDBFromHA(ctx context.Context, cfg BuildConfig) error {

	// Normalize URL (trim trailing slash)
	url := urlutil.NormalizeURL(cfg.HAURL)

	// 1. Determine entity IDs
	var entityIDs []string
	if cfg.HADevices != "" {
		parts := strings.Split(cfg.HADevices, ",")
		for _, p := range parts {
			if id := strings.TrimSpace(p); id != "" {
				entityIDs = append(entityIDs, id)
			}
		}
		if len(entityIDs) == 0 {
			return fmt.Errorf("no valid entity IDs provided")
		}
	} else if cfg.HAAll {
		// Discover all devices automatically without prompting
		trackers, err := homeassistant.DiscoverDeviceTrackers(ctx, url, cfg.HAToken, nil)
		if err != nil {
			return fmt.Errorf("failed to discover device trackers: %w", err)
		}
		if len(trackers) == 0 {
			return fmt.Errorf("no device_tracker entities found")
		}
		fmt.Printf("Discovering all %d device_tracker entities...\n", len(trackers))
		entityIDs = make([]string, len(trackers))
		for i, t := range trackers {
			entityIDs[i] = t.EntityID
		}
	} else {
		// Discover devices interactively
		trackers, err := homeassistant.DiscoverDeviceTrackers(ctx, url, cfg.HAToken, nil)
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
	if cfg.HADays > 0 {
		end = time.Now()
		start = end.Add(-time.Duration(cfg.HADays) * 24 * time.Hour)
	} else if cfg.HAStart != "" && cfg.HAEnd != "" {
		start, err = time.Parse(time.RFC3339, cfg.HAStart)
		if err != nil {
			return fmt.Errorf("invalid ha-start: %w", err)
		}
		end, err = time.Parse(time.RFC3339, cfg.HAEnd)
		if err != nil {
			return fmt.Errorf("invalid ha-end: %w", err)
		}
	} else {
		// Default: last 365 days
		end = time.Now()
		start = end.Add(-365 * 24 * time.Hour)
	}

	// 3. Create HA client
	client := homeassistant.NewClient(url, cfg.HAToken)

	// 4. Fetch location history
	entries, err := homeassistant.FetchLocationHistory(ctx, client, start, end, entityIDs)
	if err != nil {
		return fmt.Errorf("failed to fetch location history: %w", err)
	}

	// 5. Insert into database
	count := 0
	repo, err := database.Connect(cfg.OutputDB)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer repo.Close()
	for _, e := range entries {
		if err := repo.Insert(ctx, e); err != nil {
			log.Printf("Warning: failed to insert location for %s: %v", e.DeviceModel, err)
		} else {
			count++
		}
	}

	fmt.Printf("Successfully built database at %s with %d entries from Home Assistant.\n", cfg.OutputDB, count)
	return nil
}

// TagImages tags raw images with GPS data from the database.
func TagImages(ctx context.Context, rawDir string, dbPath string, dryRun bool, priorityDevices []string, opts matcher.ProviderOptions) error {
	repo, err := database.Connect(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer repo.Close()

	provider := matcher.NewSQLiteLocationProvider(repo, opts)

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
		if !hasExtension(ext, RawFileExtensions) {
			return nil
		}

		meta, err := exiftool.ReadMetadata(path)
		if err != nil {
			log.Printf("Failed to read metadata for %s: %v\n", path, err)
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
			log.Printf("Warning: No valid timestamp for %s\n", path)
			skipped++
			return nil
		}

		match, err := provider.FindBestMatch(ctx, ts, priorityDevices)
		if err != nil {
			log.Printf("No match found for %s (time: %s): %v\n", path, ts, err)
			skipped++
			return nil
		}

		// Use helper to copy match data and avoid data race (loop variable reuse)
		newMeta := copyLocationEntry(match)

		if err := exiftool.WriteMetadata(path, newMeta, dryRun); err != nil {
			log.Printf("Failed to write metadata to %s: %v\n", path, err)
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
