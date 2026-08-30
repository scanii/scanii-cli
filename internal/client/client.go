package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"strings"
)

// RequestEditorFn is a callback for modifying requests before sending.
type RequestEditorFn func(ctx context.Context, req *http.Request) error

// ClientOption allows setting custom parameters during construction.
type ClientOption func(*Client) error

// Client is a hand-written HTTP client for the Scanii v2.2 API.
type Client struct {
	baseURL    string
	httpClient *http.Client
	editors    []RequestEditorFn
}

// New creates a new Client with the given base URL and options.
func New(baseURL string, opts ...ClientOption) (*Client, error) {
	c := &Client{
		baseURL:    strings.TrimSuffix(baseURL, "/"),
		httpClient: &http.Client{},
	}
	for _, o := range opts {
		if err := o(c); err != nil {
			return nil, err
		}
	}
	return c, nil
}

// WithHTTPClient sets a custom *http.Client.
func WithHTTPClient(hc *http.Client) ClientOption {
	return func(c *Client) error {
		c.httpClient = hc
		return nil
	}
}

// WithRequestEditorFn adds a request editor callback.
func WithRequestEditorFn(fn RequestEditorFn) ClientOption {
	return func(c *Client) error {
		c.editors = append(c.editors, fn)
		return nil
	}
}

// RequestIDHeader carries the API's identifier for a single request, which is
// what support needs to look one up on the server side.
const RequestIDHeader = "X-Scanii-Request-Id"

// Response is the base response containing HTTP metadata.
type Response struct {
	StatusCode int
	Header     http.Header
	// Timings is the wall-clock breakdown of the exchange that produced this
	// response.
	Timings Timings
}

// RequestID returns the API's identifier for the request, or an empty string if
// the response carried none.
func (r *Response) RequestID() string {
	return r.Header.Get(RequestIDHeader)
}

// PingResult is the response from the ping endpoint.
type PingResult struct {
	Response
	Message string `json:"message"`
	Key     string `json:"key"`
}

// AccountResult is the response from the account endpoint.
type AccountResult struct {
	Response
	Account *AccountInfo
}

// ProcessFileResult is the response from the synchronous file processing endpoint.
type ProcessFileResult struct {
	Response
	Result *ProcessingResponse
	Error  *ErrorResponse
}

// ProcessFileAsyncResult is the response from the async file processing endpoint.
type ProcessFileAsyncResult struct {
	Response
	Pending *ProcessingPendingResponse
	Error   *ErrorResponse
}

// ProcessFileFetchResult is the response from the file fetch endpoint.
type ProcessFileFetchResult struct {
	Response
	Pending *ProcessingPendingResponse
	Error   *ErrorResponse
}

// RetrieveFileResult is the response from the file retrieve endpoint.
type RetrieveFileResult struct {
	Response
	Result *ProcessingResponse
}

// RetrieveTraceResult is the response from the trace retrieve endpoint.
type RetrieveTraceResult struct {
	Response
	Trace *TraceResponse
	Error *ErrorResponse
}

// DeleteFileResult is the response from the file delete endpoint.
type DeleteFileResult struct {
	Response
}

// DeleteFileTraceResult is the response from the trace delete endpoint.
type DeleteFileTraceResult struct {
	Response
}

// CreateTokenResult is the response from the create token endpoint.
type CreateTokenResult struct {
	Response
	Token *AuthToken
}

// RetrieveTokenResult is the response from the retrieve token endpoint.
type RetrieveTokenResult struct {
	Response
	Token *AuthToken
}

// do executes an HTTP request and returns the response metadata and body bytes.
// Every exchange is timed, so that callers which care about where the wall clock
// went can read it off the response.
func (c *Client) do(ctx context.Context, method, path, contentType string, body io.Reader) (*Response, []byte, error) {
	var t tracer
	ctx = httptrace.WithClientTrace(ctx, t.trace())

	url := c.baseURL + path
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, nil, fmt.Errorf("creating request: %w", err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	for _, editor := range c.editors {
		if err := editor(ctx, req); err != nil {
			return nil, nil, fmt.Errorf("applying request editor: %w", err)
		}
	}
	t.begin()
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	data, readErr := io.ReadAll(resp.Body)
	t.bodyRead()
	r := &Response{StatusCode: resp.StatusCode, Header: resp.Header, Timings: t.result()}
	if readErr != nil {
		return r, nil, fmt.Errorf("reading response body: %w", readErr)
	}
	return r, data, nil
}

// Ping validates API credentials.
func (c *Client) Ping(ctx context.Context) (*PingResult, error) {
	resp, body, err := c.do(ctx, http.MethodGet, "/ping", "", nil)
	if err != nil {
		return nil, err
	}
	result := &PingResult{Response: *resp}
	if resp.StatusCode == http.StatusOK && len(body) > 0 {
		_ = json.Unmarshal(body, result)
	}
	return result, nil
}

// Account retrieves account information.
func (c *Client) Account(ctx context.Context) (*AccountResult, error) {
	resp, body, err := c.do(ctx, http.MethodGet, "/account.json", "", nil)
	if err != nil {
		return nil, err
	}
	result := &AccountResult{Response: *resp}
	if resp.StatusCode == http.StatusOK && len(body) > 0 {
		var info AccountInfo
		if err := json.Unmarshal(body, &info); err != nil {
			return nil, fmt.Errorf("parsing account response: %w", err)
		}
		result.Account = &info
	}
	return result, nil
}

// ProcessFile processes a file synchronously.
func (c *Client) ProcessFile(ctx context.Context, contentType string, body io.Reader) (*ProcessFileResult, error) {
	resp, respBody, err := c.do(ctx, http.MethodPost, "/files", contentType, body)
	if err != nil {
		return nil, err
	}
	result := &ProcessFileResult{Response: *resp}
	if len(respBody) > 0 {
		switch {
		case resp.StatusCode == http.StatusCreated:
			var pr ProcessingResponse
			if err := json.Unmarshal(respBody, &pr); err != nil {
				return nil, fmt.Errorf("parsing process file response: %w", err)
			}
			result.Result = &pr
		case resp.StatusCode >= 400:
			var er ErrorResponse
			if err := json.Unmarshal(respBody, &er); err == nil {
				result.Error = &er
			}
		}
	}
	return result, nil
}

// ProcessFileAsync processes a file asynchronously.
func (c *Client) ProcessFileAsync(ctx context.Context, contentType string, body io.Reader) (*ProcessFileAsyncResult, error) {
	resp, respBody, err := c.do(ctx, http.MethodPost, "/files/async", contentType, body)
	if err != nil {
		return nil, err
	}
	result := &ProcessFileAsyncResult{Response: *resp}
	if len(respBody) > 0 {
		switch {
		case resp.StatusCode == http.StatusAccepted:
			var pr ProcessingPendingResponse
			if err := json.Unmarshal(respBody, &pr); err != nil {
				return nil, fmt.Errorf("parsing async response: %w", err)
			}
			result.Pending = &pr
		case resp.StatusCode >= 400:
			var er ErrorResponse
			if err := json.Unmarshal(respBody, &er); err == nil {
				result.Error = &er
			}
		}
	}
	return result, nil
}

// ProcessFileFetch submits a URL for asynchronous processing.
func (c *Client) ProcessFileFetch(ctx context.Context, contentType string, body io.Reader) (*ProcessFileFetchResult, error) {
	resp, respBody, err := c.do(ctx, http.MethodPost, "/files/fetch", contentType, body)
	if err != nil {
		return nil, err
	}
	result := &ProcessFileFetchResult{Response: *resp}
	if len(respBody) > 0 {
		switch {
		case resp.StatusCode == http.StatusAccepted:
			var pr ProcessingPendingResponse
			if err := json.Unmarshal(respBody, &pr); err != nil {
				return nil, fmt.Errorf("parsing fetch response: %w", err)
			}
			result.Pending = &pr
		case resp.StatusCode >= 400:
			var er ErrorResponse
			if err := json.Unmarshal(respBody, &er); err == nil {
				result.Error = &er
			}
		}
	}
	return result, nil
}

// RetrieveFile retrieves a previously processed file result.
func (c *Client) RetrieveFile(ctx context.Context, id string) (*RetrieveFileResult, error) {
	resp, body, err := c.do(ctx, http.MethodGet, "/files/"+id, "", nil)
	if err != nil {
		return nil, err
	}
	result := &RetrieveFileResult{Response: *resp}
	if resp.StatusCode == http.StatusOK && len(body) > 0 {
		var pr ProcessingResponse
		if err := json.Unmarshal(body, &pr); err != nil {
			return nil, fmt.Errorf("parsing retrieve file response: %w", err)
		}
		result.Result = &pr
	}
	return result, nil
}

// DeleteFile hard-deletes a previously processed file result.
func (c *Client) DeleteFile(ctx context.Context, id string) (*DeleteFileResult, error) {
	resp, _, err := c.do(ctx, http.MethodDelete, "/files/"+id, "", nil)
	if err != nil {
		return nil, err
	}
	return &DeleteFileResult{Response: *resp}, nil
}

// DeleteFileTrace deletes the processing trace for a previously processed file.
func (c *Client) DeleteFileTrace(ctx context.Context, id string) (*DeleteFileTraceResult, error) {
	resp, _, err := c.do(ctx, http.MethodDelete, "/files/"+id+"/trace", "", nil)
	if err != nil {
		return nil, err
	}
	return &DeleteFileTraceResult{Response: *resp}, nil
}

// RetrieveTrace retrieves the processing trace for a previously processed file.
func (c *Client) RetrieveTrace(ctx context.Context, id string) (*RetrieveTraceResult, error) {
	resp, body, err := c.do(ctx, http.MethodGet, "/files/"+id+"/trace", "", nil)
	if err != nil {
		return nil, err
	}
	result := &RetrieveTraceResult{Response: *resp}
	if len(body) > 0 {
		switch {
		case resp.StatusCode == http.StatusOK:
			var tr TraceResponse
			if err := json.Unmarshal(body, &tr); err != nil {
				return nil, fmt.Errorf("parsing retrieve trace response: %w", err)
			}
			result.Trace = &tr
		case resp.StatusCode >= 400:
			var er ErrorResponse
			if err := json.Unmarshal(body, &er); err == nil {
				result.Error = &er
			}
		}
	}
	return result, nil
}

// CreateToken creates a temporary authentication token.
func (c *Client) CreateToken(ctx context.Context, contentType string, body io.Reader) (*CreateTokenResult, error) {
	resp, respBody, err := c.do(ctx, http.MethodPost, "/auth/tokens", contentType, body)
	if err != nil {
		return nil, err
	}
	result := &CreateTokenResult{Response: *resp}
	if resp.StatusCode == http.StatusCreated && len(respBody) > 0 {
		var token AuthToken
		if err := json.Unmarshal(respBody, &token); err != nil {
			return nil, fmt.Errorf("parsing create token response: %w", err)
		}
		result.Token = &token
	}
	return result, nil
}

// RetrieveToken retrieves an existing authentication token.
func (c *Client) RetrieveToken(ctx context.Context, id string) (*RetrieveTokenResult, error) {
	resp, body, err := c.do(ctx, http.MethodGet, "/auth/tokens/"+id, "", nil)
	if err != nil {
		return nil, err
	}
	result := &RetrieveTokenResult{Response: *resp}
	if resp.StatusCode == http.StatusOK && len(body) > 0 {
		var token AuthToken
		if err := json.Unmarshal(body, &token); err != nil {
			return nil, fmt.Errorf("parsing retrieve token response: %w", err)
		}
		result.Token = &token
	}
	return result, nil
}

// DeleteToken deletes an authentication token.
func (c *Client) DeleteToken(ctx context.Context, id string) (*Response, error) {
	resp, _, err := c.do(ctx, http.MethodDelete, "/auth/tokens/"+id, "", nil)
	if err != nil {
		return nil, err
	}
	return resp, nil
}
