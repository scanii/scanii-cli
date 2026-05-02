package file

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/spf13/cobra"
	"github.com/uvasoftware/scanii-cli/internal/client"
	profile2 "github.com/uvasoftware/scanii-cli/internal/commands/profile"
	"github.com/uvasoftware/scanii-cli/internal/terminal"
)

func traceCommand(ctx context.Context, profileName *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:        "trace [flags] [id]",
		Short:      "Retrieve the processing trace for a previously created processing result",
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

			_, err = callFileTrace(ctx, c, args[0])
			return err
		},
	}

	return cmd
}

type traceRecord struct {
	id     string
	events []traceEventRecord
}

type traceEventRecord struct {
	timestamp string
	message   string
}

func callFileTrace(ctx context.Context, c *client.Client, id string) (*traceRecord, error) {
	if id == "" {
		return nil, errors.New("id cannot be empty")
	}

	slog.Debug("retrieving trace", "id", id)

	resp, err := c.RetrieveTrace(ctx, id)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("no trace exists for processing id %s", id)
	}

	if resp.StatusCode != http.StatusOK {
		if resp.Error != nil && resp.Error.Error != nil {
			return nil, fmt.Errorf("error retrieving trace for id %s: %s", id, *resp.Error.Error)
		}
		return nil, fmt.Errorf("error retrieving trace for id %s, status code %d", id, resp.StatusCode)
	}

	tr := resp.Trace
	record := traceRecord{id: id}
	if tr != nil && tr.ID != nil {
		record.id = *tr.ID
	}
	if tr != nil && tr.Events != nil {
		for _, e := range *tr.Events {
			ev := traceEventRecord{}
			if e.Timestamp != nil {
				ev.timestamp = *e.Timestamp
			}
			if e.Message != nil {
				ev.message = *e.Message
			}
			record.events = append(record.events, ev)
		}
	}

	printTraceResult(&record)
	return &record, nil
}

func printTraceResult(r *traceRecord) {
	terminal.KeyValue("id:", r.id)

	if len(r.events) == 0 {
		terminal.KeyValue("events:", "none")
		return
	}

	rows := make([][]string, 0, len(r.events))
	for _, e := range r.events {
		rows = append(rows, []string{terminal.FormatTime(e.timestamp), e.message})
	}
	fmt.Println("  events:")
	terminal.Table([]string{"timestamp", "message"}, rows)
}
