package file

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"
)

func TestProgressReaderCountsBytes(t *testing.T) {
	var seen atomic.Uint64
	r := &progressReader{
		reader:  strings.NewReader(strings.Repeat("a", 5000)),
		onBytes: func(n uint64) { seen.Add(n) },
	}

	n, err := io.Copy(io.Discard, r)
	if err != nil {
		t.Fatalf("copy failed: %s", err)
	}
	if n != 5000 {
		t.Fatalf("expected 5000 bytes copied, got %d", n)
	}
	if seen.Load() != 5000 {
		t.Fatalf("expected 5000 bytes reported, got %d", seen.Load())
	}
}

func TestProgressReaderIgnoresEmptyReads(t *testing.T) {
	var calls atomic.Uint64
	r := &progressReader{
		reader:  bytes.NewReader(nil),
		onBytes: func(uint64) { calls.Add(1) },
	}

	if _, err := io.Copy(io.Discard, r); err != nil {
		t.Fatalf("copy failed: %s", err)
	}
	if calls.Load() != 0 {
		t.Fatalf("expected no reports for an empty reader, got %d", calls.Load())
	}
}

// quietLogger pins the default logger above debug for the duration of a test.
// Other tests in this package turn debug logging on, which deliberately
// suppresses the progress bar.
func quietLogger(t *testing.T) {
	t.Helper()
	original := slog.Default()
	t.Cleanup(func() { slog.SetDefault(original) })
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelInfo})))
}

func TestProgressTrackerLabels(t *testing.T) {
	tests := []struct {
		name        string
		isDirectory bool
		filesDone   uint64
		want        string
	}{
		{name: "single file", isDirectory: false, want: "Uploading sample.txt"},
		{name: "directory start", isDirectory: true, filesDone: 0, want: "Files 0/1,500"},
		{name: "directory partial", isDirectory: true, filesDone: 750, want: "Files 750/1,500"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tracker := newProgressTracker(context.Background(), tc.isDirectory, 1500, 1000, "sample.txt")
			if got := tracker.label(tc.filesDone); got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

func TestProgressTrackerInertWhenNothingToShow(t *testing.T) {
	tests := []struct {
		name       string
		bytesTotal uint64
		debug      bool
	}{
		{name: "no bytes to upload", bytesTotal: 0},
		{name: "debug logging owns the terminal", bytesTotal: 1000, debug: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.debug {
				original := slog.Default()
				t.Cleanup(func() { slog.SetDefault(original) })
				slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug})))
			}

			tracker := newProgressTracker(context.Background(), false, 1, tc.bytesTotal, "sample.txt")
			if tracker.bar != nil {
				t.Fatal("expected no progress bar")
			}

			// the tracker is still driven by the upload, so it must stay safe to call
			tracker.addBytes(100)
			tracker.fileDone(1)
			tracker.stop()
		})
	}
}

func TestProgressTrackerStopIsIdempotent(t *testing.T) {
	quietLogger(t)
	tracker := newProgressTracker(context.Background(), false, 1, 1000, "sample.txt")
	tracker.stop()
	tracker.stop()

	// analyzing after stop must not resurrect a spinner that would never be stopped
	tracker.analyzing()
	if tracker.spinner != nil {
		t.Fatal("expected no spinner after stop")
	}
}

func TestProgressTrackerSwapsToSpinnerWhenUploadCompletes(t *testing.T) {
	quietLogger(t)
	tracker := newProgressTracker(context.Background(), false, 1, 1000, "sample.txt")

	tracker.addBytes(600)
	if tracker.spinner != nil {
		t.Fatal("expected no spinner while the upload is in flight")
	}

	tracker.addBytes(400)
	if tracker.spinner == nil {
		t.Fatal("expected a spinner once the upload completed")
	}

	tracker.stop()
}

func TestProgressTrackerDirectoryKeepsBarForServerWait(t *testing.T) {
	quietLogger(t)
	// with many files in flight the file counter keeps moving, so the bar stays
	tracker := newProgressTracker(context.Background(), true, 2, 1000, "dir")

	tracker.addBytes(1000)
	if tracker.spinner != nil {
		t.Fatal("expected no spinner in directory mode")
	}

	tracker.stop()
}
