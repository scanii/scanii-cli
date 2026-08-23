package file

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/spf13/cobra"
	"github.com/uvasoftware/scanii-cli/internal/client"
	profile2 "github.com/uvasoftware/scanii-cli/internal/commands/profile"
	"github.com/uvasoftware/scanii-cli/internal/terminal"
)

func retrieveCommand(ctx context.Context, profileName *string, perf *bool) *cobra.Command {
	var wait int

	cmd := &cobra.Command{
		Use:        "retrieve [flags] [id]",
		Short:      "Retrieve a previously created processing result",
		Args:       cobra.ExactArgs(1),
		ArgAliases: []string{"id"},
		RunE: func(cmd *cobra.Command, args []string) error {

			profile, err := profile2.Load(*profileName)
			if err != nil {
				return err
			}
			c, err := profile.Client()
			if err != nil {
				return err
			}

			// with --wait this covers every poll, so the summary reports the mean
			start := time.Now()
			timings := &perfReport{}
			_, err = callFileRetrieve(ctx, c, args[0], wait, timings)
			if *perf {
				timings.print(time.Since(start))
			}
			return err
		},
	}

	cmd.PersistentFlags().IntVarP(&wait, "wait", "w", 0, "Seconds to poll for the result before giving up")

	return cmd
}

func callFileRetrieve(ctx context.Context, c *client.Client, s string, wait int, timings *perfReport) (*resultRecord, error) {
	if s == "" {
		return nil, errors.New("id cannot be empty")
	}

	startTime := time.Now()

	var spinner *terminal.Spinner
	if wait > 0 {
		spinner = terminal.NewSpinner(fmt.Sprintf("Waiting on id [%s] for up to %ds", s, wait))
	}

	// ensure spinner is always stopped
	defer func() {
		if spinner != nil {
			spinner.Stop()
		}
	}()

	for {
		slog.Debug("retrieving file", "id", s)

		resp, err := c.RetrieveFile(ctx, s)
		if err != nil {
			return nil, err
		}
		timings.addResponse(&resp.Response)

		if resp.StatusCode != http.StatusOK {
			if wait == 0 {
				return nil, fmt.Errorf("error retrieving file with id %s, status code %d", s, resp.StatusCode)
			}
			// continue polling if wait > 0 and we got a non-200 response
		} else {
			pr := resp.Result

			result := resultRecord{
				id: *pr.ID,
			}
			if pr.Metadata != nil {
				result.metadata = *pr.Metadata
			}
			if pr.ContentType != nil {
				result.contentType = *pr.ContentType
			}
			if pr.Checksum != nil {
				result.checksum = *pr.Checksum
			}
			if pr.Findings != nil {
				result.findings = *pr.Findings
			}
			if pr.ContentLength != nil {
				result.contentLength = uint64(*pr.ContentLength)
			}
			if pr.CreationDate != nil {
				result.creationDate = *pr.CreationDate
			}
			if pr.Error != nil {
				result.err = fmt.Errorf("error retrieving file with id %s: %s", s, *pr.Error)
			}

			// If result has a checksum, processing is complete
			if result.checksum != "" || result.err != nil {
				if spinner != nil {
					spinner.Stop()
				}
				terminal.Success(fmt.Sprintf("Result found in %s", terminal.FormatDuration(time.Since(startTime))))
				printFileResult(&result)
				return &result, nil
			}

			// Processing not complete yet — if not waiting, return what we have
			if wait == 0 {
				printFileResult(&result)
				return &result, nil
			}
		}

		// Check timeout
		if time.Since(startTime) > time.Duration(wait)*time.Second {
			return nil, fmt.Errorf("timed out waiting for processing result after %ds", wait)
		}

		time.Sleep(100 * time.Millisecond)
	}
}
