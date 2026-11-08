package server

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/uvasoftware/scanii-cli/internal/engine"
)

// These tests verify callback behavior against the v2.2 OpenAPI spec
// (openapi/src/v22.yaml):
//
//   - POST /files/async accepts a multipart "callback" field.
//   - POST /files/fetch accepts a url-encoded "callback" field.
//
// The spec does not define a schema for the callback payload itself,
// but the reasonable contract — and what consumers rely on — is that
// the server POSTs JSON describing the processing result to the URL.
// These tests pin that contract.

type callbackHit struct {
	method      string
	contentType string
	userAgent   string
	body        map[string]any
}

type callbackReceiver struct {
	server *httptest.Server
	mu     sync.Mutex
	hits   []callbackHit
	arrive chan struct{}
}

func newCallbackReceiver() *callbackReceiver {
	r := &callbackReceiver{arrive: make(chan struct{}, 10)}
	r.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body, _ := io.ReadAll(req.Body)
		hit := callbackHit{
			method:      req.Method,
			contentType: req.Header.Get("Content-Type"),
			userAgent:   req.Header.Get("User-Agent"),
		}
		_ = json.Unmarshal(body, &hit.body)
		r.mu.Lock()
		r.hits = append(r.hits, hit)
		r.mu.Unlock()
		w.WriteHeader(http.StatusOK)
		select {
		case r.arrive <- struct{}{}:
		default:
		}
	}))
	return r
}

func (r *callbackReceiver) close() {
	r.server.Close()
}

// waitFor blocks until a callback arrives or the timeout elapses.
func (r *callbackReceiver) waitFor(t *testing.T, timeout time.Duration) callbackHit {
	t.Helper()
	select {
	case <-r.arrive:
	case <-time.After(timeout):
		t.Fatalf("timed out waiting for callback delivery after %s", timeout)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.hits[len(r.hits)-1]
}

func (r *callbackReceiver) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.hits)
}

// startServer spins up a scanii mock server on a random port using
// the same wiring as RunServer but without the CLI terminal output.
func startServer(t *testing.T) *httptest.Server {
	t.Helper()
	return startServerIn(t, t.TempDir())
}

// startServerIn is startServer with the data directory chosen by the caller,
// for tests that look at what the server keeps on disk.
func startServerIn(t *testing.T, data string) *httptest.Server {
	t.Helper()
	eng, err := engine.New()
	if err != nil {
		t.Fatalf("engine.New: %s", err)
	}
	mux := http.NewServeMux()
	ts := httptest.NewUnstartedServer(CORS(mux))
	// Setup needs the externally visible base URL for Location headers;
	// we pass it after the listener is bound.
	ts.Start()
	t.Cleanup(ts.Close)
	storage, err := Setup(mux, eng, "key", "secret", data, ts.URL)
	if err != nil {
		t.Fatalf("Setup: %s", err)
	}
	// cleanups run last-in first-out: this closes the data directory before
	// the TempDir cleanup removes it, which Windows requires
	t.Cleanup(func() { _ = storage.Close() })
	return ts
}

// authReq builds an HTTP request with basic auth set to key/secret.
func authReq(t *testing.T, url string, body io.Reader, contentType string) *http.Request {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, url, body)
	if err != nil {
		t.Fatalf("new request: %s", err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	req.SetBasicAuth("key", "secret")
	return req
}

// multipartBody builds a multipart/form-data body with the given
// string fields and a single "file" part containing the given bytes.
func multipartBody(t *testing.T, fields map[string]string, fileBytes []byte) (_ io.Reader, contentType string) {
	t.Helper()
	buf := &bytes.Buffer{}
	mw := multipart.NewWriter(buf)
	if fileBytes != nil {
		fw, err := mw.CreateFormFile("file", "payload.bin")
		if err != nil {
			t.Fatalf("create form file: %s", err)
		}
		if _, err := fw.Write(fileBytes); err != nil {
			t.Fatalf("write file: %s", err)
		}
	}
	for k, v := range fields {
		if err := mw.WriteField(k, v); err != nil {
			t.Fatalf("write field %s: %s", k, err)
		}
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close multipart: %s", err)
	}
	return buf, mw.FormDataContentType()
}

// assertProcessingCallbackShape verifies the callback payload carries
// the expected identifying fields. Returns the decoded id for further
// assertions.
func assertProcessingCallbackShape(t *testing.T, hit callbackHit, wantID string) {
	t.Helper()
	if hit.method != http.MethodPost {
		t.Errorf("callback method: want POST, got %s", hit.method)
	}
	if !strings.HasPrefix(hit.contentType, "application/json") {
		t.Errorf("callback content-type: want application/json, got %q", hit.contentType)
	}
	if hit.userAgent == "" {
		t.Errorf("callback user-agent: want non-empty, got empty")
	}
	if hit.body == nil {
		t.Fatalf("callback body: could not decode JSON")
	}
	got, _ := hit.body["id"].(string)
	if got != wantID {
		t.Errorf("callback id: want %q, got %q", wantID, got)
	}
	// Spec's ProcessingResponse fields we expect to see echoed in the
	// notification payload for a successful scan of real content.
	for _, field := range []string{"checksum", "content_length", "content_type", "findings", "creation_date"} {
		if _, ok := hit.body[field]; !ok {
			t.Errorf("callback body missing field %q: %v", field, hit.body)
		}
	}
}

func TestCallbackAsyncDelivers(t *testing.T) {
	server := startServer(t)
	recv := newCallbackReceiver()
	defer recv.close()

	fields := map[string]string{
		"callback":     recv.server.URL,
		"metadata[k1]": "v1",
	}
	body, ctype := multipartBody(t, fields, []byte("hello async world"))

	resp, err := http.DefaultClient.Do(authReq(t, server.URL+"/v2.2/files/async", body, ctype))
	if err != nil {
		t.Fatalf("post: %s", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("status: want 202, got %d: %s", resp.StatusCode, raw)
	}
	var pending struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&pending); err != nil {
		t.Fatalf("decode pending: %s", err)
	}
	if pending.ID == "" {
		t.Fatal("expected pending response to have an id")
	}

	hit := recv.waitFor(t, 5*time.Second)
	assertProcessingCallbackShape(t, hit, pending.ID)

	// Metadata posted with the request should appear in the callback body.
	md, ok := hit.body["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("callback body metadata: want object, got %T (%v)", hit.body["metadata"], hit.body["metadata"])
	}
	if md["k1"] != "v1" {
		t.Errorf("callback metadata k1: want v1, got %v", md["k1"])
	}
}

func TestCallbackFetchDelivers(t *testing.T) {
	server := startServer(t)
	recv := newCallbackReceiver()
	defer recv.close()

	// Use scanii-cli's own /static/eicar.txt endpoint as the source
	// URL so we don't need external network access. The content here
	// is not important — we only care that the callback fires with
	// the processed result.
	form := url.Values{}
	form.Set("location", server.URL+"/static/eicar.txt")
	form.Set("callback", recv.server.URL)
	form.Set("metadata[purpose]", "callback-test")

	req := authReq(t, server.URL+"/v2.2/files/fetch",
		strings.NewReader(form.Encode()),
		"application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post: %s", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("status: want 202, got %d: %s", resp.StatusCode, raw)
	}
	var pending struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&pending); err != nil {
		t.Fatalf("decode pending: %s", err)
	}

	hit := recv.waitFor(t, 5*time.Second)
	assertProcessingCallbackShape(t, hit, pending.ID)

	md, ok := hit.body["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("callback body metadata: want object, got %T", hit.body["metadata"])
	}
	if md["purpose"] != "callback-test" {
		t.Errorf("callback metadata purpose: want callback-test, got %v", md["purpose"])
	}
}

func TestCallbackNotFiredWhenAbsent(t *testing.T) {
	server := startServer(t)
	recv := newCallbackReceiver()
	defer recv.close()

	body, ctype := multipartBody(t, nil, []byte("no callback please"))
	resp, err := http.DefaultClient.Do(authReq(t, server.URL+"/v2.2/files/async", body, ctype))
	if err != nil {
		t.Fatalf("post: %s", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("status: want 202, got %d", resp.StatusCode)
	}

	// Give the engine ample time to deliver a callback if it were going to.
	time.Sleep(250 * time.Millisecond)
	if recv.count() != 0 {
		t.Fatalf("expected zero callbacks, got %d", recv.count())
	}
}

func TestCallbackFetchWithDownloadErrorStillDelivers(t *testing.T) {
	server := startServer(t)
	recv := newCallbackReceiver()
	defer recv.close()

	// Point location at a URL that 404s so the engine records an error.
	form := url.Values{}
	form.Set("location", server.URL+"/static/does-not-exist")
	form.Set("callback", recv.server.URL)

	req := authReq(t, server.URL+"/v2.2/files/fetch",
		strings.NewReader(form.Encode()),
		"application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post: %s", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("status: want 202, got %d: %s", resp.StatusCode, raw)
	}

	hit := recv.waitFor(t, 5*time.Second)
	if hit.method != http.MethodPost {
		t.Errorf("callback method: want POST, got %s", hit.method)
	}
	if hit.body == nil {
		t.Fatalf("callback body: could not decode JSON")
	}
	errMsg, _ := hit.body["error"].(string)
	if errMsg == "" {
		t.Errorf("callback body error: want non-empty, got empty (body=%v)", hit.body)
	}
}

func TestCallbackAsyncRejectsUnreachableURLGracefully(t *testing.T) {
	server := startServer(t)

	// Point at a port unlikely to be listening. The server must not crash
	// or surface the delivery failure to the client — delivery is fire-
	// and-forget per Principle 3 (integration-only, no retries).
	body, ctype := multipartBody(t,
		map[string]string{"callback": "http://127.0.0.1:1/no-listener"},
		[]byte("fire and forget"))

	resp, err := http.DefaultClient.Do(authReq(t, server.URL+"/v2.2/files/async", body, ctype))
	if err != nil {
		t.Fatalf("post: %s", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("status: want 202 even with bad callback, got %d: %s", resp.StatusCode, raw)
	}

	// Follow up with a valid scan to confirm the server + engine are
	// still serving requests after the failed delivery attempt.
	body2, ctype2 := multipartBody(t, nil, []byte("still alive"))
	resp2, err := http.DefaultClient.Do(authReq(t, server.URL+"/v2.2/files/async", body2, ctype2))
	if err != nil {
		t.Fatalf("follow-up post: %s", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusAccepted {
		t.Fatalf("follow-up status: want 202, got %d", resp2.StatusCode)
	}
}
