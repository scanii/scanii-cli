package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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

// Response is the base response containing HTTP metadata.
type Response struct {
	StatusCode int
	Header     http.Header
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

// do executes an HTTP request and returns the status code, headers, and body bytes.
func (c *Client) do(ctx context.Context, method, path, contentType string, body io.Reader) (int, http.Header, []byte, error) { //nolint:gocritic
	url := c.baseURL + path
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return 0, nil, nil, fmt.Errorf("creating request: %w", err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	for _, editor := range c.editors {
		if err := editor(ctx, req); err != nil {
			return 0, nil, nil, fmt.Errorf("applying request editor: %w", err)
		}
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, nil, nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, resp.Header, nil, fmt.Errorf("reading response body: %w", err)
	}
	return resp.StatusCode, resp.Header, data, nil
}

// Ping validates API credentials.
func (c *Client) Ping(ctx context.Context) (*PingResult, error) {
	status, header, body, err := c.do(ctx, http.MethodGet, "/ping", "", nil)
	if err != nil {
		return nil, err
	}
	result := &PingResult{Response: Response{StatusCode: status, Header: header}}
	if status == http.StatusOK && len(body) > 0 {
		_ = json.Unmarshal(body, result)
	}
	return result, nil
}

// Account retrieves account information.
func (c *Client) Account(ctx context.Context) (*AccountResult, error) {
	status, header, body, err := c.do(ctx, http.MethodGet, "/account.json", "", nil)
	if err != nil {
		return nil, err
	}
	result := &AccountResult{Response: Response{StatusCode: status, Header: header}}
	if status == http.StatusOK && len(body) > 0 {
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
	status, header, respBody, err := c.do(ctx, http.MethodPost, "/files", contentType, body)
	if err != nil {
		return nil, err
	}
	result := &ProcessFileResult{Response: Response{StatusCode: status, Header: header}}
	if len(respBody) > 0 {
		switch {
		case status == http.StatusCreated:
			var pr ProcessingResponse
			if err := json.Unmarshal(respBody, &pr); err != nil {
				return nil, fmt.Errorf("parsing process file response: %w", err)
			}
			result.Result = &pr
		case status >= 400:
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
	status, header, respBody, err := c.do(ctx, http.MethodPost, "/files/async", contentType, body)
	if err != nil {
		return nil, err
	}
	result := &ProcessFileAsyncResult{Response: Response{StatusCode: status, Header: header}}
	if len(respBody) > 0 {
		switch {
		case status == http.StatusAccepted:
			var pr ProcessingPendingResponse
			if err := json.Unmarshal(respBody, &pr); err != nil {
				return nil, fmt.Errorf("parsing async response: %w", err)
			}
			result.Pending = &pr
		case status >= 400:
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
	status, header, respBody, err := c.do(ctx, http.MethodPost, "/files/fetch", contentType, body)
	if err != nil {
		return nil, err
	}
	result := &ProcessFileFetchResult{Response: Response{StatusCode: status, Header: header}}
	if len(respBody) > 0 {
		switch {
		case status == http.StatusAccepted:
			var pr ProcessingPendingResponse
			if err := json.Unmarshal(respBody, &pr); err != nil {
				return nil, fmt.Errorf("parsing fetch response: %w", err)
			}
			result.Pending = &pr
		case status >= 400:
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
	status, header, body, err := c.do(ctx, http.MethodGet, "/files/"+id, "", nil)
	if err != nil {
		return nil, err
	}
	result := &RetrieveFileResult{Response: Response{StatusCode: status, Header: header}}
	if status == http.StatusOK && len(body) > 0 {
		var pr ProcessingResponse
		if err := json.Unmarshal(body, &pr); err != nil {
			return nil, fmt.Errorf("parsing retrieve file response: %w", err)
		}
		result.Result = &pr
	}
	return result, nil
}

// RetrieveTrace retrieves the processing trace for a previously processed file.
func (c *Client) RetrieveTrace(ctx context.Context, id string) (*RetrieveTraceResult, error) {
	status, header, body, err := c.do(ctx, http.MethodGet, "/files/"+id+"/trace", "", nil)
	if err != nil {
		return nil, err
	}
	result := &RetrieveTraceResult{Response: Response{StatusCode: status, Header: header}}
	if len(body) > 0 {
		switch {
		case status == http.StatusOK:
			var tr TraceResponse
			if err := json.Unmarshal(body, &tr); err != nil {
				return nil, fmt.Errorf("parsing retrieve trace response: %w", err)
			}
			result.Trace = &tr
		case status >= 400:
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
	status, header, respBody, err := c.do(ctx, http.MethodPost, "/auth/tokens", contentType, body)
	if err != nil {
		return nil, err
	}
	result := &CreateTokenResult{Response: Response{StatusCode: status, Header: header}}
	if status == http.StatusCreated && len(respBody) > 0 {
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
	status, header, body, err := c.do(ctx, http.MethodGet, "/auth/tokens/"+id, "", nil)
	if err != nil {
		return nil, err
	}
	result := &RetrieveTokenResult{Response: Response{StatusCode: status, Header: header}}
	if status == http.StatusOK && len(body) > 0 {
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
	status, header, _, err := c.do(ctx, http.MethodDelete, "/auth/tokens/"+id, "", nil)
	if err != nil {
		return nil, err
	}
	return &Response{StatusCode: status, Header: header}, nil
}
