package processor

import "testing"

func TestHasExtension(t *testing.T) {
	tests := []struct {
		name       string
		ext        string
		extensions []string
		want       bool
	}{
		{
			name:       "empty list",
			ext:        ".jpg",
			extensions: []string{},
			want:       false,
		},
		{
			name:       "single match",
			ext:        ".jpg",
			extensions: []string{".jpg", ".jpeg"},
			want:       true,
		},
		{
			name:       "multiple matches",
			ext:        ".png",
			extensions: []string{".jpg", ".jpeg", ".png", ".heic"},
			want:       true,
		},
		{
			name:       "no match",
			ext:        ".gif",
			extensions: []string{".jpg", ".jpeg", ".png"},
			want:       false,
		},
		{
			name:       "case sensitive match",
			ext:        ".JPG",
			extensions: []string{".JPG", ".PNG"},
			want:       true,
		},
		{
			name:       "case sensitive no match",
			ext:        ".jpg",
			extensions: []string{".JPG", ".PNG"},
			want:       false,
		},
		{
			name:       "image file extensions list",
			ext:        ".heic",
			extensions: ImageFileExtensions,
			want:       true,
		},
		{
			name:       "raw file extensions list",
			ext:        ".cr2",
			extensions: RawFileExtensions,
			want:       true,
		},
		{
			name:       "jpeg in raw list",
			ext:        ".jpg",
			extensions: RawFileExtensions,
			want:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasExtension(tt.ext, tt.extensions)
			if got != tt.want {
				t.Errorf("hasExtension(%q, %v) = %v, want %v", tt.ext, tt.extensions, got, tt.want)
			}
		})
	}
}
