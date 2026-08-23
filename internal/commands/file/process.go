package file

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/spf13/cobra"
	"github.com/uvasoftware/scanii-cli/internal/commands/profile"
	"github.com/uvasoftware/scanii-cli/internal/terminal"
)

func processCommand(ctx context.Context, profile, metadata *string) *cobra.Command {
	concurrencyLimit := 32 * runtime.NumCPU()
	ignoreHidden := false
	perf := false
	var callback string

	cmd := &cobra.Command{
		Use:        "process [flags] [path]",
		Args:       cobra.ExactArgs(1),
		ArgAliases: []string{"file/directory"},
		Short:      "Process a local file or directory synchronously",
		Long: `Process a local file synchronously. The file can be a single file or a directory.
If a directory is provided, all files in the directory will be processed recursively.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			parsedMetadata := extractMetadata(*metadata)
			return process(ctx, *profile, args[0], parsedMetadata, concurrencyLimit, ignoreHidden, false, callback, perf)
		},
	}

	cmd.PersistentFlags().StringVar(&callback, "callback", "", "Callback URL to be invoked when processing is complete")
	cmd.PersistentFlags().IntVarP(&concurrencyLimit, "concurrency", "c", concurrencyLimit, "Number of concurrent requests to use")
	cmd.PersistentFlags().BoolVarP(&ignoreHidden, "ignore-hidden", "i", false, "Ignore hidden files")
	cmd.PersistentFlags().BoolVar(&perf, "perf", false, "Print a timing breakdown of the API requests after the result")

	return cmd
}

func asyncCommand(ctx context.Context, profile, metadata *string) *cobra.Command {
	concurrencyLimit := 32 * runtime.NumCPU()
	ignoreHidden := false
	var callback string

	cmd := &cobra.Command{
		Use:        "async [flags] [file]",
		Short:      "Process a local file or directory asynchronously",
		Args:       cobra.ExactArgs(1),
		ArgAliases: []string{"file/directory"},
		RunE: func(cmd *cobra.Command, args []string) error {
			parsedMetadata := extractMetadata(*metadata)
			return process(ctx, *profile, args[0], parsedMetadata, concurrencyLimit, ignoreHidden, true, callback, false)
		},
	}

	cmd.PersistentFlags().StringVar(&callback, "callback", "", "Callback URL to be invoked when processing is complete")
	cmd.PersistentFlags().IntVarP(&concurrencyLimit, "concurrency", "c", concurrencyLimit, "Number of concurrent requests to use")
	cmd.PersistentFlags().BoolVarP(&ignoreHidden, "ignore-hidden", "i", false, "Ignore hidden files")

	return cmd
}

func process(
	ctx context.Context,
	profileName string,
	path string,
	metadata map[string]string,
	concurrencyLimit int,
	ignoreHidden bool,
	async bool,
	callback string,
	perf bool,
) error {
	// counters
	filesStarted := atomic.Uint64{}
	filesFinished := atomic.Uint64{}
	filesFailed := atomic.Uint64{}
	filesWithFindings := atomic.Uint64{}
	isDirectory := false
	filesTotal := uint64(0)
	bytesTotal := uint64(0)

	p, err := profile.Load(profileName)
	if err != nil {
		return fmt.Errorf("failed to load profile: %w", err)
	}

	terminal.Info(fmt.Sprintf("Using endpoint: %s and API key: %s", p.Endpoint, p.APIKey()))

	// support .
	if path == "." {
		path, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get working directory: %w", err)
		}
	}

	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("failed to stat path: %w", err)
	}

	if info.IsDir() {
		isDirectory = true
		emptyFiles, err := fsWalker(path, ignoreHidden, func(_ string, fi os.FileInfo) {
			bytesTotal += uint64(fi.Size()) //nolint:gosec
			filesTotal++
		})

		if err != nil {
			return fmt.Errorf("failed to walk directory: %w", err)
		}
		terminal.Info(fmt.Sprintf("Processing recursive directory %s with ~%s files | ~%s", path, terminal.FormatNumber(int64(filesTotal)), terminal.FormatBytes(bytesTotal))) //nolint:gosec
		if emptyFiles > 0 {
			terminal.Info(fmt.Sprintf("Skipping %s empty file(s)", terminal.FormatNumber(int64(emptyFiles))))
		}
	} else {
		if ignoreHidden && strings.HasPrefix(filepath.Base(path), ".") {
			slog.Debug("ignoring hidden file", "path", path)
			terminal.Info(fmt.Sprintf("Skipping hidden file %s", path))
			return nil
		}
		// the API rejects empty content with a 400, so there is nothing to learn
		// from sending it
		if info.Size() == 0 {
			slog.Debug("ignoring empty file", "path", path)
			terminal.Info(fmt.Sprintf("Skipping empty file %s", path))
			return nil
		}
		filesTotal = 1
		terminal.Info(fmt.Sprintf("Processing file %s", path))
		bytesTotal += uint64(info.Size()) //nolint:gosec
	}

	// a walk that gives up part way through leaves files unvisited, which is a
	// failure of the run rather than of any one file — counting it as a file
	// would leave the count one ahead of the list of files that failed
	var walkErr error
	var walkMu sync.Mutex

	fileChannel := make(chan string)
	go func() {
		_, err := fsWalker(path, ignoreHidden, func(filePath string, _ os.FileInfo) {
			filesStarted.Add(1)
			fileChannel <- filePath
		})
		if err != nil {
			walkMu.Lock()
			walkErr = err
			walkMu.Unlock()
			slog.Debug("failed to walk directory", "error", err)
		}
		close(fileChannel)
	}()

	// a directory run only reports counts while it works, so the files that
	// actually carry findings — and the ones that could not be processed at all —
	// are collected and listed at the end
	findings := &findingsReport{}
	failures := &failureReport{}
	timings := &perfReport{}

	// results arrive from one goroutine per in-flight file, and a failure is
	// reported in two writes — clearing the progress bar, then the message
	var reportMu sync.Mutex

	// Progress is measured in bytes handed to the transport rather than in files
	// completed, so that a single large file — or a directory of a few of them —
	// advances smoothly instead of flipping from 0 to 100%.
	tracker := newProgressTracker(ctx, isDirectory, filesTotal, bytesTotal, filepath.Base(path))
	defer tracker.stop()

	startTime := time.Now()
	fs, err := newService(p)
	if err != nil {
		return fmt.Errorf("failed to create service: %w", err)
	}
	err = fs.process(ctx, fileChannel, processOptions{
		maxConcurrency: concurrencyLimit,
		callback:       callback,
		async:          async,
		metadata:       metadata,
		onBytes:        tracker.addBytes,
	}, func(result resultRecord) {
		if perf {
			timings.add(&result)
		}

		if result.err != nil {
			slog.Debug("failed to process file", "file", result.path, "error", result.err)
			failed := filesFailed.Add(1)
			failures.add(&result)

			// a long directory run should not keep the first error to itself
			// until the very end — but nor should it scroll the summary away
			if isDirectory && failed <= maxFailuresShownLive {
				reportMu.Lock()
				terminal.ClearLine()
				terminal.Error(fmt.Sprintf("%s — %s", result.path, result.err))
				if failed == maxFailuresShownLive {
					terminal.ClearLine()
					terminal.Warn("further errors are suppressed, see the summary at the end of the run")
				}
				reportMu.Unlock()
			}
		} else {
			filesFinished.Add(1)
		}

		if len(result.findings) > 0 {
			filesWithFindings.Add(1)
			findings.add(&result)
		}
		if isDirectory {
			slog.Debug("progress", "files_started", filesStarted.Load(), "files_finished", filesFinished.Load(), "files_failed", filesFailed.Load(), "files_with_findings", filesWithFindings.Load(), "total_files", filesTotal)
			tracker.fileDone(filesFinished.Load() + filesFailed.Load())
		} else {
			tracker.stop()
			printFileResult(&result)
		}

	})
	if err != nil {
		return err
	}
	tracker.stop()
	elapsed := time.Since(startTime)
	throughput := float64(bytesTotal) / elapsed.Seconds()

	// a single file has already printed its own result above
	if isDirectory {
		if withFindings := findings.sorted(); len(withFindings) > 0 {
			terminal.Section("Files with findings")
			for i := range withFindings {
				printFileResult(&withFindings[i])
			}
		}
	}

	if perf {
		timings.print(elapsed)
	}

	fmt.Println()
	terminal.Success(fmt.Sprintf("Completed in %s, %s file(s) analyzed. Throughput %s/s", terminal.FormatDuration(elapsed), terminal.FormatNumber(int64(filesFinished.Load())), terminal.FormatBytes(uint64(throughput)))) //nolint:gosec

	counts := fmt.Sprintf("Files with findings: %d, unable to process: %d and successfully processed: %d", filesWithFindings.Load(), filesFailed.Load(), filesFinished.Load())
	if filesFailed.Load() > 0 {
		// a green checkmark next to "unable to process: 6" reads as success
		terminal.Warn(counts)
	} else {
		terminal.Success(counts)
	}

	walkMu.Lock()
	walked := walkErr
	walkMu.Unlock()
	if walked != nil {
		terminal.Error(fmt.Sprintf("stopped reading %s, some files were never sent: %s", path, walked))
	}

	if filesFailed.Load() == 0 {
		if walked != nil {
			return fmt.Errorf("failed to read %s: %w", path, walked)
		}
		return nil
	}

	// a single file has already printed its own error above, and a directory run
	// short enough to have reported every failure as it went does not need to
	// repeat itself — the list is for the runs where they scrolled away
	if isDirectory && filesFailed.Load() > maxFailuresShownLive {
		printFailures(failures.sorted())
	}

	return fmt.Errorf("%d of %d file(s) could not be processed", filesFailed.Load(), filesTotal)
}

const (
	// maxFailuresShownLive bounds how many failures are reported while a
	// directory run is still going.
	maxFailuresShownLive = 10

	// maxFailuresListed bounds the end-of-run failure list. A directory where
	// every file was rejected should not scroll the summary off the screen.
	maxFailuresListed = 25
)

// printFailures lists the files a run could not process, with the reason for
// each, so that they can be retried without re-reading the whole log.
func printFailures(failed []resultRecord) {
	if len(failed) == 0 {
		return
	}

	fmt.Println()
	terminal.Error(fmt.Sprintf("%s file(s) could not be processed:", terminal.FormatNumber(int64(len(failed))))) //nolint:gosec

	listed := min(len(failed), maxFailuresListed)
	items := make([]string, 0, listed)
	for i := range failed[:listed] {
		items = append(items, fmt.Sprintf("%s — %s", failed[i].path, failed[i].err))
	}
	terminal.ErrorList(items)

	if remaining := len(failed) - listed; remaining > 0 {
		terminal.Error(fmt.Sprintf("…and %s more", terminal.FormatNumber(int64(remaining)))) //nolint:gosec
	}
}

// fsWalker calls handler for every file under root that is worth sending for
// analysis, and reports how many empty files it skipped along the way. Empty
// content is rejected by the API with a 400, so uploading it only buys an error.
func fsWalker(root string, ignoreHidden bool, handler func(path string, info os.FileInfo)) (emptyFiles int, err error) {
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if ignoreHidden && strings.HasPrefix(filepath.Base(path), ".") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			slog.Debug("ignoring hidden file", "path", path)
			return nil
		}
		if d.IsDir() {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			// a file we cannot stat is one we cannot size or open; the walk
			// carries on rather than failing the whole run over it
			slog.Debug("ignoring unreadable file", "path", path, "error", err)
			return nil
		}
		if info.Size() == 0 {
			slog.Debug("ignoring empty file", "path", path)
			emptyFiles++
			return nil
		}

		handler(path, info)
		return nil
	})
	return emptyFiles, err
}
