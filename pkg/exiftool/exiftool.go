package exiftool

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Retry configuration for transient errors
var (
	MaxRetries     = 3
	InitialBackoff = 100 * time.Millisecond
	MaxBackoff     = 5 * time.Second
)

// withRetry executes a function with exponential backoff retry logic.
// It retries on transient errors (exiftool failures) up to MaxRetries.
func withRetry(operation func() error) error {
	var err error
	backoff := InitialBackoff

	for attempt := 0; attempt < MaxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(backoff)
			if backoff < MaxBackoff {
				backoff = backoff * 2
				if backoff > MaxBackoff {
					backoff = MaxBackoff
				}
			}
		}

		err = operation()
		if err == nil {
			return nil
		}

		// Only retry on certain error types (transient failures)
		// For exiftool, we retry on any error as it could be file locking, temp I/O, etc.
	}

	return fmt.Errorf("exceeded max retries (%d): %w", MaxRetries, err)
}

type Metadata struct {
	GPSLatitude            *float64 `json:"GPSLatitude,omitempty"`
	GPSLongitude           *float64 `json:"GPSLongitude,omitempty"`
	GPSAltitude            *float64 `json:"GPSAltitude,omitempty"`
	City                   *string  `json:"City,omitempty"`
	State                  *string  `json:"State,omitempty"`
	Country                *string  `json:"Country,omitempty"`
	Model                  *string  `json:"Model,omitempty"`
	GPSDateTime            *string  `json:"GPSDateTime,omitempty"`
	SubSecDateTimeOriginal *string  `json:"SubSecDateTimeOriginal,omitempty"`
	SubSecCreateDate       *string  `json:"SubSecCreateDate,omitempty"`
	SubSecModifyDate       *string  `json:"SubSecModifyDate,omitempty"`
	CreateDate             *string  `json:"CreateDate,omitempty"`
	DateTimeOriginal       *string  `json:"DateTimeOriginal,omitempty"`
	FileModifyDate         *string  `json:"FileModifyDate,omitempty"`
}

// GetTimestamp tries to parse the best available timestamp.
// It first attempts RFC3339 parsing directly, then falls back to normalizing
// EXIF-style formats ("YYYY:MM:DD HH:MM:SS" variations) into RFC3339.
func (m *Metadata) GetTimestamp() (time.Time, error) {
	candidates := []*string{
		m.GPSDateTime,            // e.g., "2025:07:18 10:15:23Z" or RFC3339
		m.SubSecDateTimeOriginal, // e.g., "2025:07:18 12:15:50.633+02:00"
		m.SubSecCreateDate,
		m.SubSecModifyDate,
		m.DateTimeOriginal, // e.g., "2025:07:18 12:15:50"
		m.CreateDate,
		m.FileModifyDate,
	}
	for _, c := range candidates {
		if c != nil && *c != "" {
			s := *c
			// Try direct RFC3339 parse first (covers ISO8601 and already-normalized strings)
			if t, err := time.ParseInLocation(time.RFC3339, s, time.Local); err == nil {
				return t, nil
			}
			// Check if it's an EXIF format: contains ":" in date part and " " separator
			if strings.Contains(s, ":") && strings.Contains(s, " ") {
				// Normalize EXIF to RFC3339: replace colons in date part with dashes, space with 'T'
				parts := strings.SplitN(s, " ", 2)
				if len(parts) == 2 {
					datePart := strings.ReplaceAll(parts[0], ":", "-")
					timePart := parts[1]
					normalized := datePart + "T" + timePart
					if t, err := time.ParseInLocation(time.RFC3339, normalized, time.Local); err == nil {
						return t, nil
					}
				}
			}
		}
	}
	return time.Time{}, fmt.Errorf("no valid timestamp found")
}

func ReadMetadata(filePath string) (Metadata, error) {
	// Wrap exiftool call with retry logic for transient errors
	var result Metadata
	err := withRetry(func() error {
		// -n: Print numeric values
		// -json: JSON output
		// -m: Ignore minor warnings
		cmd := exec.Command("exiftool", "-json", "-n", "-m", filePath)
		output, err := cmd.Output()
		if err != nil {
			// Output usually contains stderr info
			return fmt.Errorf("exiftool failed: %w", err)
		}

		var metaList []Metadata
		if err := json.Unmarshal(output, &metaList); err != nil {
			return fmt.Errorf("failed to parse exiftool output: %w", err)
		}

		if len(metaList) == 0 {
			return fmt.Errorf("no metadata found for %s", filePath)
		}

		result = metaList[0]
		return nil
	})

	return result, err
}

func WriteMetadata(filePath string, meta Metadata, dryRun bool) error {
	args := []string{}
	// Only write tags that are present and not nil
	if meta.GPSLatitude != nil && meta.GPSLongitude != nil {
		args = append(args, fmt.Sprintf("-GPSLatitude=%f", *meta.GPSLatitude))
		args = append(args, fmt.Sprintf("-GPSLongitude=%f", *meta.GPSLongitude))
		// Optional: writing references makes it more compliant
		latRef := "N"
		if *meta.GPSLatitude < 0 {
			latRef = "S"
		}
		lonRef := "E"
		if *meta.GPSLongitude < 0 {
			lonRef = "W"
		}
		args = append(args, fmt.Sprintf("-GPSLatitudeRef=%s", latRef))
		args = append(args, fmt.Sprintf("-GPSLongitudeRef=%s", lonRef))
	}
	if meta.GPSAltitude != nil {
		args = append(args, fmt.Sprintf("-GPSAltitude=%f", *meta.GPSAltitude))
		altRef := "0" // above sea level
		if *meta.GPSAltitude < 0 {
			altRef = "1" // below sea level
		}
		args = append(args, fmt.Sprintf("-GPSAltitudeRef=%s", altRef))
	}
	if meta.City != nil {
		args = append(args, fmt.Sprintf("-City=%s", *meta.City))
	}
	if meta.State != nil {
		args = append(args, fmt.Sprintf("-State=%s", *meta.State))
	}
	if meta.Country != nil {
		args = append(args, fmt.Sprintf("-Country=%s", *meta.Country))
	}

	if len(args) == 0 {
		return nil // Nothing to write
	}

	// Overwrite original instead of leaving _original files
	args = append(args, "-overwrite_original")
	args = append(args, filePath)

	if dryRun {
		fmt.Printf("[DRY RUN] Would write to %s: %s\n", filePath, strings.Join(args[:len(args)-1], " "))
		return nil
	}

	// Wrap exiftool call with retry logic for transient errors
	err := withRetry(func() error {
		cmd := exec.Command("exiftool", args...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("exiftool write failed: %s, %w", string(output), err)
		}
		return nil
	})

	return err
}
