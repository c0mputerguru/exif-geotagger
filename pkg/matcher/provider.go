package matcher

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/abpatel/exif-geotagger/pkg/database"
)

// DefaultSearchWindow is the default time window to search for location matches.
// 12 hours is a reasonable window for geotagging photos taken over a half-day period.
const DefaultSearchWindow = 12 * time.Hour

// DefaultTimeThreshold is the maximum acceptable time difference for a match.
// 6 hours ensures we don't match locations that are too far apart in time.
const DefaultTimeThreshold = 6 * time.Hour

// DefaultPriorityMultiplier is the score multiplier applied to priority devices.
// A factor of 5.0 gives a 5x boost to the base score for priority devices.
const DefaultPriorityMultiplier = 5.0

// LocationProvider defines how to fetch the best location match.
type LocationProvider interface {
	FindBestMatch(targetTime time.Time, priorityDevices []string) (database.LocationEntry, error)
}

type SQLiteLocationProvider struct {
	repo               *database.Repository
	searchWindow       time.Duration
	timeThreshold      time.Duration // Max acceptable time difference
	priorityMultiplier float64
}

// ProviderOptions defines configuration for the location provider
type ProviderOptions struct {
	SearchWindow       time.Duration
	TimeThreshold      time.Duration
	PriorityMultiplier float64
}

// DefaultProviderOptions returns the default settings
func DefaultProviderOptions() ProviderOptions {
	return ProviderOptions{
		SearchWindow:       DefaultSearchWindow,
		TimeThreshold:      DefaultTimeThreshold,
		PriorityMultiplier: DefaultPriorityMultiplier,
	}
}

// NewSQLiteLocationProvider creates a new provider with optional configuration
func NewSQLiteLocationProvider(repo *database.Repository, opts ...ProviderOptions) *SQLiteLocationProvider {
	opt := DefaultProviderOptions()
	if len(opts) > 0 {
		opt = opts[0]
	}
	return &SQLiteLocationProvider{
		repo:               repo,
		searchWindow:       opt.SearchWindow,
		timeThreshold:      opt.TimeThreshold,
		priorityMultiplier: opt.PriorityMultiplier,
	}
}

func (s *SQLiteLocationProvider) FindBestMatch(ctx context.Context, targetTime time.Time, priorityDevices []string) (database.LocationEntry, error) {
	entries, err := s.repo.FindClosest(ctx, targetTime, s.searchWindow)
	if err != nil {
		return database.LocationEntry{}, err
	}
	if len(entries) == 0 {
		return database.LocationEntry{}, fmt.Errorf("no matches found within %v", s.searchWindow)
	}

	bestEntry := entries[0]
	bestScore := -1.0

	// Pre-compute lowercase priority devices to optimize device checking
	lowerPriorityDevices := make([]string, len(priorityDevices))
	for i, p := range priorityDevices {
		lowerPriorityDevices[i] = strings.ToLower(p)
	}

	// Calculate a score for each entry.
	// Score calculation:
	// - Start with a max base score for time proximity (e.g. 100).
	// - Subtract points based on time difference (e.g., 100 - (diffInMinutes / 60)).
	// - Apply a score multiplier for priority devices.

	for _, entry := range entries {
		diff := entry.Timestamp.Sub(targetTime)
		if diff < 0 {
			diff = -diff
		}

		if diff > s.timeThreshold {
			continue // Too far apart
		}

		// Base score out of 100 based on how close it is (0 diff = 100 score)
		// Decay linearly to 0 at the threshold.
		score := 100.0 * (1.0 - float64(diff)/float64(s.timeThreshold))

		if isPriorityDevice(entry.DeviceModel, lowerPriorityDevices) {
			score *= s.priorityMultiplier // Score multiplier for priority device
		}

		if score > bestScore {
			bestScore = score
			bestEntry = entry
		}
	}

	if bestScore < 0 {
		return database.LocationEntry{}, fmt.Errorf("no matches found within threshold of %v", s.timeThreshold)
	}

	return bestEntry, nil
}

func isPriorityDevice(model string, lowerPriorityDevices []string) bool {
	lowerModel := strings.ToLower(model)
	for _, p := range lowerPriorityDevices {
		if strings.Contains(lowerModel, p) {
			return true
		}
	}
	return false
}
