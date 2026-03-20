# Implementation Plan: Generate ExifTool Script Feature

## Overview
Add a flag to `tag-images` that generates a shell script containing all exiftool invocations for tagging images, with comments for files that cannot be tagged.

## User Story
As a user, I want to generate a shell script of exiftool commands so I can:
- Review commands before execution
- Execute manually with shell features (set -e, verbose, etc.)
- Run in environments where direct execution might be risky
- Audit exactly what changes will be made

## Design Principles
1. **Exact Match**: Script produces identical results to direct tagging
2. **Minimal Changes**: Reuse existing logic; avoid code duplication
3. **Shell-Safe**: Properly escape file paths with spaces, special chars
4. **Informative**: Clear comments for skipped files with reasons
5. **Optional**: Non-breaking addition; defaults to current behavior

---

## Architecture Analysis

### Current Flow (tag-images)
```
Walk raw-dir → Read metadata → Skip if has GPS → Find DB match → WriteMetadata
```

### Key Components
- `tag-images.go`: Flag parsing, `TagImagesConfig`
- `processor.TagImages()`: Core orchestration (processor.go:329)
- `exiftool.WriteMetadata()`: Builds args and executes (exiftool.go:177)
- `matcher.ProviderOptions`: Matching algorithm config

### Data Needed for Script
For each image:
- File path (escaped for shell)
- GPS coordinates (lat/lon ±WGS84)
- Altitude (if available)
- City/State/Country (if available)
- Match info: device model, timestamp, time diff
- Skip reason (if applicable)

---

## Implementation Tasks (Parallelizable)

### Task 1: Define Script Format & Escaping Rules
**Subtasks:**
1.1. Determine shebang (`#!/bin/bash` vs `#!/usr/bin/env bash`)
1.2. Define comment syntax for skipped files (e.g., `# SKIPPED: reason`)
1.3. Define exiftool command format with proper arg escaping
1.4. Add optional header with metadata (timestamp, config, stats)
1.5. Define execution safety features (set -e, set -u options as comment)
1.6. Document script format in README

**Deliverable:** `docs/script-format.md` or inline comments

**Complexity:** Low (design only)

---

### Task 2: Create Script Writer Abstraction
**File:** `pkg/processor/script_writer.go`

**Subtasks:**
2.1. Define `ScriptWriter` interface:
```go
type ScriptWriter interface {
    WriteHeader(cfg TagImagesConfig, total, tagged, skipped int) error
    WriteTagCommand(filePath string, meta exiftool.Metadata, match database.LocationEntry, timeDiff time.Duration) error
    WriteSkipComment(filePath, reason string) error
    Close() error
}
```

2.2. Implement `FileScriptWriter`:
- Writes to specified file
- Escapes file paths with `filepath.ToSlash` + shell quoting
- Escapes string values (city/state/country) for shell
- Handles UTF-8/encoding

2.3. Implement `StdoutScriptWriter`:
- Writes to stdout (for piping)
- Same escaping logic

2.4. Add unit tests:
- Test path escaping with spaces, quotes, special chars
- Test string value escaping
- Test empty/zero values (altitude)
- Test error handling

**Complexity:** Medium

**Dependencies:** None (new package)

---

### Task 3: Generate ExifTool Arg Strings
**File:** `pkg/exiftool/exiftool.go` (extract existing logic)

**Subtasks:**
3.1. Extract arg-building logic from `WriteMetadata()` into new function:
```go
func BuildExiftoolArgs(filePath string, meta Metadata) []string
```
Returns arg slice without executing.

3.2. Ensure it matches exactly what `WriteMetadata` would execute
- Same order: latitude, longitude, latRef, lonRef, altitude, altRef, city, state, country
- Same formatting: `%f` for floats
- Includes `-overwrite_original`

3.3. Add unit tests to verify BuildExiftoolArgs matches WriteMetadata output

**Complexity:** Low

**Dependencies:** Task 2 (uses arg list)

---

### Task 4: Modify processor.TagImages for Script Mode
**File:** `pkg/processor/processor.go`

**Subtasks:**
4.1. Add `ScriptWriter` parameter to `TagImages()`:
```go
func TagImages(ctx context.Context, rawDir, dbPath string, dryRun bool,
    priorityDevices []string, opts matcher.ProviderOptions,
    scriptWriter ScriptWriter) error
```
If `scriptWriter == nil`, current behavior.

4.2. Refactor walk loop to collect results:
- Maintain counters: total processed, tagged, skipped
- For each file, determine outcome BEFORE writing:
  - Already has GPS → skip with reason
  - No timestamp → skip with reason
  - No match → skip with reason
  - Match found → collect metadata for script

4.3. When `scriptWriter != nil`:
- Call `scriptWriter.WriteTagCommand()` instead of `exiftool.WriteMetadata()`
- Call `scriptWriter.WriteSkipComment()` for skipped files
- At end, call `scriptWriter.WriteHeader()` or `Close()` writes stats
- Print summary to stdout (same as current)

4.4. Tests:
- Extend existing tests to verify script mode produces correct counts
- Ensure dry-run + script mode works (both preview, no changes)

**Complexity:** Medium-High

**Dependencies:** Tasks 2, 3

---

### Task 5: Add CLI Flags and Integration
**File:** `tag-images.go`

**Subtasks:**
5.1. Add flags in `parseTagImagesArgs()`:
- `-generate-script` (bool, default false)
- `-script-output` (string, default "" means stdout)

5.2. Validate flags:
- `-generate-script` and `-dry-run` can coexist (script still shows what would run)
- If `-script-output` is a directory, create with timestamped filename?

5.3. Create script writer in `runTagImages()`:
```go
var writer processor.ScriptWriter
if cfg.GenerateScript {
    if cfg.ScriptOutput == "" {
        writer = processor.NewStdoutScriptWriter()
    } else {
        f, err := os.Create(cfg.ScriptOutput)
        // handle error
        writer = processor.NewFileScriptWriter(f)
    }
    defer writer.Close()
}
```

5.4. Pass writer to `processor.TagImages()`

5.5. Update `TagImagesConfig` struct with new fields

5.6. Update tests in `tag-images_test.go` for new flags

**Complexity:** Medium

**Dependencies:** Task 4

---

### Task 6: Documentation & Examples
**File:** `README.md` (update)

**Subtasks:**
6.1. Add new "Script Generation" section
6.2. Document flags with examples:
```bash
# Generate script to file
exif-geotagger tag-images -raw-dir ./raw -db db.sqlite -generate-script -script-output tag.sh

# Preview to stdout
exif-geotagger tag-images -raw-dir ./raw -db db.sqlite -generate-script > tag.sh

# Combine with dry-run for review
exif-geotagger tag-images -raw-dir ./raw -db db.sqlite -dry-run -generate-script -script-output preview.sh
```
6.3. Show sample script output with comments
6.4. Document shell escaping behavior
6.5. Note: script is Bash, may need adjustments for other shells

6.6. Mention that script must be reviewed and made executable (`chmod +x`) before running

**Complexity:** Low

---

### Task 7: Integration Testing
**File:** `pkg/processor/processor_integration_test.go` (or new file)

**Subtasks:**
7.1. Create test scenario:
- Build test database with known locations
- Create raw images directory with various test cases:
  - Image with no timestamp
  - Image with existing GPS
  - Image with no matching location in DB
  - Image with good match

7.2. Run `TagImages` with script writer
7.3. Verify generated script:
- Contains correct exiftool commands for matched files
- Contains skipped comments with correct reasons
- Proper escaping for paths with spaces/special chars
- Header with stats

7.4. Test that script can actually be executed (dry-run) and produces expected output

**Complexity:** Medium

---

### Task 8: Edge Cases & Robustness
**File:** Various

**Subtasks:**
8.1. Unicode/UTF-8 in file paths and metadata (city names, etc.)
8.2. Very long file paths (shell limits)
8.3. Files with quotes, backticks, dollar signs in names
8.4. Altitude nil handling (omit or 0?)
8.5. Timezone handling in script comments (show in local time?)
8.6. Large directories (memory efficiency - streaming vs buffering)
8.7. Concurrent modifications: script generation should not interfere with actual tagging (if both used separately)

**Complexity:** Medium

---

## Parallel Work Strategy

**Phase 1 - Can run in parallel:**
- Task 1 (design/spec) ← can start immediately
- Task 2 (script writer) ← needs only spec from Task 1
- Task 3 (exiftool arg builder) ← independent

**Phase 2 - Sequential:**
- Task 4 (processor.TagImages modification) ← depends on 2,3
- Task 5 (CLI flags) ← depends on 4
- Task 6 (docs) ← depends on 5
- Task 7 (integration tests) ← depends on 5
- Task 8 (edge cases) ← alongside others

---

## Technical Notes

### Shell Escaping Strategy
Use single quotes for paths and string values:
```bash
exiftool -GPSLatitude=37.7749 -GPSLongitude=-122.4194 -City='San Francisco' '/path/with spaces/file.jpg'
```

For single quotes within strings, use: `'foo'\''bar'` pattern.

Implement helper: `shellEscape(s string) string`

### Script Header Format
```bash
#!/bin/bash
# Generated by exif-geotagger on 2025-03-20 10:30:00 UTC
# Config: dry-run=false, db=db.sqlite, search-window=12h, time-threshold=6h
# Stats will be appended after processing
```

Or write stats as footer after processing:
```bash
# Total: 10 files, Tagged: 7, Skipped: 3
```

### Thread Safety
Current `TagImages` is single-threaded (sequential walk). No concurrency issues.

### Backward Compatibility
- All new fields have defaults
- Old binary ignores new flags
- Database format unchanged
- No impact on `build-db` or `print-db`

---

## Acceptance Criteria

1. ✅ `-generate-script` flag exists and is boolean
2. ✅ `-script-output` flag exists (optional, default stdout)
3. ✅ Script contains shebang and is valid Bash syntax
4. ✅ For each taggable file: one exiftool command with all necessary tags
5. ✅ For each skipped file: comment with file path and reason
6. ✅ File paths properly escaped for shell
7. ✅ Script produces identical results to direct execution (verified by dry-run)
8. ✅ Script can be saved to file or piped
9. ✅ Works with all existing flags (dry-run, priority-devices, etc.)
10. ✅ Updated documentation with examples
11. ✅ Unit tests for script writer escaping
12. ✅ Integration test validates end-to-end

---

## Risks & Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| Shell escaping bugs | Security/execution failure | Extensive tests with special chars; use simple single-quote strategy |
| Script doesn't match actual tagging | User confusion | Build script from same code path as execution, duplicate logic minimal |
| Large directories cause memory issues | OOM | Stream to file, don't buffer entire script in memory |
| Cross-platform path issues (Windows) | Windows users can't use script | Note in docs: script is for Unix/Bash; Windows users can use WSL or adapt |
| Time zone confusion in comments | Misinterpretation | Use UTC in comments or local with offset |

---

## Follow-up Enhancements (Out of Scope)

- `-verify-script` mode: run script in dry-run and report differences
- Script templates/customization
- Different shell formats (zsh, fish)
- Compression/obfuscation for sensitive paths
- `-append-to-script` for incremental runs

---

## Estimated Complexity

- **Total Tasks:** 8 main, ~25 subtasks
- **New Code:** ~400-600 lines (including tests)
- **Modified Code:** ~100 lines
- **Parallelizable:** ~60% (Tasks 1-3)
- **Testing:** 30% of effort

---

## Questions for Review

1. Should `-dry-run` imply script generation? (Separate flags seems clearer)
2. Default output: stdout vs file? (stdout allows redirection/piping)
3. Should we include `set -euo pipefail` in script? (Safe default in header comment)
4. Skipped files: comment format? (`#` vs `: <<'SKIP'`)
5. Script footer: summary stats as comment or `exit 0`?
6. Should the script include timestamp/db checks (`[ -f "$DB" ]`)? (No, keep simple)
