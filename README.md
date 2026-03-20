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
- `print-db` - Print database contents as JSON
- `tag-images` - Tag raw images with GPS data from the database

### Building the Database

Extract GPS metadata from a directory of reference images (e.g., photos from a phone with GPS) and create a SQLite database:

```bash
exif-geotagger build-db -input /path/to/reference/images -output db.sqlite
```

The `build-db` command supports two data sources: local images (default) and Home Assistant. Use the `-source` flag to choose between them.

**Options:**
- `-input` (required when `-source=images`): Directory containing reference images with GPS data
- `-output` (optional): Path to output SQLite database (default: `db.sqlite`)
- `-source` (optional): Data source: `images` (default) or `ha`
- `-all` (optional, images source only): Discover and include all device models automatically
- `-models` (optional, images source only): Comma-separated list of device models to include (e.g., `-models "iPhone 14 Pro,Pixel 8"`)

For the `ha` source, additional flags are required; see below.

The tool walks the input directory recursively, processing files with extensions: `.jpg`, `.jpeg`, `.heic`, `.png`. Only images with valid GPS coordinates are added to the database.

**Example:**
```bash
exif-geotagger build-db -input ~/Photos/iPhone\ 14\ Pro -output ~/geotag-db.sqlite
```

### Building Database from Home Assistant

You can build the location database directly from Home Assistant device tracker entities instead of using local images. This is useful if your mobile devices (phones, tablets) report GPS locations to Home Assistant via their companion apps.

#### Prerequisites

1. **Home Assistant instance** with device tracker entities for your mobile devices.
2. **Long-lived access token** with permission to read entity states.
3. Entities must provide latitude and longitude attributes.

#### Creating a Home Assistant Token

1. Log into Home Assistant as the user whose devices you want to track.
2. Click your user profile (bottom-left corner).
3. Scroll down to **Long-lived access tokens**.
4. Click **Create token**.
5. Give it a descriptive name (e.g., "EXIF Geotagger").
6. Copy the token immediately (you won't be able to see it again).

**Important:** Keep this token secure; it grants access to your Home Assistant data.

#### Discovering Entity IDs

Device tracker entities in Home Assistant have IDs like `device_tracker.iphone` or `device_tracker.pixel_8`.

To find them:

1. Go to **Settings → Devices & services → Devices**.
2. Find your mobile devices (iPhone, Android, etc.).
3. Click on a device to see its entities.
4. Look for entities of type `device_tracker` or `sensor`.
5. Note the **Entity ID** (shown in the top-right of the entity details).

Alternatively, use **Developer Tools → States** and filter by `device_tracker` or `source_type: gps`.

**Example entity IDs:**
```
device_tracker.iphone_14_pro
device_tracker.pixel_8
device_tracker.samsung_s24
```

#### Command-Line Options for HA Source

When using `-source=ha`, the following flags become available:

- `-ha-url` (required): Home Assistant base URL (e.g., `http://homeassistant.local:8123` or `https://my-ha.example.com`)
- `-ha-token` (required): Long-lived access token created above
- `-ha-devices` (optional): Comma-separated list of entity IDs to fetch (e.g., `device_tracker.iphone,device_tracker.pixel`). If omitted, all device_tracker entities with GPS data are used.
- `-ha-start` / `-ha-end` (optional): Time range for fetching history (RFC3339 format, e.g., `-ha-start 2024-01-01T00:00:00Z -ha-end 2024-01-31T23:59:59Z`). Default is last 30 days.
- `-ha-days` (optional): Number of days of history to fetch (overrides `-ha-start`/`-ha-end` if present). Default: 365.
- `-output` (optional): Path to output SQLite database (default: `db.sqlite`)

#### Example Commands

**Basic usage with explicit device list:**
```bash
exif-geotagger build-db -source=ha \
  -ha-url http://homeassistant.local:8123 \
  -ha-token "your_long_lived_token_here" \
  -ha-devices device_tracker.iphone_14_pro,device_tracker.pixel_8 \
  -output ha-db.sqlite
```

**Fetch last 7 days of history:**
```bash
exif-geotagger build-db -source=ha \
  -ha-url https://my-ha.example.com \
  -ha-token "TOKEN" \
  -ha-days 7 \
  -output ha-db.sqlite
```

**Fetch specific date range:**
```bash
exif-geotagger build-db -source=ha \
  -ha-url http://192.168.1.100:8123 \
  -ha-token "TOKEN" \
  -ha-start 2024-12-01T00:00:00Z -ha-end 2024-12-15T23:59:59Z \
  -output ha-db.sqlite
```

**Use all device_tracker entities (no -ha-devices):**
```bash
exif-geotagger build-db -source=ha \
  -ha-url http://homeassistant.local:8123 \
  -ha-token "TOKEN" \
  -output ha-db.sqlite
```

#### How It Works

The `-source=ha` mode works as follows:

1. **Authenticates** to your Home Assistant API using the provided token.
2. **Discovers** device_tracker entities (or uses the list from `-ha-devices`).
3. **Fetches location history** from the Home Assistant `logbook` or `recorder` database for each entity over the specified time range.
4. **Extracts** latitude, longitude, and timestamps from the state changes.
5. **Builds** the SQLite database with `LocationEntry` records, using the entity ID as the device model name.
6. **Merges** with existing database entries using UPSERT semantics (same timestamp + device = update).

#### Troubleshooting

**"Error: -ha-url and -ha-token are required when -source=ha"**
- You forgot to provide the HA URL or token. Both are mandatory when using `-source=ha`.

**"Failed to connect to Home Assistant: connection refused"**
- Verify the URL is correct and accessible from this machine.
- If using `http://homeassistant.local`, ensure mDNS/Bonjour works on your network.
- Try using the IP address instead (e.g., `http://192.168.1.50:8123`).
- Ensure Home Assistant is running and port 8123 is open.

**"Invalid token or insufficient permissions"**
- Long-lived tokens are created per user. The token only has access to entities visible to that user.
- If your device entities were created by another user, they may not be visible.
- Create the token under a user that can see all devices (e.g., the owner account).

**"No location history found for entity device_tracker.xxx"**
- The entity may not have recorded any position changes in the specified time range.
- Check Home Assistant's history: go to **Developer Tools → States**, find the entity, and click **Show Graph**.
- Expand the time range or try a larger `-days` value.
- Ensure the entity actually reports GPS coordinates (latitude/longitude attributes).

**"Entity device_tracker.xxx has no latitude/longitude"**
- Some device_tracker entities source location from Wi-Fi or Bluetooth and only provide `source_type: router` without GPS coordinates.
- Only entities with `latitude` and `longitude` attributes will be used.
- Check entity attributes in **Developer Tools → States** (click the entity, view attributes JSON).

**"Build failed: time parsing error"**
- Home Assistant timestamps are in ISO 8601 format. The parser expects `YYYY-MM-DDTHH:MM:SSZ` or similar.
- This shouldn't happen with the HA API response, but if you encounter it, file a bug.

**Slow performance / long runtime**
- See **Performance Tips** below for optimization strategies.
- Building from HA history can be slower than local images if the time range is large or there are many entities.
- Consider limiting `-days` or using `-ha-devices` to fetch only needed devices.

#### Performance Tips

- **Limit time range:** Use `-days` or `-start`/`-end` to fetch only the history you need. Smaller range = faster.
- **Specify devices:** If you know which devices provided the GPS data for your photos, use `-ha-devices` to skip unnecessary entities.
- **Database reuse:** If you're incrementally adding new data, use `-output` to an existing database; it will UPSERT new entries without duplicates.
- **Token placement:** Store your HA token in an environment variable instead of typing it on the command line (prevents it from appearing in shell history):
  ```bash
  export HA_TOKEN="your_token_here"
  exif-geotagger build-db -source=ha -ha-url http://homeassistant:8123 -ha-token "$HA_TOKEN" -output db.sqlite
  ```
- **Network proximity:** Run the tool on the same local network as Home Assistant to reduce latency.
- **HA performance:** If Home Assistant's recorder database is large, history queries can be slow. Consider using a dedicated short-term retention policy or export your history periodically.

#### Combining HA and Image Sources

Once the HA integration is available, you can combine sources by running `build-db` multiple times with different `-source` values and the same `-output` database:

```bash
# Build from local images first
exif-geotagger build-db -input ~/Photos/iPhone -output db.sqlite

# Then augment with HA history (adds new entries, updates conflicts)
exif-geotagger build-db -source=ha -ha-url http://ha:8123 -ha-token "$HA_TOKEN" -output db.sqlite
```

The database uses UPSERT semantics based on `(timestamp, device_model)`, so duplicate entries are safely merged.

#### Known Limitations

- HA location history is stored in the recorder database, which by default retains data for only 10 days (configurable). Older history may be unavailable unless you increased retention.
- GPS coordinates from the Home Assistant mobile app have limited precision compared to original photos (typically 5-10 decimal places).
- The tool reads history via the REST API, which may be rate-limited if you have a very large number of entities or extremely long history. Consider splitting into multiple runs with `-ha-devices`.
- If your device trackers use `source_type: gps` (preferred), coordinates are available. If they use other sources (router, bluetooth), location might be less accurate or unavailable.

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
- `-search-window` (optional): Search window duration (default: `12h`). Accepts durations like `12h`, `30m`.
- `-time-threshold` (optional): Maximum acceptable time difference (default: `6h`). Accepts durations.
- `-priority-multiplier` (optional): Score multiplier for priority devices (default: `5.0`).

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

### Script Generation

Instead of directly executing exiftool commands, you can generate a shell script containing all the commands that would be run. This is useful for:

- Reviewing changes before applying them
- Version-controlling the tagging operations
- Running the tagging on a different machine
- Running in batches with custom logic

**Flags:**
- `-generate-script` (optional): Generate a shell script instead of executing exiftool directly
- `-script-output` (optional): Path to output file for the generated script. If omitted with `-generate-script`, the script is written to stdout.

**Important notes:**
- **Bash required**: The generated script uses Bash syntax and requires `bash` to run (not `sh` or other shells).
- **Make executable**: If saving to a file, run `chmod +x script.sh` before executing.
- **Shell escaping**: All filenames and arguments are properly escaped for shell safety.
- **ExifTool still required**: The generated script calls `exiftool`, so exiftool must be installed on the machine where you run the script.
- **Dry-run compatible**: You can combine `-dry-run` with `-generate-script` to see what would be tagged without even generating commands for files that would be skipped.

**Example - Generate script to file:**
```bash
exif-geotagger tag-images -raw-dir ~/Photos/RAW -db ~/geotag-db.sqlite -generate-script -script-output tagging_script.sh
chmod +x tagging_script.sh
# Review the script, then execute:
./tagging_script.sh
```

**Example - Generate to stdout and pipe through less:**
```bash
exif-geotagger tag-images -raw-dir ~/Photos/RAW -db ~/geotag-db.sqlite -generate-script | less
```

**Example - Dry run combined with script generation:**
```bash
# See which files would be tagged, and generate commands only for those
exif-geotagger tag-images -raw-dir ~/Photos/RAW -db ~/geotag-db.sqlite -dry-run -generate-script -script-output preview.sh
```

**Sample script output:**
```bash
#!/usr/bin/env bash
# Generated by exif-geotagger on 2026-03-20T19:00:00Z (UTC)
# Config: dry-run=false, db=~/geotag-db.sqlite, search-window=12h0m0s, time-threshold=6h0m0s, priority-devices=
# This script tags images with GPS data using exiftool.
# Review and execute manually with: bash script.sh

# Tag IMG_1234.jpg using iPhone 14 Pro (time diff: 15m30s)
exiftool -overwrite_original -GPSLatitude=37.7749 -GPSLongitude=-122.4194 -GPSLatitudeRef=N -GPSLongitudeRef=W 'IMG_1234.jpg'

# Tag IMG_1235.jpg using Pixel 8 (time diff: 2h5m10s)
exiftool -overwrite_original -GPSLatitude=37.7750 -GPSLongitude=-122.4195 -GPSLatitudeRef=N -GPSLongitudeRef=W 'IMG_1235.jpg'

# SKIP: IMG_1236.jpg - already has GPS data

# Total: 3 files, Tagged: 2, Skipped: 1
```

The script includes comments showing which files were tagged (with device model and time difference), which were skipped (and why), and a summary footer. Filenames with spaces or special characters are automatically escaped.

### Print Database

Print all entries in the database as JSON:

```bash
exif-geotagger print-db -db db.sqlite
```

**Options:**
- `-db` (optional): Path to SQLite database (default: `db.sqlite`)

The output is JSON with an array of location entries, each containing timestamp, latitude, longitude, altitude, city, state, country, and device_model.

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
// BuildDB builds a location database from either reference images or Home Assistant.
// Parameters:
//   - ctx: context.Context for cancellation and timeout
//   - cfg: BuildConfig containing all configuration options
// Returns error if database creation fails.
func BuildDB(ctx context.Context, cfg BuildConfig) error

// TagImages processes a directory of raw images and applies GPS metadata
// from the database based on timestamp matching.
// Parameters:
//   - ctx: context.Context
//   - rawDir: directory containing raw images
//   - dbPath: path to SQLite database
//   - dryRun: if true, only preview changes without writing
//   - priorityDevices: list of device model substrings to prioritize
//   - opts: matcher.ProviderOptions for matching algorithm configuration
// Returns error if processing fails.
func TagImages(ctx context.Context, rawDir, dbPath string, dryRun bool, priorityDevices []string, opts matcher.ProviderOptions) error
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

### homeassistant Package

REST API client for Home Assistant device_tracker history.

#### Types

```go
// DeviceTracker represents a discovered device_tracker entity.
type DeviceTracker struct {
    EntityID     string
    FriendlyName string
    SourceType   string
}

// HAState represents a single state entry from HA history.
type HAState struct {
    EntityID    string
    State       string
    Attributes  map[string]interface{}
    LastChanged string
    LastUpdated string
}

// HistoryResponse represents the 2D array response from /api/history/period.
type HistoryResponse [][]HAState
```

#### Functions

```go
// DiscoverDeviceTrackers returns all device_tracker entities from Home Assistant.
// Parameters:
//   - ctx: context.Context
//   - haURL: Home Assistant base URL (e.g., "http://homeassistant:8123")
//   - haToken: Long-lived access token for authentication
//   - client: Optional HTTP client; if nil, a default client with 30s timeout is used.
// Returns:
//   - []DeviceTracker: list of discovered device trackers
//   - error: network or decoding errors
func DiscoverDeviceTrackers(ctx context.Context, haURL, haToken string, client *http.Client) ([]DeviceTracker, error)

// FetchLocationHistory retrieves historical states for the given device trackers
// within the specified time range. start and end should be in UTC.
// It calls /api/history/period and converts each state to a LocationEntry.
// Only states with valid latitude/longitude are included.
func FetchLocationHistory(ctx context.Context, client Client, start, end time.Time, entityIDs []string) ([]database.LocationEntry, error)

// GetTimezone retrieves the Home Assistant timezone configuration.
func (c *Client) GetTimezone(ctx context.Context) (string, error)
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
    ├── homeassistant/   # Home Assistant REST API client
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

## Cleanup

- (ge-xmw) Removed binary-dependent integration tests (build-db_integration_test.go) from mayor/rig and refinery/rig; removed exif-geotagger binary; added to .gitignore to prevent future check-ins.
