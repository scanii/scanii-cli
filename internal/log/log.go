package log

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/uvasoftware/scanii-cli/internal/terminal"
)

// ConsoleLogHandler is a slog.Handler that formats log output to something inspired by Spring Boot's console logger
type ConsoleLogHandler struct {
	opts      Options
	w         io.Writer
	mu        sync.Mutex
	pid       int
	groups    []string
	attrs     []slog.Attr
	addSource bool
}

// Options configure the customLogHandle.
type Options struct {
	// Level is the minimum log level to output.
	Level slog.Leveler

	// AddSource adds source file information to log output.
	AddSource bool

	// Color emits ANSI styling. Callers set it from TTY detection, so that a
	// run whose output is piped to a file writes plain text.
	Color bool
}

// NewConsoleLogHandler creates a new customLogHandle that writes to w.
func NewConsoleLogHandler(w io.Writer, opts *Options) *ConsoleLogHandler {
	h := &ConsoleLogHandler{
		w:   w,
		pid: os.Getpid(),
	}
	if opts != nil {
		h.opts = *opts
		h.addSource = opts.AddSource
	}
	return h
}

// color styles text with the given ANSI code, or leaves it alone when the
// handler is writing somewhere that cannot render it.
func (h *ConsoleLogHandler) color(code string, parts ...string) string {
	if !h.opts.Color {
		return strings.Join(parts, "")
	}
	return terminal.ToString(code, parts...)
}

func (h *ConsoleLogHandler) Enabled(_ context.Context, level slog.Level) bool {
	minLevel := slog.LevelInfo
	if h.opts.Level != nil {
		minLevel = h.opts.Level.Level()
	}
	return level >= minLevel
}

func (h *ConsoleLogHandler) Handle(_ context.Context, r slog.Record) error { //nolint:gocritic
	var buf bytes.Buffer

	// Timestamp - format: 2026-02-13 06:30:26.866
	ts := r.Time.Format("2006-01-02 15:04:05.000")
	_, err := fmt.Fprintf(&buf, "%s ", h.color(terminal.Dim, ts))
	if err != nil {
		return err
	}

	// Level (colored by level, right-padded to 5 chars)
	level := r.Level.String()
	levelColor := levelToColor(r.Level)
	_, err = fmt.Fprintf(&buf, "%5s ", h.color(levelColor, level))
	if err != nil {
		return err
	}

	// PID (magenta)
	_, err = fmt.Fprintf(&buf, "%s ", h.color(terminal.Magenta, strconv.Itoa(h.pid)))
	if err != nil {
		return err
	}

	// Source file location
	// Shows the rightmost 50 characters of the path relative to the module root
	// the column is fixed-width, so it is left out entirely rather than padded
	// to fifty blanks when source reporting is off
	if h.addSource {
		source := ""
		fs := runtime.CallersFrames([]uintptr{r.PC})
		f, _ := fs.Next()
		if f.File != "" {
			relPath := shortenPath(f.File)
			source = fmt.Sprintf("%s:%d", relPath, f.Line)
		}
		_, err = fmt.Fprintf(&buf, "%50s ", h.color(terminal.Cyan, truncateOrPad(source, 50)))
		if err != nil {
			return err
		}
	}

	// Separator and message
	_, err = fmt.Fprintf(&buf, "%s", h.color(terminal.Default, ": ", r.Message))
	if err != nil {
		return err
	}

	// Collect all attributes (pre-set + record attrs)
	var attrs []slog.Attr
	attrs = append(attrs, h.attrs...)
	r.Attrs(func(a slog.Attr) bool {
		attrs = append(attrs, a)
		return true
	})

	// Print attributes on the same line (for simple key-value pairs)
	// but handle stack traces especially to preserve newlines
	for _, a := range attrs {
		a = resolveAttr(a, h.groups)
		if a.Equal(slog.Attr{}) {
			continue
		}

		val := a.Value.Resolve()
		if val.Kind() == slog.KindGroup {
			groupAttrs := val.Group()
			if len(groupAttrs) > 0 {
				_, _ = fmt.Fprintf(&buf, " %s={", h.color(terminal.BrightWhite, a.Key))
				for i, ga := range groupAttrs {
					if i > 0 {
						buf.WriteString(", ")
					}
					_, _ = fmt.Fprintf(&buf, "%s=%s", h.color(terminal.BrightWhite, ga.Key), formatValue(ga.Value))
				}
				buf.WriteString("}")
			}
		} else if a.Key == "trace" || a.Key == "stack" || a.Key == "stacktrace" {
			// Stack traces get printed on a new line with preserved formatting
			str := val.String()
			if str != "" {
				buf.WriteString("\n")
				buf.WriteString(str)
			}
		} else {
			_, _ = fmt.Fprintf(&buf, " %s=%s", h.color(terminal.BrightWhite, a.Key), formatValue(val))
		}
	}

	buf.WriteByte('\n')

	h.mu.Lock()
	defer h.mu.Unlock()
	_, err = h.w.Write(buf.Bytes())
	return err
}

func (h *ConsoleLogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	h2 := h.clone()
	h2.attrs = append(h2.attrs, attrs...)
	return h2
}

func (h *ConsoleLogHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	h2 := h.clone()
	h2.groups = append(h2.groups, name)
	return h2
}

func (h *ConsoleLogHandler) clone() *ConsoleLogHandler {
	return &ConsoleLogHandler{
		opts:      h.opts,
		w:         h.w,
		pid:       h.pid,
		groups:    append([]string{}, h.groups...),
		attrs:     append([]slog.Attr{}, h.attrs...),
		addSource: h.addSource,
	}
}

// levelToColor returns the ANSI color for a given log level, matching Spring Boot.
func levelToColor(level slog.Level) string {
	switch {
	case level >= slog.LevelError:
		return terminal.Red
	case level >= slog.LevelWarn:
		return terminal.Yellow
	case level >= slog.LevelInfo:
		return terminal.BrightGreen
	default:
		// DEBUG
		return terminal.Green
	}
}

// shortenPath strips the absolute path down to a project-relative path.
// It looks for known Go project root markers (cmd/, internal/, pkg/) and
// returns from that point. Falls back to the full path if no marker is found.
func shortenPath(fullPath string) string {
	// Normalize to forward slashes
	p := filepath.ToSlash(fullPath)
	for _, marker := range []string{"/cmd/", "/internal/", "/pkg/", "/assets/"} {
		if idx := strings.Index(p, marker); idx != -1 {
			return p[idx+1:]
		}
	}
	return filepath.Base(fullPath)
}

// truncateOrPad ensures s is exactly n characters (truncated or right-padded).
func truncateOrPad(s string, n int) string {
	if len(s) > n {
		return s[len(s)-n:]
	}
	return s + strings.Repeat(" ", n-len(s))
}

// formatValue formats a slog.Value for display.
func formatValue(v slog.Value) string {
	switch v.Kind() {
	case slog.KindTime:
		return v.Time().Format(time.RFC3339)
	case slog.KindDuration:
		return v.Duration().String()
	default:
		return fmt.Sprintf("%v", v.Any())
	}
}

// resolveAttr prepends group names to attribute keys.
func resolveAttr(a slog.Attr, groups []string) slog.Attr {
	if len(groups) > 0 && a.Value.Kind() != slog.KindGroup {
		key := strings.Join(groups, ".") + "." + a.Key
		return slog.Attr{Key: key, Value: a.Value}
	}
	return a
}
