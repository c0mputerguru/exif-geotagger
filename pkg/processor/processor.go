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
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}
	return resp.Body, nil
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
		return buildFromHA(repo, haURL, haToken, haDevices, haStart, haEnd, outputDB, haDays)
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

func buildFromHA(repo *database.Repository, url, token, devices, startStr, endStr, outputDB string, days int) error {
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
	} else {
		// Discover devices interactively
		trackers, err := homeassistant.DiscoverDeviceTrackers(url, token)
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
			fmt.Printf("%d. %s (%s)\n", i+1, name, t.EntityID)
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
		// Default: all available history from year 2000
		start = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
		end = time.Now()
	}

	// 3. Create HA client
	client := &haClient{
		baseURL: url,
		token:   token,
		client:  &http.Client{Timeout: 30 * time.Second},
	}

	// 4. Fetch location history
	ctx := context.Background()
	entries, err := homeassistant.FetchLocationHistory(ctx, client, start, end, entityIDs)
	if err != nil {
		return fmt.Errorf("failed to fetch location history: %w", err)
	}

	// 5. Insert into database
	count := 0
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
