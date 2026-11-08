package server

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
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

// TestDeleteFile_KnownID verifies that DELETE /v2.2/files/{id} removes a known
// processing result and returns 204, and that a subsequent GET returns 404.
func TestDeleteFile_KnownID(t *testing.T) {
	ts := startServer(t)

	body, ctype := multipartBody(t, nil, []byte("delete me"))
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

	deleteReq, err := http.NewRequestWithContext(t.Context(), http.MethodDelete, ts.URL+"/v2.2/files/"+created.ID, http.NoBody)
	if err != nil {
		t.Fatalf("new delete request: %s", err)
	}
	deleteReq.SetBasicAuth("key", "secret")
	deleteResp, err := http.DefaultClient.Do(deleteReq)
	if err != nil {
		t.Fatalf("delete: %s", err)
	}
	defer deleteResp.Body.Close()
	if deleteResp.StatusCode != http.StatusNoContent {
		raw, _ := io.ReadAll(deleteResp.Body)
		t.Fatalf("delete status: want 204, got %d: %s", deleteResp.StatusCode, raw)
	}

	getReq, err := http.NewRequestWithContext(t.Context(), http.MethodGet, ts.URL+"/v2.2/files/"+created.ID, http.NoBody)
	if err != nil {
		t.Fatalf("new retrieve request: %s", err)
	}
	getReq.SetBasicAuth("key", "secret")
	getResp, err := http.DefaultClient.Do(getReq)
	if err != nil {
		t.Fatalf("retrieve after delete: %s", err)
	}
	defer getResp.Body.Close()
	if getResp.StatusCode != http.StatusNotFound {
		raw, _ := io.ReadAll(getResp.Body)
		t.Fatalf("retrieve-after-delete status: want 404, got %d: %s", getResp.StatusCode, raw)
	}

	traceReq, err := http.NewRequestWithContext(t.Context(), http.MethodGet, ts.URL+"/v2.2/files/"+created.ID+"/trace", http.NoBody)
	if err != nil {
		t.Fatalf("new trace request: %s", err)
	}
	traceReq.SetBasicAuth("key", "secret")
	traceResp, err := http.DefaultClient.Do(traceReq)
	if err != nil {
		t.Fatalf("retrieve trace after delete: %s", err)
	}
	defer traceResp.Body.Close()
	if traceResp.StatusCode != http.StatusOK {
		t.Fatalf("trace-after-delete status: want 200, got %d", traceResp.StatusCode)
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

// do sends a request with basic auth and returns the response status, closing
// the body.
func do(t *testing.T, method, url, user, pass string) int {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), method, url, http.NoBody)
	if err != nil {
		t.Fatalf("new request: %s", err)
	}
	req.SetBasicAuth(user, pass)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %s", method, url, err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	return resp.StatusCode
}

// processFile submits content and returns the processing id.
func processFile(t *testing.T, baseURL string) string {
	t.Helper()
	body, ctype := multipartBody(t, nil, []byte("some content"))
	resp, err := http.DefaultClient.Do(authReq(t, baseURL+"/v2.2/files", body, ctype))
	if err != nil {
		t.Fatalf("process: %s", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("process status: want 201, got %d", resp.StatusCode)
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode created: %s", err)
	}
	return created.ID
}

// TestStoreRoutes_RejectTraversal verifies that ids carrying an encoded path
// separator are not found and never reach a file outside the data directory.
func TestStoreRoutes_RejectTraversal(t *testing.T) {
	parent := t.TempDir()
	data := filepath.Join(parent, "a", "data")
	if err := os.MkdirAll(data, 0o700); err != nil {
		t.Fatal(err)
	}
	victim := filepath.Join(parent, "victim.json")
	const content = `{"ID":"victim","id":"victim","expiration_date":"2999-01-01T00:00:00Z"}`
	if err := os.WriteFile(victim, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	ts := startServerIn(t, data)

	const id = "..%2F..%2Fvictim"
	cases := []struct{ method, path string }{
		{http.MethodDelete, "/v2.2/files/" + id},
		{http.MethodGet, "/v2.2/files/" + id},
		{http.MethodGet, "/v2.2/files/" + id + "/trace"},
		{http.MethodDelete, "/v2.2/files/" + id + "/trace"},
		{http.MethodGet, "/v2.2/auth/tokens/" + id},
		{http.MethodDelete, "/v2.2/auth/tokens/" + id},
	}
	for _, tc := range cases {
		if got := do(t, tc.method, ts.URL+tc.path, "key", "secret"); got != http.StatusNotFound {
			t.Errorf("%s %s: want 404, got %d", tc.method, tc.path, got)
		}
	}

	got, err := os.ReadFile(victim)
	if err != nil || string(got) != content {
		t.Fatalf("file outside the data directory changed: %q, %v", got, err)
	}
}

// TestAuth_ResultIDIsNotAToken verifies that a processing result id, which
// every response carries, cannot be used as an auth token.
func TestAuth_ResultIDIsNotAToken(t *testing.T) {
	ts := startServer(t)
	id := processFile(t, ts.URL)

	if got := do(t, http.MethodGet, ts.URL+"/v2.2/ping", id, ""); got != http.StatusUnauthorized {
		t.Fatalf("ping with a result id: want 401, got %d", got)
	}
	// nor does a token id retrieve a processing result
	if got := do(t, http.MethodGet, ts.URL+"/v2.2/auth/tokens/"+id, "key", "secret"); got != http.StatusNotFound {
		t.Fatalf("retrieve token by result id: want 404, got %d", got)
	}
}

// TestAuth_Token verifies that a freshly created token authenticates, and
// stops doing so once deleted.
func TestAuth_Token(t *testing.T) {
	ts := startServer(t)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, ts.URL+"/v2.2/auth/tokens", strings.NewReader("timeout=60"))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth("key", "secret")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("create token: %s", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create token: want 201, got %d", resp.StatusCode)
	}
	var token struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&token); err != nil {
		t.Fatal(err)
	}

	if got := do(t, http.MethodGet, ts.URL+"/v2.2/ping", token.ID, ""); got != http.StatusOK {
		t.Fatalf("ping with token: want 200, got %d", got)
	}
	if got := do(t, http.MethodGet, ts.URL+"/v2.2/auth/tokens/"+token.ID, "key", "secret"); got != http.StatusOK {
		t.Fatalf("retrieve token: want 200, got %d", got)
	}
	// a token id does not retrieve a processing result
	if got := do(t, http.MethodGet, ts.URL+"/v2.2/files/"+token.ID, "key", "secret"); got != http.StatusNotFound {
		t.Fatalf("retrieve file by token id: want 404, got %d", got)
	}
	if got := do(t, http.MethodDelete, ts.URL+"/v2.2/auth/tokens/"+token.ID, "key", "secret"); got != http.StatusNoContent {
		t.Fatalf("delete token: want 204, got %d", got)
	}
	if got := do(t, http.MethodGet, ts.URL+"/v2.2/ping", token.ID, ""); got != http.StatusUnauthorized {
		t.Fatalf("ping with deleted token: want 401, got %d", got)
	}
}

// TestAuth_BadTokenRecords verifies that token records that are expired,
// lack an expiration or an id, or carry another id are refused without a
// panic.
func TestAuth_BadTokenRecords(t *testing.T) {
	data := t.TempDir()
	ts := startServerIn(t, data)

	other := generateID()
	cases := map[string]func(id string) string{
		"expired": func(id string) string {
			return `{"id":"` + id + `","expiration_date":"2000-01-01T00:00:00Z"}`
		},
		"no expiration": func(id string) string { return `{"id":"` + id + `"}` },
		"bad expiration": func(id string) string {
			return `{"id":"` + id + `","expiration_date":"tomorrow"}`
		},
		"no id":    func(string) string { return `{"expiration_date":"2999-01-01T00:00:00Z"}` },
		"empty":    func(string) string { return `{}` },
		"other id": func(string) string { return `{"id":"` + other + `","expiration_date":"2999-01-01T00:00:00Z"}` },
		"not json": func(string) string { return `not json` },
	}
	for name, record := range cases {
		t.Run(name, func(t *testing.T) {
			id := generateID()
			if err := os.WriteFile(filepath.Join(data, fileName(kindToken, id)), []byte(record(id)), 0o600); err != nil {
				t.Fatal(err)
			}
			if got := do(t, http.MethodGet, ts.URL+"/v2.2/ping", id, ""); got != http.StatusUnauthorized {
				t.Fatalf("ping: want 401, got %d", got)
			}
		})
	}
}

// TestProcessFileFetch_Unreachable verifies that a location that cannot be
// fetched gives a 400 rather than a panic.
func TestProcessFileFetch_Unreachable(t *testing.T) {
	ts := startServer(t)

	// a listener that is closed straight away gives a port nothing answers on
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	req := authReq(t, ts.URL+"/v2.2/files/fetch", strings.NewReader("location=http://"+addr+"/"), "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("fetch: %s", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("fetch status: want 400, got %d", resp.StatusCode)
	}
}
