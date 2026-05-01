package server

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/uvasoftware/scanii-cli/internal/client"
	"github.com/uvasoftware/scanii-cli/internal/engine"
)

var (
	errorNoFileSent          = "Regrettably, you did not send us any content to process - please see https://docs.scanii.com"
	errorFileAndLocationSent = "File and Location cannot be passed in at the same time"
	errorArgMissing          = "A required argument is missing"
	errorCloudNotDownload    = "Sadly, we could not download content for processing due to a network error."
)

const basePath = "/v2.2/files/"

type FakeHandler struct {
	engine  *engine.Engine
	baseurl string
	store   store
}

func (h FakeHandler) ProcessFileAsync(w http.ResponseWriter, r *http.Request) {
	id := generateID()
	result := engine.Result{ID: id}
	fileFound := false
	metadata := make(map[string]string)

	reader, err := r.MultipartReader()
	if err != nil {
		h.renderClientError(http.StatusMethodNotAllowed, w, err.Error())
		return
	}
	var callback string
	for {
		part, err := reader.NextPart()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
		}
		// odd but can happen:
		if part == nil {
			continue
		}

		slog.Debug("processing part", "name", part.FormName())
		if part.FormName() == "file" {
			fileFound = true

			// performing analysis, it has to happen while we're parsing the stream
			result, err = h.engine.Process(part)
			if err != nil {
				h.renderServerError(w, err.Error())
				return
			}
		}

		if part.FormName() == "callback" {
			builder := strings.Builder{}
			if _, err := io.Copy(&builder, part); err != nil {
				slog.Error("failed to read callback field", "error", err)
				continue
			}
			callback = builder.String()
		}

		if strings.HasPrefix(part.FormName(), "metadata[") {
			k := extractMetadataKey(part.FormName())
			buf := strings.Builder{}
			_, err := io.Copy(&buf, part)
			if err != nil {
				slog.Warn("could not parse metadata", "key", k, "error", err.Error())
			}

			metadata[k] = buf.String()
		}
	}

	if !fileFound {
		h.renderClientError(http.StatusBadRequest, w, errorNoFileSent)
		return
	}

	// engine.Process returns a fresh Result so restore the ID + metadata
	result.ID = id
	result.Metadata = metadata

	slog.Debug("saving result")
	err = h.store.save(id, &result)
	if err != nil {
		h.renderServerError(w, err.Error())
		return
	}

	if result.ContentLength == 0 {
		h.renderClientError(http.StatusBadRequest, w, errorNoFileSent)
		return
	}

	// sending callback if available:
	if callback != "" {
		h.engine.QueueCallback(callback, &result)
	}

	// sending response
	resp := client.ProcessingPendingResponse{
		ID: &id,
	}

	headers := http.Header{}
	headers.Set("Location", h.baseurl+basePath+id)

	err = writeJSON(w, http.StatusAccepted, resp, headers)
	if err != nil {
		h.renderServerError(w, err.Error())
	}
}

func (h FakeHandler) ProcessFileFetch(w http.ResponseWriter, r *http.Request) {

	id := generateID()
	metadata := make(map[string]string)
	result := engine.Result{
		ID: id,
	}

	err := r.ParseForm()
	if err != nil {
		h.renderClientError(http.StatusBadRequest, w, err.Error())
		return
	}

	location := r.Form.Get("location")
	if location == "" {
		h.renderClientError(http.StatusBadRequest, w, errorArgMissing)
		return
	}

	// extracting metadata
	for k, v := range r.Form {
		if strings.HasPrefix(k, "metadata[") {
			metadata[extractMetadataKey(k)] = v[0]
		}
	}

	// fetching content
	slog.Debug("fetching content from", "location", location)
	//nolint:gosec
	request, err := http.NewRequestWithContext(r.Context(), http.MethodGet, location, http.NoBody)
	if err != nil {
		h.renderServerError(w, err.Error())
		return
	}

	httpResponse, err := http.DefaultClient.Do(request)
	if err != nil {
		h.renderClientError(http.StatusBadRequest, w, err.Error())
	}
	defer httpResponse.Body.Close()

	if httpResponse.StatusCode == http.StatusOK {
		// performing analysis, it has to happen while we're parsing the stream
		result, err = h.engine.Process(httpResponse.Body)
		if err != nil {
			h.renderServerError(w, err.Error())
			return
		}
	} else {
		result.Error = errorCloudNotDownload
	}

	// saving result
	result.ID = id
	result.Metadata = metadata
	err = h.store.save(id, &result)
	if err != nil {
		h.renderServerError(w, err.Error())
		return
	}

	// sending callback if available:
	if r.Form.Get("callback") != "" {
		h.engine.QueueCallback(r.Form.Get("callback"), &result)
	}

	headers := http.Header{}
	headers.Set("Location", h.baseurl+basePath+id)

	// sending response
	resp := client.ProcessingPendingResponse{
		ID: &id,
	}

	err = writeJSON(w, http.StatusAccepted, resp, headers)
	if err != nil {
		h.renderServerError(w, err.Error())
	}
}

func (h FakeHandler) RetrieveFile(w http.ResponseWriter, _ *http.Request, id string) {
	if id == "" {
		h.renderClientError(http.StatusBadRequest, w, errorArgMissing)
		return
	}

	result := engine.Result{}
	err := h.store.load(id, &result)
	if err != nil {
		h.renderClientError(http.StatusNotFound, w, "Sadly, we could not find a file by that id %s")
		return
	}

	if result.Error != "" {
		resp := client.ErrorResponse{
			Error:    &result.Error,
			Metadata: &result.Metadata,
			ID:       &result.ID,
		}

		err = writeJSON(w, http.StatusOK, resp, nil)
		if err != nil {
			h.renderServerError(w, err.Error())
		}
		return

	}

	resp := client.ProcessingResponse{
		ID:            &id,
		Findings:      &result.Findings,
		Checksum:      &result.Sha1,
		ContentLength: new(float32(result.ContentLength)),
		ContentType:   &result.ContentType,
		Metadata:      &result.Metadata,
		CreationDate:  &result.CreationDate,
	}

	err = writeJSON(w, http.StatusOK, resp, nil)
	if err != nil {
		h.renderServerError(w, err.Error())
	}
}

func (h FakeHandler) Ping(w http.ResponseWriter, r *http.Request) {
	key := r.Context().Value(keyInContext).(string)

	resp := map[string]string{
		"message": "pong",
		"key":     key,
	}
	err := writeJSON(w, http.StatusOK, resp, nil)
	if err != nil {
		h.renderServerError(w, err.Error())
	}

}

func (h FakeHandler) Account(w http.ResponseWriter, r *http.Request) {

	key := r.Context().Value(keyInContext).(string)

	// account basically returns made up stuff
	creationDate := time.Now().UTC().AddDate(0, 0, -30).Format(time.RFC3339) // keys
	keyDetectionCategoriesEnabled := []string{"malware", "unsafe_language", "unsafe_image"}
	keyTags := []string{"tag1", "tag2"}

	apiKey := &client.APIKey{
		Active:                     new(true),
		CreationDate:               new(time.Now().UTC().AddDate(0, 0, -30).Format(time.RFC3339)),
		DetectionCategoriesEnabled: &keyDetectionCategoriesEnabled,
		LastSeenDate:               new(time.Now().UTC().AddDate(0, 0, -30).Format(time.RFC3339)),
		Tags:                       &keyTags,
	}

	// user:
	user1 := &client.User{
		CreationDate: new(time.Now().UTC().AddDate(0, 0, -30).Format(time.RFC3339)),
		LastLogin:    nil,
	}

	resp := client.AccountInfo{
		Balance:          new(float32(42_000)),
		BillingEmail:     new("admin@example.com"),
		CreationDate:     &creationDate,
		Keys:             &map[string]client.APIKey{key: *apiKey},
		ModificationDate: &creationDate,
		Name:             new("ACME Inc."),
		StartingBalance:  new(float32(100_000)),
		Subscription:     new("Premium"),
		Users:            &map[string]client.User{"bob@example.com": *user1},
	}

	err := writeJSON(w, http.StatusOK, resp, nil)
	if err != nil {
		h.renderServerError(w, err.Error())
	}

}

func (h FakeHandler) CreateToken(w http.ResponseWriter, r *http.Request) {

	err := r.ParseForm()
	if err != nil {
		h.renderServerError(w, err.Error())
	}
	timeoutInSeconds := 300
	if r.Form.Get("timeout") != "" {
		timeoutInSeconds, err = strconv.Atoi(r.Form.Get("timeout")) //nolint:staticcheck
		if err != nil {
			h.renderClientError(http.StatusBadRequest, w, "could not parse timeout")
			return
		}
	}

	id := generateID()
	token := &client.AuthToken{
		CreationDate:   new(time.Now().UTC().Format(time.RFC3339)),
		ExpirationDate: new(time.Now().UTC().Add(time.Second * time.Duration(timeoutInSeconds)).Format(time.RFC3339)),
		ID:             &id,
	}

	err = h.store.save(id, token)
	if err != nil {
		h.renderServerError(w, err.Error())
		return
	}

	err = writeJSON(w, http.StatusCreated, token, nil)
	if err != nil {
		h.renderServerError(w, err.Error())
	}

}

func (h FakeHandler) DeleteToken(w http.ResponseWriter, _ *http.Request, id string) {
	found, err := h.store.remove(id)
	if !found {
		h.renderClientError(http.StatusNotFound, w, "could not find token")
		return
	}

	if err != nil {
		h.renderServerError(w, err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h FakeHandler) RetrieveToken(w http.ResponseWriter, _ *http.Request, id string) {
	token := &client.AuthToken{}
	err := h.store.load(id, token)
	if err != nil {
		h.renderClientError(http.StatusNotFound, w, "could not find token")
		return
	}

	err = writeJSON(w, http.StatusOK, token, nil)
	if err != nil {
		h.renderServerError(w, err.Error())
	}
}

func (h FakeHandler) ProcessFile(w http.ResponseWriter, r *http.Request) {

	result := engine.Result{}
	id := generateID()
	fileFound, locationFound := false, false
	metadata := make(map[string]string)

	reader, err := r.MultipartReader()
	if err != nil {
		h.renderClientError(http.StatusMethodNotAllowed, w, err.Error())
		return
	}
	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		// odd but can happen:
		if part == nil {
			continue
		}

		slog.Debug("processing part", "name", part.FormName())
		if part.FormName() == "file" {
			fileFound = true

			// performing analysis, it has to happen while we're parsing the stream
			result, err = h.engine.Process(part)
			if err != nil {
				h.renderServerError(w, err.Error())
				return
			}

		}

		if strings.HasPrefix(part.FormName(), "metadata[") {
			k := extractMetadataKey(part.FormName())
			buf := strings.Builder{}
			_, err := io.Copy(&buf, part)
			if err != nil {
				slog.Warn("could not parse metadata", "key", k, "error", err.Error())
			}

			metadata[k] = buf.String()
		}
		if part.FormName() == "location" {
			builder := strings.Builder{}
			_, err := io.Copy(&builder, part)
			if err != nil {
				h.renderServerError(w, err.Error())
				return
			}

			locationFound = true
			location := builder.String()
			// fetching content
			slog.Debug("fetching content from", "location", location)
			req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, location, http.NoBody)
			if err != nil {
				h.renderServerError(w, err.Error())
				return
			}
			resp, err := http.DefaultClient.Do(req) //nolint:gosec
			if err != nil {
				h.renderClientError(http.StatusBadRequest, w, err.Error())
				return
			}
			defer resp.Body.Close() //nolint
			if resp.StatusCode == http.StatusOK {
				// performing analysis, it has to happen while we're parsing the stream
				result, err = h.engine.Process(resp.Body)
				if err != nil {
					h.renderServerError(w, err.Error())
					return
				}
			} else {
				result.Error = errorCloudNotDownload
			}
		}
	}

	// todo: this is inefficient as we're doing the analysis twice
	if locationFound && fileFound {
		h.renderClientError(http.StatusBadRequest, w, errorFileAndLocationSent)
		return
	}

	if !fileFound && !locationFound {
		h.renderClientError(http.StatusBadRequest, w, errorNoFileSent)
		return
	}

	// engine.Process returns a fresh Result so restore the ID + metadata
	result.ID = id
	result.Metadata = metadata

	slog.Debug("saving result")
	err = h.store.save(id, &result)
	if err != nil {
		h.renderServerError(w, err.Error())
		return
	}

	if result.ContentLength == 0 {
		h.renderClientError(http.StatusBadRequest, w, errorNoFileSent)
		return
	}

	// sending response
	foo := client.ProcessingResponse{
		ID:            &id,
		Findings:      &result.Findings,
		Checksum:      &result.Sha1,
		ContentLength: new(float32(result.ContentLength)),
		ContentType:   &result.ContentType,
		Metadata:      &metadata,
		CreationDate:  &result.CreationDate,
	}

	headers := http.Header{}
	headers.Set("Location", h.baseurl+basePath+id)

	err = writeJSON(w, http.StatusCreated, foo, headers)
	if err != nil {
		h.renderServerError(w, err.Error())
	}

}
func (h FakeHandler) RetrieveTrace(w http.ResponseWriter, _ *http.Request, id string) {
	result := engine.Result{}
	if err := h.store.load(id, &result); err != nil {
		h.renderClientError(http.StatusNotFound, w, fmt.Sprintf("No trace exists for processing id %s", id))
		return
	}

	creationTime, err := time.Parse(time.RFC3339Nano, result.CreationDate)
	if err != nil {
		creationTime = time.Now().UTC()
	}

	receivedAt := creationTime.Add(-100 * time.Millisecond).Format(time.RFC3339Nano)
	processedAt := creationTime.Format(time.RFC3339Nano)
	receivedMsg := "content received for processing"
	processedMsg := "content processed successfully"

	events := []client.TraceEvent{
		{Timestamp: &receivedAt, Message: &receivedMsg},
		{Timestamp: &processedAt, Message: &processedMsg},
	}

	resp := client.TraceResponse{
		ID:     &id,
		Events: &events,
	}

	if err := writeJSON(w, http.StatusOK, resp, nil); err != nil {
		h.renderServerError(w, err.Error())
	}
}

func (h FakeHandler) renderServerError(w http.ResponseWriter, message string) {
	trace := fmt.Sprintf("%s\n%s", message, debug.Stack())
	slog.Error(trace)
	h.renderClientError(http.StatusInternalServerError, w, message)
}

func (h FakeHandler) renderClientError(status int, w http.ResponseWriter, message string) {
	err := writeJSON(w, status, client.ErrorResponse{
		Error: &message,
	}, nil)
	if err != nil {
		panic(err)
	}
}
