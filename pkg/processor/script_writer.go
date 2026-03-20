package processor

import (
	"os"
	"strings"
)

// ScriptWriter provides an interface for writing shell scripts.
// It writes commands with proper escaping for shell safety.
type ScriptWriter interface {
	// WriteCommand writes a command and its arguments as a single shell-safe line.
	// Arguments are escaped to prevent shell injection and ensure correct parsing.
	WriteCommand(command string, args ...string) error

	// WriteLine writes a raw line to the script without escaping.
	// Used for comments, shebang, and other non-command lines.
	WriteLine(line string) error

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

// WriteLine writes a raw line to the file followed by a newline.
func (w *FileScriptWriter) WriteLine(line string) error {
	_, err := w.file.WriteString(line + "\n")
	return err
}

// Close closes the file.
func (w *FileScriptWriter) Close() error {
	if w.file != nil {
		return w.file.Close()
	}
	return nil
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

// WriteLine writes a raw line to stdout followed by a newline.
func (w *StdoutScriptWriter) WriteLine(line string) error {
	_, err := w.writer.WriteString(line + "\n")
	return err
}

// Close is a no-op for StdoutScriptWriter since it doesn't own os.Stdout.
func (w *StdoutScriptWriter) Close() error {
	return nil
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
