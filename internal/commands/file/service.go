package file

import (
	"context"
	sha1hash "crypto/sha1" //nolint:gosec
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"

	"github.com/uvasoftware/scanii-cli/internal/client"
	"github.com/uvasoftware/scanii-cli/internal/commands/profile"
	"golang.org/x/sync/errgroup"
)

type service struct {
	client *client.Client
}

func newService(profile *profile.Profile) (*service, error) {

	c, err := profile.Client()
	if err != nil {
		return nil, err
	}
	return &service{client: c}, nil
}

type consumer func(record resultRecord)

// processOptions carries the per-run settings for service.process.
type processOptions struct {
	maxConcurrency int
	callback       string
	async          bool
	metadata       map[string]string
	// onBytes, when set, is called with the number of file bytes handed to the
	// HTTP transport. It is called from one goroutine per in-flight file.
	onBytes func(n uint64)
}

// progressReader reports bytes as they are read. The request body is an io.Pipe
// drained by the HTTP transport, so a read only advances once the previous chunk
// has been written to the wire — which makes this an upload progress signal
// rather than a read-ahead count.
type progressReader struct {
	reader  io.Reader
	onBytes func(n uint64)
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.reader.Read(b)
	if n > 0 {
		p.onBytes(uint64(n)) //nolint:gosec
	}
	return n, err
}

func (s *service) retrieve(ctx context.Context, id string) (*resultRecord, error) {
	resp, err := s.client.RetrieveFile(ctx, id)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, nil
	}

	r := resp.Result
	return &resultRecord{
		id:            *r.ID,
		contentType:   *r.ContentType,
		checksum:      *r.Checksum,
		findings:      *r.Findings,
		contentLength: uint64(*r.ContentLength),
		creationDate:  *r.CreationDate,
		metadata:      *r.Metadata,
	}, nil
}

// process is the main function that processes the files in the stream
// an error is returned only in catastrophic situations, individual file errors are recorded in the resultRecord and passed to the consumer for handling
func (s *service) process(ctx context.Context, stream chan string, opts processOptions, consumer consumer) error {

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(opts.maxConcurrency)

	for path := range stream {
		g.Go(func() error {

			r := resultRecord{path: path}

			fd, err := os.Open(path)
			if err != nil {
				slog.Debug("could not open file", "path", path, "error", err.Error())
				r.err = err
				consumer(r)
				return nil
			}

			type writerResult struct {
				sha1 string
				err  error
			}

			pipeReader, pipeWriter := io.Pipe()
			mpb := multipart.NewWriter(pipeWriter)
			contentType := mpb.FormDataContentType()
			writeDone := make(chan writerResult, 1)

			go func() {
				defer close(writeDone)
				defer func() { _ = fd.Close() }()

				sha1 := sha1hash.New() //nolint:gosec
				fdAndShaReader := io.TeeReader(fd, sha1)
				if opts.onBytes != nil {
					fdAndShaReader = &progressReader{reader: fdAndShaReader, onBytes: opts.onBytes}
				}

				sendErr := func(prefix string, err error) {
					_ = pipeWriter.CloseWithError(err)
					writeDone <- writerResult{err: fmt.Errorf("%s: %w", prefix, err)}
				}

				filePartWriter, err := mpb.CreateFormFile("file", filepath.Base(path))
				if err != nil {
					sendErr("create form file", err)
					return
				}

				if _, err = io.Copy(filePartWriter, fdAndShaReader); err != nil {
					sendErr("copy file", err)
					return
				}

				for k, v := range opts.metadata {
					if err = mpb.WriteField(fmt.Sprintf("metadata[%s]", k), v); err != nil {
						sendErr("write metadata", err)
						return
					}
				}

				if opts.callback != "" {
					if err = mpb.WriteField("callback", opts.callback); err != nil {
						sendErr("write callback", err)
						return
					}
				}

				if err = mpb.Close(); err != nil {
					sendErr("close multipart writer", err)
					return
				}

				if err = pipeWriter.Close(); err != nil {
					writeDone <- writerResult{err: fmt.Errorf("close pipe writer: %w", err)}
					return
				}

				writeDone <- writerResult{sha1: fmt.Sprintf("%x", sha1.Sum(nil))}
			}()

			waitForWriter := func() writerResult {
				res, ok := <-writeDone
				if !ok {
					return writerResult{}
				}
				return res
			}

			handleWriterError := func(res writerResult) bool {
				if res.err == nil {
					return false
				}
				slog.Debug("could not build multipart payload", "path", path, "error", res.err.Error())
				r.err = res.err
				consumer(r)
				return true
			}

			if opts.async {
				result, err := s.client.ProcessFileAsync(ctx, contentType, pipeReader)
				if err != nil {
					_ = pipeWriter.CloseWithError(err)
					writeRes := waitForWriter()
					if handleWriterError(writeRes) {
						return nil
					}
					slog.Debug("could not process file", "path", path, "error", err.Error())
					r.err = err
					consumer(r)
					return nil
				}

				writeRes := waitForWriter()
				if handleWriterError(writeRes) {
					return nil
				}

				if result.StatusCode != http.StatusAccepted {
					r.err = apiError(result.StatusCode, result.Header, result.Error)
					slog.Debug("api error processing file", "path", path, "status", result.StatusCode, "error", r.err.Error())
				} else {
					r.id = *result.Pending.ID
					r.location = result.Header.Get("Location")
				}

				consumer(r)
			} else {
				result, localErr := s.client.ProcessFile(ctx, contentType, pipeReader)
				if localErr != nil {
					_ = pipeWriter.CloseWithError(localErr)
					writeRes := waitForWriter()
					if handleWriterError(writeRes) {
						return nil
					}
					slog.Debug("could not process file", "path", path, "error", localErr.Error())
					r.err = localErr
					consumer(r)
					return nil
				}

				writeRes := waitForWriter()
				if handleWriterError(writeRes) {
					return nil
				}

				calculatedSha1 := writeRes.sha1
				slog.Debug("calculated sha1", "sha1", calculatedSha1)

				slog.Debug("response", "status", result.StatusCode)

				if result.StatusCode != http.StatusCreated {
					// there is no result to check the checksum against, and the
					// error the API returned is the one worth reporting
					r.err = apiError(result.StatusCode, result.Header, result.Error)
					slog.Debug("api error processing file", "path", path, "status", result.StatusCode, "error", r.err.Error())
					slog.Debug("skipping checksum verification, the api returned an error", "path", path)
				} else {
					pr := result.Result
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

					// a comparison is only meaningful with both sides in hand —
					// treating a missing checksum as a mismatch would fail a file
					// the API accepted just fine
					switch {
					case r.checksum == "":
						slog.Warn("no checksum in response, skipping verification", "path", path)
					case calculatedSha1 == "":
						slog.Warn("no locally calculated checksum, skipping verification", "path", path)
					case r.checksum != calculatedSha1:
						slog.Debug("checksum mismatch", "path", path, "expected", calculatedSha1, "actual", r.checksum)
						r.err = fmt.Errorf("checksum mismatch, expected %s, actual %s", calculatedSha1, r.checksum)
					default:
						slog.Debug("checksum verified", "expected", calculatedSha1, "actual", r.checksum)
					}
				}

				consumer(r)
			}

			return nil
		})

	}
	err := g.Wait()
	if err != nil {
		return err
	}
	return nil
}
