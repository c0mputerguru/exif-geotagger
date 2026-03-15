package processor

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/abpatel/exif-geotagger/pkg/database"
	"github.com/abpatel/exif-geotagger/pkg/exiftool"
	"github.com/abpatel/exif-geotagger/pkg/homeassistant"
	"github.com/abpatel/exif-geotagger/pkg/matcher"
)

// Supported file extensions
var (
	// ImageFileExtensions are extensions for standard images (JPG, JPEG, HEIC, PNG)
	ImageFileExtensions = []string{".jpg", ".jpeg", ".heic", ".png"}
	// RawFileExtensions are extensions for raw camera formats plus JPEG
	RawFileExtensions = []string{".cr2", ".cr3", ".nef", ".arw", ".dng", ".jpg"}
)

// haClient is a concrete implementation of homeassistant.Client using HTTP.
type haClient struct {
	baseURL string
	token   string
	client  *http.Client
}

func (c *haClient) Get(ctx context.Context, url string) (io.ReadCloser, error) {
	fullURL := c.baseURL + url
	req, err := http.NewRequestWithContext(ctx, "GET", fullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read error response: %w", err)
		}
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}
	return resp.Body, nil
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

func (c *haClient) GetTimezone(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/api/config", nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to get timezone: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	var cfg struct {
		Timezone string `json:"timezone"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&cfg); err != nil {
		return "", fmt.Errorf("failed to decode config: %w", err)
	}
	return cfg.Timezone, nil
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
		trackers, err := homeassistant.DiscoverDeviceTrackers(ctx, url, token)
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
		trackers, err := homeassistant.DiscoverDeviceTrackers(ctx, url, token)
		if err != nil {
			return fmt.Errorf("failed to discover device trackers: %w", err)
		}
		if len(trackers) == 0 {
			return fmt.Errorf("no device_tracker entities found")
		}
		fmt.Println("Discovered device_tracker entities:")
		for i, t := range trackers {
			name := t.FriendlyName
			if name == "" {
				name = t.EntityID
			}
			lastSeen := t.LastSeen
			if lastSeen != "" {
				lastSeen = " (last seen: " + lastSeen + ")"
			}
			fmt.Printf("%d. %s%s (%s)\n", i+1, name, lastSeen, t.EntityID)
		}
		fmt.Print("Enter numbers (comma-separated) to include: ")
		scanner := bufio.NewScanner(os.Stdin)
		if !scanner.Scan() {
			return fmt.Errorf("failed to read selection")
		}
		input := scanner.Text()
		if strings.TrimSpace(input) == "" {
			return fmt.Errorf("no devices selected")
		}
		idxStrs := strings.Split(input, ",")
		selected := []string{}
		for _, idxStr := range idxStrs {
			idxStr = strings.TrimSpace(idxStr)
			if idxStr == "" {
				continue
			}
			idx, err := strconv.Atoi(idxStr)
			if err != nil || idx < 1 || idx > len(trackers) {
				fmt.Fprintf(os.Stderr, "Invalid selection: %s\n", idxStr)
				continue
			}
			selected = append(selected, trackers[idx-1].EntityID)
		}
		if len(selected) == 0 {
			return fmt.Errorf("no valid devices selected")
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
		if err := repo.Insert(e); err != nil {
			log.Printf("Warning: failed to insert location for %s: %v", e.DeviceModel, err)
		} else {
			count++
		}
	}

	fmt.Printf("Successfully built database at %s with %d entries from Home Assistant.\n", outputDB, count)
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

		match, err := provider.FindBestMatch(ts, priorityDevices)
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
