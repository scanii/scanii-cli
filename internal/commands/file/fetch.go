package file

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/uvasoftware/scanii-cli/internal/client"
	"github.com/uvasoftware/scanii-cli/internal/commands/profile"
	"github.com/uvasoftware/scanii-cli/internal/terminal"
)

func fetchCommand(ctx context.Context, profileName, metadata *string, perf *bool) *cobra.Command {
	var callback string
	var wait int

	cmd := &cobra.Command{
		Use:        "fetch [flags] [url]",
		Short:      "Submit a URL for asynchronous processing",
		Args:       cobra.ExactArgs(1),
		ArgAliases: []string{"file/directory"},
		RunE: func(cmd *cobra.Command, args []string) error {
			config, err := profile.Load(*profileName)
			if err != nil {
				return err
			}
			terminal.Info(fmt.Sprintf("Using endpoint: %s and API key: %s", config.Endpoint, config.APIKey()))

			c, err := config.Client()
			if err != nil {
				return err
			}

			// one report for the whole invocation: with --wait that covers the
			// submission and every poll that followed it
			start := time.Now()
			timings := &perfReport{}
			defer func() {
				if *perf {
					timings.print(time.Since(start))
				}
			}()

			result, err := callFilesFetch(ctx, c, args[0], callback, *metadata, timings)
			if err != nil {
				return err
			}

			if wait > 0 {
				_, err = callFileRetrieve(ctx, c, result.id, wait, timings)
				return err
			}
			return nil
		},
	}

	cmd.PersistentFlags().StringVar(&callback, "callback", "", "Callback URL to be invoked when processing is complete")
	cmd.PersistentFlags().IntVarP(&wait, "wait", "w", 0, "Seconds to poll for the result before giving up")

	return cmd
}

// callFilesFetch processes a remote url.
func callFilesFetch(ctx context.Context, c *client.Client, location, callback, metadata string, timings *perfReport) (*resultRecord, error) {
	startTime := time.Now()
	slog.Debug("processing location", "url", location)

	// verifying url
	if _, err := url.Parse(location); err != nil {
		return nil, fmt.Errorf("unable to parse url: %w", err)
	}

	m := extractMetadata(metadata)

	// because of how we pass metadata arguments, we must manually encode the payload
	form := url.Values{}
	for k, v := range m {
		form.Add(fmt.Sprintf("metadata[%s]", k), v)
	}
	form.Add("location", location)

	if callback != "" {
		form.Add("callback", callback)
	}

	slog.Debug("form", "form", form.Encode())
	result, err := c.ProcessFileFetch(ctx, "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	timings.addResponse(&result.Response)

	if result.StatusCode != http.StatusAccepted {
		if result.Error != nil && result.Error.Error != nil {
			return nil, fmt.Errorf("error: %s", *result.Error.Error)
		}
		return nil, fmt.Errorf("error: unexpected status code %d", result.StatusCode)
	}

	id := *result.Pending.ID
	terminal.Success(fmt.Sprintf("Request accepted with id %s in %s", id, terminal.FormatDuration(time.Since(startTime))))
	terminal.KeyValue("id:", id)
	if callback != "" {
		terminal.KeyValue("callback:", callback)
	}
	terminal.KeyValue("location:", result.Header.Get("Location"))
	fmt.Println()
	terminal.Info(fmt.Sprintf("Retrieve the result with: sc files retrieve %s", id))

	return &resultRecord{
		id:       id,
		location: result.Header.Get("Location"),
	}, nil
}

func runLocationProcess(ctx context.Context, c *client.Client, location, callback, metadata string) (*resultRecord, error) {
	slog.Debug("processing location", "url", location)

	// verifying url
	if _, err := url.Parse(location); err != nil {
		return nil, fmt.Errorf("unable to parse url: %w", err)
	}

	m := extractMetadata(metadata)

	// because of how we pass metadata arguments, we must manually encode the payload
	body := bytes.Buffer{}
	mp := multipart.NewWriter(&body)

	err := mp.WriteField("location", location)
	if err != nil {
		return nil, err
	}

	for k, v := range m {
		err = mp.WriteField(fmt.Sprintf("metadata[%s]", k), v)
		if err != nil {
			return nil, err
		}
	}

	if callback != "" {
		err = mp.WriteField("callback", callback)
		if err != nil {
			return nil, err
		}
	}
	err = mp.Close()
	if err != nil {
		return nil, err
	}

	result, err := c.ProcessFile(ctx, mp.FormDataContentType(), &body)
	if err != nil {
		return nil, err
	}

	if result.StatusCode != http.StatusCreated {
		if result.Error != nil && result.Error.Error != nil {
			return nil, fmt.Errorf("error: %s", *result.Error.Error)
		}
		return nil, fmt.Errorf("error: unexpected status code %d", result.StatusCode)
	}

	pr := result.Result
	r := resultRecord{}
	r.id = *pr.ID
	if pr.ContentType != nil {
		r.contentType = *pr.ContentType
	}
	if pr.Checksum != nil {
		r.checksum = *pr.Checksum
	}
	if pr.Findings != nil {
		r.findings = *pr.Findings
	}
	if pr.ContentLength != nil {
		r.contentLength = uint64(*pr.ContentLength)
	}
	if pr.CreationDate != nil {
		r.creationDate = *pr.CreationDate
	}
	if pr.Metadata != nil {
		r.metadata = *pr.Metadata
	}

	printFileResult(&r)
	return &r, nil
}
