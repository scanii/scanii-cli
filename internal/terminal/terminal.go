// Package terminal provides styled CLI output with TTY-aware ANSI colors,
// progress indicators, and spinners. Ported from the Java CLI's Terminal class.
package terminal

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/term"
)

// ANSI color and style constants
const (
	Reset = "\033[0m"

	Bold = "\033[1m"
	Dim  = "\033[2m"

	Default = "\033[39m"
	Black   = "\033[30m"
	Red     = "\033[31m"
	Green   = "\033[32m"
	Yellow  = "\033[33m"
	Blue    = "\033[34m"
	Magenta = "\033[35m"
	Cyan    = "\033[36m"
	White   = "\033[37m"

	BrightBlack   = "\033[90m"
	BrightRed     = "\033[91m"
	BrightGreen   = "\033[92m"
	BrightYellow  = "\033[93m"
	BrightBlue    = "\033[94m"
	BrightMagenta = "\033[95m"
	BrightCyan    = "\033[96m"
	BrightWhite   = "\033[97m"
)

const (
	defaultLabelWidth = 15
	blockFilled       = '█'
	blockEmpty        = '░'
)

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// stdout and stderr are the output writers. Tests can replace these to capture output.
var stdout io.Writer = os.Stdout
var stderr io.Writer = os.Stderr
var stdin io.Reader = os.Stdin

// ttyOnce caches the TTY detection result.
var ttyOnce sync.Once
var ttyResult bool

// IsTTY returns true if stdout is a terminal and color output is not suppressed.
func IsTTY() bool {
	ttyOnce.Do(func() {
		f, ok := stdout.(*os.File)
		if !ok {
			ttyResult = false
			return
		}
		ttyResult = term.IsTerminal(int(f.Fd())) &&
			os.Getenv("NO_COLOR") == "" &&
			os.Getenv("TERM") != "dumb"
	})
	return ttyResult
}

// getTerminalWidth returns the width of the terminal, defaulting to 80.
func getTerminalWidth() int {
	f, ok := stdout.(*os.File)
	if !ok {
		return 80
	}
	w, _, err := term.GetSize(int(f.Fd()))
	if err != nil || w <= 0 {
		return 80
	}
	return w
}

// ToString wraps text with the given ANSI color code. This function does NOT
// respect TTY detection — it always applies the color. Used by the log handler
// which manages its own output destination.
func ToString(color string, parts ...string) string {
	if color == "" {
		return strings.Join(parts, "")
	}
	return color + strings.Join(parts, "") + Reset
}

// styled wraps text with ANSI codes only when output is a TTY.
func styled(code, text string) string {
	if !IsTTY() {
		return text
	}
	return code + text + Reset
}

// Section prints a bold section header: \n:: Title\n
func Section(title string) {
	_, _ = fmt.Fprintln(stdout)
	_, _ = fmt.Fprintf(stdout, "%s %s\n", styled(Green, "##"), title)
}

// Title prints a titled header: \n# Title\n
func Title(title string) {
	_, _ = fmt.Fprintln(stdout)
	if IsTTY() {
		_, _ = fmt.Fprintf(stdout, "%s %s\n", styled(Green, "#"), title)
	} else {
		_, _ = fmt.Fprintf(stdout, "# %s\n", title)
	}
	_, _ = fmt.Fprintln(stdout)
}

// KeyValue prints an indented key-value pair with the default label width (15).
func KeyValue(label, value string) {
	KeyValueW(label, value, defaultLabelWidth)
}

// KeyValueW prints an indented key-value pair with a custom label width.
func KeyValueW(label, value string, width int) {
	_, _ = fmt.Fprintf(stdout, "  %-"+fmt.Sprint(width)+"s %s\n", label, value)
}

// Success prints a green checkmark followed by the message.
func Success(message string) {
	if IsTTY() {
		_, _ = fmt.Fprintf(stdout, "%s %s\n", styled(Green, "✔"), message)
	} else {
		_, _ = fmt.Fprintf(stdout, "✔ %s\n", message)
	}
}

// Error prints a red "error:" prefix to stderr.
func Error(message string) {
	if IsTTY() {
		_, _ = fmt.Fprintf(stderr, "%s %s\n", styled(Red, "Error:"), message)
	} else {
		_, _ = fmt.Fprintf(stderr, "error: %s\n", message)
	}
}

// Warn prints a yellow "warning:" prefix.
func Warn(message string) {
	if IsTTY() {
		_, _ = fmt.Fprintf(stdout, "%s %s\n", styled(Yellow, "Warning:"), message)
	} else {
		_, _ = fmt.Fprintf(stdout, "warning: %s\n", message)
	}
}

// Info prints a dim/faint message.
func Info(message string) {
	_, _ = fmt.Fprintln(stdout, styled(White, message))
}

// List prints a numbered list of items.
func List(items []string) {
	for i, item := range items {
		_, _ = fmt.Fprintf(stdout, " %d. %s\n", i+1, item)
	}
}

// Table prints an auto-aligned table with dim headers.
func Table(headers []string, rows [][]string) {
	cols := len(headers)
	widths := make([]int, cols)

	// compute max width per column
	for c := 0; c < cols; c++ {
		widths[c] = len(headers[c])
	}
	for _, row := range rows {
		for c := 0; c < cols && c < len(row); c++ {
			if len(row[c]) > widths[c] {
				widths[c] = len(row[c])
			}
		}
	}

	// build format string
	var fmtParts []string
	for c := 0; c < cols; c++ {
		fmtParts = append(fmtParts, fmt.Sprintf("%%-%ds", widths[c]))
	}
	format := strings.Join(fmtParts, "  ")

	// print header in dim
	headerArgs := make([]any, cols)
	for i, h := range headers {
		headerArgs[i] = h
	}
	_, _ = fmt.Fprintln(stdout, styled(Dim, fmt.Sprintf(format, headerArgs...)))

	// print rows
	for _, row := range rows {
		padded := make([]any, cols)
		for c := 0; c < cols; c++ {
			if c < len(row) {
				padded[c] = row[c]
			} else {
				padded[c] = ""
			}
		}
		_, _ = fmt.Fprintf(stdout, format+"\n", padded...)
	}
}

// ReadLine prints a prompt and reads a line of input from stdin.
func ReadLine(prompt string) string {
	_, _ = fmt.Fprint(stdout, prompt)
	scanner := bufio.NewScanner(stdin)
	if scanner.Scan() {
		return scanner.Text()
	}
	return ""
}

// ProgressBar displays a progress bar. In TTY mode it overwrites the current line.
// In non-TTY mode it only prints the final line when current == total.
// Silently returns when total <= 0.
func ProgressBar(label string, current, total uint64) {
	if total <= 0 {
		return
	}

	percent := int(float64(current) / float64(total) * 100)
	stats := fmt.Sprintf("%d%% (%d/%d)", percent, current, total)

	if !IsTTY() {
		if current == total {
			_, _ = fmt.Fprintf(stdout, "%s %s\n", label, stats)
		}
		return
	}

	termWidth := getTerminalWidth()
	barWidth := termWidth - len(label) - len(stats) - 7 // 7 chars for spaces and brackets
	if barWidth < 10 {
		barWidth = 10
	}

	filled := int(float64(current) / float64(total) * float64(barWidth))
	bar := strings.Repeat(string(blockFilled), filled) + strings.Repeat(string(blockEmpty), barWidth-filled)

	_, _ = fmt.Fprintf(stdout, "\r%s  [%s]  %s", label, bar, stats)
	if current == total {
		_, _ = fmt.Fprintln(stdout)
	}
}

// Spinner displays an animated braille spinner with a message.
type Spinner struct {
	message string
	done    chan struct{}
	stopped atomic.Bool
}

// NewSpinner creates and starts a new spinner. In non-TTY mode it prints the message once.
func NewSpinner(message string) *Spinner {
	s := &Spinner{
		message: message,
		done:    make(chan struct{}),
	}

	if !IsTTY() {
		_, _ = fmt.Fprintln(stdout, message)
		return s
	}

	go func() {
		frame := 0
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-s.done:
				return
			case <-ticker.C:
				f := spinnerFrames[frame%len(spinnerFrames)]
				_, _ = fmt.Fprintf(stdout, "\r%s %s", styled(Cyan, f), s.message)
				frame++
			}
		}
	}()

	return s
}

// Stop halts the spinner animation and clears the line.
func (s *Spinner) Stop() {
	if s.stopped.CompareAndSwap(false, true) {
		close(s.done)
		if IsTTY() {
			_, _ = fmt.Fprint(stdout, "\r\033[2K")
		}
	}
}
