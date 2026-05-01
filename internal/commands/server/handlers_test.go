package server

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"testing"
)

// TestRetrieveTrace_KnownID verifies that GET /v2.2/files/{id}/trace returns 200
// with a TraceResponse for a known processing id, and includes the required headers.
func TestRetrieveTrace_KnownID(t *testing.T) {
	ts := startServer(t)

	// Create a processing result to trace.
	body, ctype := multipartBody(t, nil, []byte("trace test content"))
	resp, err := http.DefaultClient.Do(authReq(t, ts.URL+"/v2.2/files", body, ctype))
	if err != nil {
		t.Fatalf("process: %s", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("process status: want 201, got %d: %s", resp.StatusCode, raw)
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode created: %s", err)
	}

	// Retrieve the trace.
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, ts.URL+"/v2.2/files/"+created.ID+"/trace", http.NoBody)
	if err != nil {
		t.Fatalf("new request: %s", err)
	}
	req.SetBasicAuth("key", "secret")
	traceResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("retrieve trace: %s", err)
	}
	defer traceResp.Body.Close()
	if traceResp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(traceResp.Body)
		t.Fatalf("trace status: want 200, got %d: %s", traceResp.StatusCode, raw)
	}

	var trace struct {
		ID     string `json:"id"`
		Events []struct {
			Timestamp string `json:"timestamp"`
			Message   string `json:"message"`
		} `json:"events"`
	}
	if err := json.NewDecoder(traceResp.Body).Decode(&trace); err != nil {
		t.Fatalf("decode trace: %s", err)
	}
	if trace.ID != created.ID {
		t.Errorf("trace id: want %q, got %q", created.ID, trace.ID)
	}
	if len(trace.Events) < 2 {
		t.Errorf("trace events: want at least 2, got %d", len(trace.Events))
	}
	for i, e := range trace.Events {
		if e.Timestamp == "" {
			t.Errorf("event[%d].timestamp: want non-empty", i)
		}
		if e.Message == "" {
			t.Errorf("event[%d].message: want non-empty", i)
		}
	}
	if traceResp.Header.Get("X-Scanii-Request-Id") == "" {
		t.Error("X-Scanii-Request-Id header missing on 200")
	}
	if traceResp.Header.Get("X-Scanii-Host-Id") == "" {
		t.Error("X-Scanii-Host-Id header missing on 200")
	}
}

// TestRetrieveTrace_UnknownID verifies that GET /v2.2/files/{id}/trace returns 404
// with an ErrorResponse for an unknown processing id, and includes the required headers.
func TestRetrieveTrace_UnknownID(t *testing.T) {
	ts := startServer(t)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, ts.URL+"/v2.2/files/doesnotexist/trace", http.NoBody)
	if err != nil {
		t.Fatalf("new request: %s", err)
	}
	req.SetBasicAuth("key", "secret")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("retrieve trace: %s", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("trace status: want 404, got %d: %s", resp.StatusCode, raw)
	}

	var errResp struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode error response: %s", err)
	}
	if errResp.Error == "" {
		t.Error("error response: want non-empty error message")
	}
	if resp.Header.Get("X-Scanii-Request-Id") == "" {
		t.Error("X-Scanii-Request-Id header missing on 404")
	}
	if resp.Header.Get("X-Scanii-Host-Id") == "" {
		t.Error("X-Scanii-Host-Id header missing on 404")
	}
}

// TestProcessFile_LocationOnly verifies that POST /v2.2/files with a location field
// (and no file) fetches the URL, scans it, and returns 201 with a ProcessingResponse.
// Uses the server's own /static/eicar.txt so no external network access is required.
func TestProcessFile_LocationOnly(t *testing.T) {
	ts := startServer(t)

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if err := mw.WriteField("location", ts.URL+"/static/eicar.txt"); err != nil {
		t.Fatalf("write field: %s", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close multipart: %s", err)
	}

	req := authReq(t, ts.URL+"/v2.2/files", &buf, mw.FormDataContentType())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post: %s", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("status: want 201, got %d: %s", resp.StatusCode, raw)
	}

	var result struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %s", err)
	}
	if result.ID == "" {
		t.Error("expected response to have an id")
	}
}

// TestProcessFile_FileAndLocation verifies that POST /v2.2/files rejects requests
// that include both a file and a location field with 400.
func TestProcessFile_FileAndLocation(t *testing.T) {
	ts := startServer(t)

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", "test.bin")
	if err != nil {
		t.Fatalf("create form file: %s", err)
	}
	if _, err := fw.Write([]byte("some content")); err != nil {
		t.Fatalf("write file bytes: %s", err)
	}
	if err := mw.WriteField("location", "https://example.com/file.bin"); err != nil {
		t.Fatalf("write location field: %s", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close multipart: %s", err)
	}

	req := authReq(t, ts.URL+"/v2.2/files", &buf, mw.FormDataContentType())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post: %s", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("status: want 400, got %d: %s", resp.StatusCode, raw)
	}
}

// TestProcessFile_NoFileNoLocation verifies that POST /v2.2/files rejects requests
// with neither a file nor a location field with 400.
func TestProcessFile_NoFileNoLocation(t *testing.T) {
	ts := startServer(t)

	// Empty multipart: no file, no location.
	body, ctype := multipartBody(t, nil, nil)
	req := authReq(t, ts.URL+"/v2.2/files", body, ctype)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post: %s", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("status: want 400, got %d: %s", resp.StatusCode, raw)
	}
}
