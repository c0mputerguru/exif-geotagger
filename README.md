# EXIF Geotagger

A command-line tool for geotagging images using GPS data extracted from reference images. The tool builds a SQLite database of GPS locations from images that already contain location data, then uses timestamp-based matching to apply those locations to raw images that lack GPS metadata.

## Features

- **Extract GPS data** from reference images (JPEG, HEIC, PNG) into a searchable SQLite database
- **Tag raw images** (CR2, CR3, NEF, ARW, DNG, JPEG) with GPS coordinates based on timestamp proximity
- **Intelligent matching**: Uses time-based lookup with configurable search window and priority device support
- **Dry-run mode**: Preview changes before applying them
- **Preserves existing metadata**: Won't overwrite images that already have GPS data
- **Support for priority devices**: Give preference to specific camera models (e.g., iPhone, Pixel)

## Installation

### Prerequisites

- **Go 1.26+** (for building from source)
- **ExifTool** by Phil Harvey - Required for reading and writing EXIF metadata

#### Installing ExifTool

**macOS:**
```bash
brew install exiftool
```

**Ubuntu/Debian:**
```bash
sudo apt-get install libimage-exiftool-perl
```

**Windows:** Download from https://exiftool.org/

### Building from Source

```bash
git clone https://github.com/abpatel/exif-geotagger.git
cd exif-geotagger
go build -o exif-geotagger .
```

Or install directly:

```bash
go install github.com/abpatel/exif-geotagger@latest
```

## Usage

### Command Overview

```
exif-geotagger <command> [options]
```

Available commands:
- `build-db` - Build a location database from reference images
- `tag-images` - Tag raw images with GPS data from the database

### Building the Database

Extract GPS metadata from a directory of reference images (e.g., photos from a phone with GPS) and create a SQLite database:

```bash
exif-geotagger build-db -input /path/to/reference/images -output db.sqlite
```

**Options:**
- `-input` (required): Directory containing reference images with GPS data
- `-output` (optional): Path to output SQLite database (default: `db.sqlite`)

The tool walks the input directory recursively, processing files with extensions: `.jpg`, `.jpeg`, `.heic`, `.png`. Only images with valid GPS coordinates are added to the database.

**Example:**
```bash
exif-geotagger build-db -input ~/Photos/iPhone\ 14\ Pro -output ~/geotag-db.sqlite
```

### Tagging Images

Apply GPS coordinates from the database to raw images based on timestamp matching:

```bash
exif-geotagger tag-images -raw-dir /path/to/raw/images -db db.sqlite
```

**Options:**
- `-raw-dir` (required): Directory containing raw images to geotag
- `-db` (optional): Path to SQLite database (default: `db.sqlite`)
- `-dry-run` (optional): Preview changes without writing metadata (default: false)
- `-priority-devices` (optional): Comma-separated list of device models to prioritize (e.g., `-priority-devices "iPhone,Pixel"`)

The tool supports raw formats: `.cr2`, `.cr3`, `.nef`, `.arw`, `.dng`, and also `.jpg`. Images that already have GPS tags are skipped.

**Example - Basic usage:**
```bash
exif-geotagger tag-images -raw-dir ~/Photos/RAW -db ~/geotag-db.sqlite
```

**Example - Dry run with priority devices:**
```bash
exif-geotagger tag-images -raw-dir ~/Photos/RAW -db ~/geotag-db.sqlite -dry-run -priority-devices "iPhone 14 Pro,Pixel 8"
```

**Example - Verbose output:**
```bash
exif-geotagger tag-images -raw-dir ~/Photos/RAW 2>&1 | tee tagging.log
```

## Configuration

### Matching Algorithm

The location matcher uses a scoring system to find the best GPS match:

1. **Search window**: Looks for database entries within ±12 hours of the image timestamp (configurable via `ProviderOptions.SearchWindow`)
2. **Time threshold**: Only considers matches within ±6 hours as acceptable (configurable via `ProviderOptions.TimeThreshold`)
3. **Scoring**: Base score of 100 linearly decays to 0 at the threshold; priority devices receive a multiplier (default 5x)
4. **Selection**: The entry with the highest score is chosen

### Provider Options (Library Users)

When using the `matcher` package programmatically, you can customize the matching behavior:

```go
import "github.com/abpatel/exif-geotagger/pkg/matcher"

opts := matcher.ProviderOptions{
    SearchWindow:        24 * time.Hour,    // Search within ±24 hours
    TimeThreshold:       12 * time.Hour,    // Max acceptable time difference
    PriorityMultiplier:  3.0,               // 3x score boost for priority devices
}
provider := matcher.NewSQLiteLocationProvider(repo, opts)
```

## API Documentation

### processor Package

Core orchestration package.

#### Functions

```go
// BuildDB scans a directory for images with GPS data and populates a SQLite database.
// Supported image extensions: .jpg, .jpeg, .heic, .png
// Returns error if database creation fails.
func BuildDB(inputDir, outputDB string) error

// TagImages processes a directory of raw images and applies GPS metadata
// from the database based on timestamp matching.
// Parameters:
//   rawDir - directory containing raw images
//   dbPath - path to SQLite database
//   dryRun - if true, only preview changes without writing
//   priorityDevices - list of device model substrings to prioritize in matching
// Returns error if processing fails.
func TagImages(rawDir, dbPath string, dryRun bool, priorityDevices []string) error
```

### database Package

SQLite persistence layer.

#### Types

```go
// LocationEntry represents a GPS location record associated with a timestamp and device.
type LocationEntry struct {
    Timestamp    time.Time // When the photo was taken
    Latitude     float64   // GPS latitude in decimal degrees
    Longitude    float64   // GPS longitude in decimal degrees
    Altitude     *float64  // Optional altitude in meters
    City         *string   // Optional city name
    State        *string   // Optional state/province
    Country      *string   // Optional country
    DeviceModel  string    // Device model that captured the location
}

// Repository provides database operations.
type Repository struct {
    db *sql.DB
}
```

#### Functions

```go
// Connect opens a SQLite database and initializes the schema if needed.
func Connect(path string) (*Repository, error)

// Insert adds or updates a location entry.
// Uses UPSERT semantics: updates if (timestamp, device_model) conflict exists.
func (r *Repository) Insert(entry LocationEntry) error

// FindClosest returns location entries within ±window of the target time.
// Results are ordered by temporal proximity (closest first).
func (r *Repository) FindClosest(target time.Time, window time.Duration) ([]LocationEntry, error)

// Close releases database resources.
func (r *Repository) Close() error
```

#### Database Schema

```sql
CREATE TABLE IF NOT EXISTS locations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp DATETIME NOT NULL,
    latitude REAL NOT NULL,
    longitude REAL NOT NULL,
    altitude REAL,
    city TEXT,
    state TEXT,
    country TEXT,
    device_model TEXT NOT NULL,
    UNIQUE(timestamp, device_model)
);
CREATE INDEX IF NOT EXISTS idx_timestamp ON locations(timestamp);
```

### exiftool Package

Wrapper around the ExifTool command-line utility for EXIF metadata operations.

#### Types

```go
// Metadata represents a subset of EXIF tags relevant to geotagging.
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
```

#### Functions

```go
// ReadMetadata reads EXIF metadata from a file using exiftool.
// Returns Metadata struct or error if exiftool fails or finds no data.
func ReadMetadata(filePath string) (Metadata, error)

// WriteMetadata writes selected EXIF tags to a file.
// Only non-nil fields in the Metadata struct are written.
// If dryRun is true, only logs intended changes without modifying the file.
// Automatically adds GPSLatitudeRef/GPSLongitudeRef/GPSAltitudeRef based on sign.
func WriteMetadata(filePath string, meta Metadata, dryRun bool) error

// GetTimestamp extracts the best available timestamp from the metadata.
// Tries fields in order: GPSDateTime, SubSecDateTimeOriginal, SubSecCreateDate,
// SubSecModifyDate, DateTimeOriginal, CreateDate, FileModifyDate.
// Parses multiple EXIF date/time formats including timezone variants.
// Returns error if no valid timestamp is found.
func (m *Metadata) GetTimestamp() (time.Time, error)
```

### matcher Package

Timestamp-based location matching engine.

#### Types

```go
// LocationProvider defines the interface for finding the best GPS match.
type LocationProvider interface {
    FindBestMatch(targetTime time.Time, priorityDevices []string) (database.LocationEntry, error)
}

// SQLiteLocationProvider implements LocationProvider using a SQLite repository.
type SQLiteLocationProvider struct {
    repo               *database.Repository
    searchWindow       time.Duration // ±window around target time to search
    timeThreshold      time.Duration // max acceptable time difference
    priorityMultiplier float64       // score multiplier for priority devices
}

// ProviderOptions configures a SQLiteLocationProvider.
type ProviderOptions struct {
    SearchWindow        time.Duration
    TimeThreshold       time.Duration
    PriorityMultiplier  float64
}
```

#### Functions

```go
// DefaultProviderOptions returns the default configuration:
//   SearchWindow: 12 hours
//   TimeThreshold: 6 hours
//   PriorityMultiplier: 5.0
func DefaultProviderOptions() ProviderOptions

// NewSQLiteLocationProvider creates a new provider with optional custom options.
// If no options provided, uses DefaultProviderOptions.
func NewSQLiteLocationProvider(repo *database.Repository, opts ...ProviderOptions) *SQLiteLocationProvider

// FindBestMatch finds the best location entry matching the target time.
// Scans entries within the search window, scores them by time proximity
// (closer = higher) and applies priority device multiplier.
// Only entries within timeThreshold are considered.
// Returns error if no suitable match is found.
func (s *SQLiteLocationProvider) FindBestMatch(targetTime time.Time, priorityDevices []string) (database.LocationEntry, error)
```

#### Scoring Algorithm

For each candidate entry within the search window:

1. Calculate absolute time difference: `diff = |entry.Timestamp - targetTime|`
2. Discard if `diff > timeThreshold`
3. Base score: `score = 100 * (1 - diff/timeThreshold)` (0-100 range)
4. If device matches any in `priorityDevices` (case-insensitive substring), multiply score by `priorityMultiplier`
5. Select entry with highest score

## Troubleshooting

### Common Issues

**"exiftool not found"**
- Ensure ExifTool is installed and in your PATH.
- Test with: `exiftool -version`

**"Failed to build database: no metadata found"**
- Verify reference images actually contain GPS data.
- Check file permissions.
- Run exiftool manually on a sample: `exiftool -G -n image.jpg`

**"No match found for image"**
- The database may not have any entries near the image's timestamp.
- Increase the search window (library users) or verify database contains appropriate time range.
- Check that reference images and raw images have synchronized clocks.

**"Skipping file (already has GPS data)"**
- This is informational; the tool skips images that already have GPS coordinates.
- Use `-dry-run` to preview changes without risk.

**Permission errors on Windows**
- Run Command Prompt or PowerShell as Administrator.
- Or use `-overwrite_original` is already set; ensure files are not read-only.

### Logging

The tool uses `log.Printf` for warnings and errors, and `fmt.Printf` for informational messages. Redirect or tee output to capture logs:

```bash
exif-geotagger tag-images -raw-dir ./raw 2>&1 | tee geotag.log
```

## Project Structure

```
exif-geotagger/
├── main.go              # CLI entry point
├── go.mod               # Go module definition
├── go.sum               # Dependency checksums
├── README.md            # This file
└── pkg/
    ├── processor/       # High-level workflows (BuildDB, TagImages)
    ├── database/        # SQLite repository and schema
    ├── exiftool/        # ExifTool wrapper and metadata types
    └── matcher/         # Timestamp-based location matching
```

## Contributing

Contributions are welcome! Please:

1. Fork the repository
2. Create a feature branch
3. Ensure code passes `go test ./...` (if tests exist)
4. Submit a pull request

## License

[Specify license here - e.g., MIT, Apache 2.0, etc.]

## Acknowledgments

- **ExifTool** by Phil Harvey - https://exiftool.org/
- Built with Go and SQLite
