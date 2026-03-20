package processor

import (
	"os"
	"strings"

	"github.com/abpatel/exif-geotagger/pkg/exiftool"
)

// ScriptWriter provides an interface for writing shell scripts.
// It writes commands with proper escaping for shell safety.
type ScriptWriter interface {
	// WriteCommand writes a command and its arguments as a single shell-safe line.
	// Arguments are escaped to prevent shell injection and ensure correct parsing.
	WriteCommand(command string, args ...string) error

	// WriteTagCommand writes an exiftool command to tag an image with metadata.
	// It builds the appropriate exiftool arguments from the metadata and writes
	// a shell command that would tag the given file.
	WriteTagCommand(filePath string, meta exiftool.Metadata) error

	// WriteSkipComment writes a comment explaining why a file was skipped.
	// This produces a commented line in the script (not executable).
	WriteSkipComment(filePath string, reason string) error

	// Close closes the underlying writer and releases any resources.
	Close() error
}

// FileScriptWriter writes shell scripts to a file.
type FileScriptWriter struct {
	file *os.File
}

// NewFileScriptWriter creates a new FileScriptWriter writing to the given file path.
// It creates the file if it doesn't exist, or truncates it if it does.
func NewFileScriptWriter(path string) (*FileScriptWriter, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	return &FileScriptWriter{file: f}, nil
}

// WriteCommand writes a command with escaped arguments followed by a newline.
func (w *FileScriptWriter) WriteCommand(command string, args ...string) error {
	line := shellEscape(command, args...)
	_, err := w.file.WriteString(line + "\n")
	if err != nil {
		return err
	}
	return nil
}

// Close closes the file.
func (w *FileScriptWriter) Close() error {
	if w.file != nil {
		return w.file.Close()
	}
	return nil
}

// WriteTagCommand writes an exiftool command to tag an image with metadata.
// It builds the appropriate exiftool arguments and writes a shell-safe command line.
func (w *FileScriptWriter) WriteTagCommand(filePath string, meta exiftool.Metadata) error {
	args := exiftool.BuildExiftoolArgs(filePath, meta)
	if len(args) == 0 {
		// Nothing to write, but we can write a comment indicating no tags needed
		_, err := w.file.WriteString("# No metadata to write for " + filePath + "\n")
		return err
	}
	return w.WriteCommand("exiftool", args...)
}

// WriteSkipComment writes a comment explaining why a file was skipped.
// The line is commented out (shell comment) and includes the file path and reason.
func (w *FileScriptWriter) WriteSkipComment(filePath string, reason string) error {
	comment := "# SKIP " + filePath + " - " + reason
	_, err := w.file.WriteString(comment + "\n")
	return err
}

// StdoutScriptWriter writes shell scripts to standard output.
type StdoutScriptWriter struct {
	writer *os.File
}

// NewStdoutScriptWriter creates a new StdoutScriptWriter.
// By default it writes to os.Stdout.
func NewStdoutScriptWriter() *StdoutScriptWriter {
	return &StdoutScriptWriter{writer: os.Stdout}
}

// WriteCommand writes a command with escaped arguments followed by a newline.
func (w *StdoutScriptWriter) WriteCommand(command string, args ...string) error {
	line := shellEscape(command, args...)
	_, err := w.writer.WriteString(line + "\n")
	return err
}

// Close is a no-op for StdoutScriptWriter since it doesn't own os.Stdout.
func (w *StdoutScriptWriter) Close() error {
	return nil
}

// WriteTagCommand writes an exiftool command to tag an image with metadata.
// It builds the appropriate exiftool arguments and writes a shell-safe command line.
func (w *StdoutScriptWriter) WriteTagCommand(filePath string, meta exiftool.Metadata) error {
	args := exiftool.BuildExiftoolArgs(filePath, meta)
	if len(args) == 0 {
		// Nothing to write, but we can write a comment indicating no tags needed
		_, err := w.writer.WriteString("# No metadata to write for " + filePath + "\n")
		return err
	}
	return w.WriteCommand("exiftool", args...)
}

// WriteSkipComment writes a comment explaining why a file was skipped.
// The line is commented out (shell comment) and includes the file path and reason.
func (w *StdoutScriptWriter) WriteSkipComment(filePath string, reason string) error {
	comment := "# SKIP " + filePath + " - " + reason
	_, err := w.writer.WriteString(comment + "\n")
	return err
}

// shellEscape escapes a command and its arguments for safe use in a POSIX shell.
// It returns a single string that can be safely used as a shell command line.
// Each argument is wrapped in single quotes if it contains any characters
// that have special meaning to the shell. Empty strings are represented as ''.
func shellEscape(command string, args ...string) string {
	allParts := make([]string, 0, len(args)+1)
	allParts = append(allParts, command)
	for _, arg := range args {
		allParts = append(allParts, escapeShellArg(arg))
	}
	return strings.Join(allParts, " ")
}

// escapeShellArg returns a shell-safe version of a single argument.
// If the argument contains any characters that need shell escaping, it is wrapped
// in single quotes. Empty strings become ''.
func escapeShellArg(arg string) string {
	if arg == "" {
		return "''"
	}
	// Check if we need to quote (contains shell special chars)
	needsQuoting := false
	for _, r := range arg {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' ||
			r == '&' || r == '|' || r == ';' || r == '<' || r == '>' ||
			r == '(' || r == ')' || r == '{' || r == '}' ||
			r == '[' || r == ']' || r == '\\' || r == '\'' || r == '"' ||
			r == '$' || r == '`' || r == '*' || r == '?' || r == '!' ||
			r == '#' || r == '~' || r == '%' || r == '^' {
			needsQuoting = true
			break
		}
	}

	if !needsQuoting {
		return arg
	}

	// Use single quotes. To include a single quote inside, close, escape, reopen.
	// From POSIX shell: 'abc'\''def' = abc'def
	var result strings.Builder
	result.WriteString("'")
	for _, r := range arg {
		if r == '\'' {
			result.WriteString("'\\''")
		} else {
			result.WriteRune(r)
		}
	}
	result.WriteString("'")
	return result.String()
}
