package homeassistant

import (
	"context"
	"io"
)

// Client defines the interface for Home Assistant API interactions.
// This will be implemented by the HA Client Core (ge-d1z).
type Client interface {
	// Get performs an authenticated GET request and returns the response body.
	// Caller should close the body.
	Get(ctx context.Context, url string) (io.ReadCloser, error)

	// GetTimezone returns the Home Assistant timezone string.
	GetTimezone(ctx context.Context) (string, error)
}
