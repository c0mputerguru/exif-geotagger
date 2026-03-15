package processor

import (
	"strings"
)

// IsBuildDBImageExt returns true if the file extension (with leading dot)
// is a supported image type for the BuildDB command.
// Supported: .jpg, .jpeg, .heic, .png
func IsBuildDBImageExt(ext string) bool {
	ext = strings.ToLower(ext)
	switch ext {
	case ".jpg", ".jpeg", ".heic", ".png":
		return true
	}
	return false
}

// IsTagImageExt returns true if the file extension is a supported image type
// for the TagImages command (raw formats plus jpg).
// Supported: .jpg, .cr2, .cr3, .nef, .arw, .dng
func IsTagImageExt(ext string) bool {
	ext = strings.ToLower(ext)
	switch ext {
	case ".jpg", ".cr2", ".cr3", ".nef", ".arw", ".dng":
		return true
	}
	return false
}
