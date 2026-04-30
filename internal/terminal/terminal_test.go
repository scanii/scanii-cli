package terminal

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

// captureOut replaces stdout with a buffer, runs the action, and returns the output.
func captureOut(action func()) string {
	var buf bytes.Buffer
	old := stdout
	stdout = &buf
	// Force non-TTY since buffer is not a terminal
	ttyOnce.Do(func() { ttyResult = false })
	defer func() { stdout = old }()
	action()
	return buf.String()
}

// captureErr replaces stderr with a buffer, runs the action, and returns the output.
func captureErr(action func()) string {
	var buf bytes.Buffer
	old := stderr
	stderr = &buf
	defer func() { stderr = old }()
	action()
	return buf.String()
}

func TestSection(t *testing.T) {
	out := captureOut(func() { Section("Account information") })
	if !strings.Contains(out, "## Account information") {
		t.Fatalf("expected section header, got %q", out)
	}
	// should have blank lines before and after
	lines := strings.Split(out, "\n")
	if lines[0] != "" {
		t.Fatalf("expected blank first line, got %q", lines[0])
	}
}

func TestTitle(t *testing.T) {
	out := captureOut(func() { Title("Processing results") })
	if !strings.Contains(out, "Processing results") {
		t.Fatalf("expected title text, got %q", out)
	}
	if !strings.Contains(out, "#") {
		t.Fatalf("expected # prefix, got %q", out)
	}
}

func TestKeyValue(t *testing.T) {
	out := captureOut(func() { KeyValue("id:", "tok-abc123") })
	if !strings.Contains(out, "id:") {
		t.Fatalf("expected label, got %q", out)
	}
	if !strings.Contains(out, "tok-abc123") {
		t.Fatalf("expected value, got %q", out)
	}
	if !strings.HasPrefix(out, "  ") {
		t.Fatalf("expected 2-space indent, got %q", out)
	}
}

func TestKeyValueCustomWidth(t *testing.T) {
	out := captureOut(func() { KeyValueW("Name:", "test", 20) })
	if !strings.Contains(out, "Name:") {
		t.Fatalf("expected label, got %q", out)
	}
	if !strings.Contains(out, "test") {
		t.Fatalf("expected value, got %q", out)
	}
	// value should start at position >= 22 (2 indent + 20 label + space)
	valuePos := strings.Index(out, "test")
	if valuePos < 22 {
		t.Fatalf("expected value at position >= 22, got %d in %q", valuePos, out)
	}
}

func TestSuccess(t *testing.T) {
	out := captureOut(func() { Success("Token deleted") })
	if !strings.Contains(out, "✔") {
		t.Fatalf("expected checkmark, got %q", out)
	}
	if !strings.Contains(out, "Token deleted") {
		t.Fatalf("expected message, got %q", out)
	}
}

func TestError(t *testing.T) {
	errOut := captureErr(func() { Error("not found") })
	if !strings.Contains(errOut, "error:") {
		t.Fatalf("expected error: prefix, got %q", errOut)
	}
	if !strings.Contains(errOut, "not found") {
		t.Fatalf("expected message, got %q", errOut)
	}
}

func TestWarn(t *testing.T) {
	out := captureOut(func() { Warn("token expiring") })
	if !strings.Contains(out, "warning:") {
		t.Fatalf("expected warning: prefix, got %q", out)
	}
	if !strings.Contains(out, "token expiring") {
		t.Fatalf("expected message, got %q", out)
	}
}

func TestInfo(t *testing.T) {
	out := captureOut(func() { Info("Using endpoint: api-us1.scanii.com") })
	if !strings.Contains(out, "Using endpoint: api-us1.scanii.com") {
		t.Fatalf("expected info message, got %q", out)
	}
}

func TestList(t *testing.T) {
	out := captureOut(func() { List([]string{"first", "second", "third"}) })
	if !strings.Contains(out, "1. first") {
		t.Fatalf("expected first item, got %q", out)
	}
	if !strings.Contains(out, "2. second") {
		t.Fatalf("expected second item, got %q", out)
	}
	if !strings.Contains(out, "3. third") {
		t.Fatalf("expected third item, got %q", out)
	}
}

func TestTable(t *testing.T) {
	out := captureOut(func() {
		Table([]string{"NAME", "STATUS"}, [][]string{{"tok-1", "active"}, {"tok-2", "expired"}})
	})
	if !strings.Contains(out, "NAME") {
		t.Fatalf("expected header NAME, got %q", out)
	}
	if !strings.Contains(out, "tok-1") {
		t.Fatalf("expected tok-1, got %q", out)
	}
	if !strings.Contains(out, "expired") {
		t.Fatalf("expected expired, got %q", out)
	}
}

func TestTableColumnAlignment(t *testing.T) {
	out := captureOut(func() {
		Table([]string{"A", "B"}, [][]string{{"short", "x"}, {"much longer value", "y"}})
	})
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d: %v", len(lines), lines)
	}
	// values in column B should start at the same position
	headerBPos := strings.Index(lines[0], "B")
	row1BPos := strings.Index(lines[1], "x")
	row2BPos := strings.Index(lines[2], "y")
	if headerBPos != row1BPos || headerBPos != row2BPos {
		t.Fatalf("columns not aligned: header=%d, row1=%d, row2=%d", headerBPos, row1BPos, row2BPos)
	}
}

func TestProgressBarNonTTY(t *testing.T) {
	out := captureOut(func() {
		ProgressBar("Uploading", 0, 10)
		ProgressBar("Uploading", 5, 10)
		ProgressBar("Uploading", 10, 10)
	})
	// only the completed line should be printed in non-TTY mode
	if !strings.Contains(out, "Uploading") {
		t.Fatalf("expected label, got %q", out)
	}
	if !strings.Contains(out, "100%") {
		t.Fatalf("expected 100%%, got %q", out)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 line in non-TTY mode, got %d: %v", len(lines), lines)
	}
}

func TestProgressBarZeroTotal(t *testing.T) {
	out := captureOut(func() { ProgressBar("Uploading", 0, 0) })
	if out != "" {
		t.Fatalf("expected empty output for zero total, got %q", out)
	}
}

func TestSpinnerStartStop(t *testing.T) {
	out := captureOut(func() {
		s := NewSpinner("Loading...")
		time.Sleep(50 * time.Millisecond)
		s.Stop()
	})
	// In non-TTY mode, message is printed once
	if !strings.Contains(out, "Loading...") {
		t.Fatalf("expected spinner message, got %q", out)
	}
}

func TestSpinnerDoubleStop(t *testing.T) {
	// Ensure double-stop doesn't panic
	captureOut(func() {
		s := NewSpinner("Test")
		s.Stop()
		s.Stop()
	})
}

func TestToString(t *testing.T) {
	result := ToString(Red, "hello")
	if !strings.Contains(result, "hello") {
		t.Fatalf("expected text, got %q", result)
	}
	if !strings.HasPrefix(result, Red) {
		t.Fatalf("expected red prefix, got %q", result)
	}
	if !strings.HasSuffix(result, Reset) {
		t.Fatalf("expected reset suffix, got %q", result)
	}
}

func TestToStringEmptyColor(t *testing.T) {
	result := ToString("", "hello", " world")
	if result != "hello world" {
		t.Fatalf("expected plain text, got %q", result)
	}
}
