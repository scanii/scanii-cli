package file

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/uvasoftware/scanii-cli/internal/terminal"
)

// progressTracker renders the progress of a process run. Progress is measured in
// bytes handed to the HTTP transport, which is the only part of the work whose
// completion the CLI can actually observe: once the last byte is sent, the wait
// is on the server, so a single file swaps the bar for a spinner rather than
// sitting at 100%.
//
// A tracker with a nil bar is inert, which is how debug runs are handled — the
// bar would fight with the log stream for the same lines.
type progressTracker struct {
	bar         *terminal.Progress
	isDirectory bool
	filesTotal  uint64
	bytesTotal  uint64
	name        string

	mu      sync.Mutex
	spinner *terminal.Spinner
	stopped bool
}

func newProgressTracker(ctx context.Context, isDirectory bool, filesTotal, bytesTotal uint64, name string) *progressTracker {
	t := &progressTracker{
		isDirectory: isDirectory,
		filesTotal:  filesTotal,
		bytesTotal:  bytesTotal,
		name:        name,
	}

	if bytesTotal == 0 || slog.Default().Enabled(ctx, slog.LevelDebug) {
		return t
	}

	t.bar = terminal.NewProgress(t.label(0), bytesTotal)
	return t
}

// label reports the bar label for the given number of finished files.
func (t *progressTracker) label(filesDone uint64) string {
	if !t.isDirectory {
		return fmt.Sprintf("Uploading %s", t.name)
	}
	return fmt.Sprintf("Files %s/%s", terminal.FormatNumber(int64(filesDone)), terminal.FormatNumber(int64(t.filesTotal))) //nolint:gosec
}

// addBytes records bytes written to the wire. It is called from one goroutine
// per in-flight file.
func (t *progressTracker) addBytes(n uint64) {
	if t.bar == nil {
		return
	}
	t.bar.Add(n)

	// a single file has nothing left to measure once it is fully uploaded
	if !t.isDirectory && t.bar.Current() >= t.bytesTotal {
		t.analyzing()
	}
}

// fileDone updates the file counter shown alongside the bar in directory mode.
func (t *progressTracker) fileDone(filesDone uint64) {
	if t.bar == nil {
		return
	}
	t.bar.SetLabel(t.label(filesDone))
}

// analyzing finishes the bar and spins until stop, for the server-side wait that
// follows the upload.
func (t *progressTracker) analyzing() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.stopped || t.spinner != nil {
		return
	}
	t.bar.Done()
	t.spinner = terminal.NewSpinner(fmt.Sprintf("Analyzing %s", t.name))
}

// stop clears any spinner and finishes the bar. It is idempotent, so callers can
// both defer it and call it at the point a result is about to be printed.
func (t *progressTracker) stop() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.stopped {
		return
	}
	t.stopped = true

	if t.spinner != nil {
		t.spinner.Stop()
	}
	if t.bar != nil {
		t.bar.Done()
	}
}
