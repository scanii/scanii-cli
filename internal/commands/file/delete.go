package file

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/spf13/cobra"
	"github.com/uvasoftware/scanii-cli/internal/client"
	profile2 "github.com/uvasoftware/scanii-cli/internal/commands/profile"
	"github.com/uvasoftware/scanii-cli/internal/terminal"
)

func deleteCommand(ctx context.Context, profileName *string, perf *bool) *cobra.Command {
	cmd := &cobra.Command{
		Use:        "delete [flags] [id]",
		Short:      "Delete a previously created processing result",
		Args:       cobra.ExactArgs(1),
		ArgAliases: []string{"id"},
		RunE: func(_ *cobra.Command, args []string) error {
			profile, err := profile2.Load(*profileName)
			if err != nil {
				return err
			}
			c, err := profile.Client()
			if err != nil {
				return err
			}

			start := time.Now()
			timings := &perfReport{}
			_, err = callFileDelete(ctx, c, args[0], timings)
			if *perf {
				timings.print(time.Since(start))
			}
			if err != nil {
				return err
			}
			terminal.Success(fmt.Sprintf("Processing result %s deleted", args[0]))
			return nil
		},
	}

	return cmd
}

func callFileDelete(ctx context.Context, c *client.Client, id string, timings *perfReport) (bool, error) {
	if id == "" {
		return false, fmt.Errorf("id cannot be empty")
	}

	slog.Debug("deleting file", "id", id)

	resp, err := c.DeleteFile(ctx, id)
	if err != nil {
		return false, err
	}
	if timings != nil {
		timings.addResponse(&resp.Response)
	}
	if resp.StatusCode != http.StatusNoContent {
		slog.Error("failed to delete processing result", "id", id, "status", resp.StatusCode)
		return false, fmt.Errorf("failed to delete processing result %s, status code %d", id, resp.StatusCode)
	}
	return true, nil
}

func deleteTraceCommand(ctx context.Context, profileName *string, perf *bool) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete-trace [flags] [id]",
		Short: "Delete the processing trace for a file",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			profile, err := profile2.Load(*profileName)
			if err != nil {
				return err
			}
			c, err := profile.Client()
			if err != nil {
				return err
			}
			start := time.Now()
			resp, err := c.DeleteFileTrace(ctx, args[0])
			if *perf {
				fmt.Printf("Request completed in %s\n", time.Since(start))
			}
			if err != nil {
				return err
			}
			if resp.StatusCode != http.StatusNoContent {
				return apiError(resp.StatusCode, resp.Header, nil)
			}
			terminal.Success(fmt.Sprintf("Processing trace %s deleted", args[0]))
			return nil
		},
	}
	return cmd
}
