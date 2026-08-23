package file

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/uvasoftware/scanii-cli/internal/client"
	"github.com/uvasoftware/scanii-cli/internal/terminal"
)

type resultRecord struct {
	path          string
	err           error
	contentType   string
	findings      []string
	checksum      string
	id            string
	location      string
	contentLength uint64
	creationDate  string
	metadata      map[string]string
	// requestID identifies the API request that produced this record, and
	// timings is where its wall clock went.
	requestID string
	timings   client.Timings
}

// recordRequest carries the per-request diagnostics off the response. The
// request id is what support needs to look a request up on the server side, so
// it is logged whether or not the run asked for a performance summary.
func (r *resultRecord) recordRequest(resp *client.Response) {
	r.requestID = resp.RequestID()
	r.timings = resp.Timings
	slog.Info("processed file", "path", r.path, "status", resp.StatusCode, "request_id", r.requestID)
}

// apiError builds an error from a non-success API response, preferring the
// message the API returned over the bare status code — "status 413: File is too
// large" tells the caller what to do about it, "status code 413" does not. The
// path is left out on purpose: every caller already knows which file it sent.
func apiError(status int, header http.Header, apiErr *client.ErrorResponse) error {
	msg := fmt.Sprintf("status %d", status)
	if apiErr != nil && apiErr.Error != nil && *apiErr.Error != "" {
		msg = fmt.Sprintf("%s: %s", msg, *apiErr.Error)
	}
	if id := header.Get(client.RequestIDHeader); id != "" {
		msg = fmt.Sprintf("%s (request id %s)", msg, id)
	}
	return errors.New(msg)
}

// extractMetadata parses the metadata string and returns a map of key/value pairs.
func extractMetadata(metadata string) map[string]string {
	result := make(map[string]string)
	if metadata == "" {
		return result
	}

	parts := strings.Split(metadata, ",")
	for _, p := range parts {
		kv := strings.Split(p, "=")
		if len(kv) != 2 {
			slog.Warn("invalid metadata entry", "entry", p)
			continue
		}
		result[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
	}
	return result
}

func printFileResult(result *resultRecord) {
	if result.path != "" {
		terminal.Title(fmt.Sprintf("%s:", result.path))
	}

	if result.err != nil {
		terminal.KeyValue("error:", result.err.Error())
		return
	}

	terminal.KeyValue("id:", result.id)

	if result.checksum != "" {
		terminal.KeyValue("checksum/sha1:", result.checksum)
	}

	if result.location != "" {
		terminal.KeyValue("location:", result.location)
	}

	if result.contentType != "" {
		terminal.KeyValue("content type:", result.contentType)
	}

	if result.contentLength != 0 {
		terminal.KeyValueW("content length:", terminal.FormatBytes(result.contentLength), 16)
	}

	if result.creationDate != "" {
		terminal.KeyValueW("creation date:", terminal.FormatTime(result.creationDate), 16)
	}

	if len(result.findings) > 0 {
		terminal.KeyValue("findings:", strings.Join(result.findings, ","))
	} else {
		terminal.KeyValue("findings:", "none")
	}

	if len(result.metadata) > 0 {
		fmt.Println("  metadata:")
		for k, v := range result.metadata {
			fmt.Printf("    %s → %s\n", k, v)
		}
	} else {
		terminal.KeyValue("metadata:", "none")
	}
}
