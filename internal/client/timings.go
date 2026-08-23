package client

import (
	"crypto/tls"
	"net/http/httptrace"
	"sync"
	"time"
)

// Timings is the wall-clock breakdown of a single HTTP exchange, captured with
// net/http/httptrace. The phases run in the order they are declared and, pool
// wait aside, add up to Total.
//
// A phase that did not happen is reported as zero: a pooled connection resolves
// no name, opens no socket and shakes no hands, and a plaintext endpoint never
// reaches the TLS phase. So is one that finished inside the clock's resolution,
// which the two cases cannot be told apart by — a distinction that does not
// matter for the latencies this is meant to expose.
type Timings struct {
	// DNS is the time spent resolving the host name.
	DNS time.Duration

	// Connect is the time spent opening the TCP connection.
	Connect time.Duration

	// TLS is the time spent on the TLS handshake.
	TLS time.Duration

	// RequestTransfer is the time from having a usable connection to the last
	// byte of the request being written. For a file upload that is the upload
	// itself — near enough, anyway: the write finishes at the socket rather than
	// at the far end, so up to a send buffer's worth of it is still in flight.
	RequestTransfer time.Duration

	// ServerProcessing is the time between the last request byte going out and
	// the first response byte coming back. That covers the API's own work — for
	// a file scan, the scan — but also the round trip carrying the question
	// there and the answer back, and nothing on this side can see where the one
	// ends and the other begins. Expect it to read higher than whatever the
	// server reports for the same request, by about one round trip.
	//
	// It is zero when the API answered before the request finished going out, as
	// it does when it rejects an oversized upload part way through.
	ServerProcessing time.Duration

	// ResponseTransfer is the time from the first byte of the response to the
	// last byte of its body.
	ResponseTransfer time.Duration

	// Total is the wall clock for the whole exchange, from the request being
	// handed to the transport to the response body being fully read.
	Total time.Duration

	// Reused reports whether the exchange rode on a pooled connection, which is
	// why the phases that establish one can legitimately read as zero.
	Reused bool

	// Complete reports whether the exchange reached the API and its response was
	// read. It is a flag rather than something inferred from the durations,
	// because every one of them can legitimately be zero: a phase that finishes
	// inside the clock's resolution measures as no time at all, and Windows
	// resolves time in milliseconds.
	Complete bool
}

// tracer collects the Timings of one request. Its hooks run on the transport's
// goroutines while the request is in flight, so the fields are guarded.
type tracer struct {
	mu           sync.Mutex
	start        time.Time
	dnsStart     time.Time
	connectStart time.Time
	tlsStart     time.Time
	gotConn      time.Time
	wroteRequest time.Time
	firstByte    time.Time
	timings      Timings
}

// begin starts the clock for Total. It is called immediately before the request
// is handed to the transport.
func (t *tracer) begin() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.start = time.Now()
}

// trace returns the hooks to attach to a request's context.
func (t *tracer) trace() *httptrace.ClientTrace {
	return &httptrace.ClientTrace{
		DNSStart: func(httptrace.DNSStartInfo) {
			t.mu.Lock()
			defer t.mu.Unlock()
			t.dnsStart = time.Now()
		},
		DNSDone: func(httptrace.DNSDoneInfo) {
			t.mu.Lock()
			defer t.mu.Unlock()
			t.timings.DNS = elapsed(t.dnsStart)
		},
		ConnectStart: func(_, _ string) {
			t.mu.Lock()
			defer t.mu.Unlock()
			t.connectStart = time.Now()
		},
		ConnectDone: func(_, _ string, _ error) {
			t.mu.Lock()
			defer t.mu.Unlock()
			t.timings.Connect = elapsed(t.connectStart)
		},
		TLSHandshakeStart: func() {
			t.mu.Lock()
			defer t.mu.Unlock()
			t.tlsStart = time.Now()
		},
		TLSHandshakeDone: func(_ tls.ConnectionState, _ error) {
			t.mu.Lock()
			defer t.mu.Unlock()
			t.timings.TLS = elapsed(t.tlsStart)
		},
		GotConn: func(info httptrace.GotConnInfo) {
			t.mu.Lock()
			defer t.mu.Unlock()
			t.gotConn = time.Now()
			t.timings.Reused = info.Reused
		},
		WroteRequest: func(httptrace.WroteRequestInfo) {
			t.mu.Lock()
			defer t.mu.Unlock()
			t.wroteRequest = time.Now()
			t.timings.RequestTransfer = elapsed(t.gotConn)
		},
		GotFirstResponseByte: func() {
			t.mu.Lock()
			defer t.mu.Unlock()
			t.firstByte = time.Now()
			t.timings.ServerProcessing = elapsed(t.wroteRequest)
		},
	}
}

// bodyRead closes out the response transfer and the total. It is called once the
// response body has been read to completion.
func (t *tracer) bodyRead() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.timings.ResponseTransfer = elapsed(t.firstByte)
	t.timings.Total = elapsed(t.start)
	t.timings.Complete = true
}

// result reports the timings collected so far.
func (t *tracer) result() Timings {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.timings
}

// elapsed reports the time since start, or zero if the phase never started —
// time.Since on a zero time measures from the year 1, which would be reported
// as a two-thousand-year DNS lookup.
func elapsed(start time.Time) time.Duration {
	if start.IsZero() {
		return 0
	}
	return time.Since(start)
}
