package urlutil

import "strings"

// NormalizeURL trims trailing slashes from a URL.
func NormalizeURL(url string) string {
	return strings.TrimSuffix(url, "/")
}
