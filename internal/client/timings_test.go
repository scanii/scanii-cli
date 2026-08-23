package client

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestElapsedIgnoresAnUnstartedPhase(t *testing.T) {
	if got := elapsed(time.Time{}); got != 0 {
		t.Fatalf("expected 0 for a phase that never started, got %s", got)
	}
	// a phase that did start is measured however briefly it ran, so the clock's
	// resolution is kept out of it by starting one a second ago
	if got := elapsed(time.Now().Add(-time.Second)); got < time.Second {
		t.Fatalf("expected at least a second, got %s", got)
	}
}

func TestDoTimesTheExchange(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set(RequestIDHeader, "req_abc123")
		_, _ = w.Write([]byte(`{"message":"pong"}`))
	}))
	defer srv.Close()

	c, err := New(srv.URL)
	if err != nil {
		t.Fatalf("failed to create client: %s", err)
	}

	result, err := c.Ping(t.Context())
	if err != nil {
		t.Fatalf("ping failed: %s", err)
	}

	// the phases of a loopback exchange can each finish inside the clock's
	// resolution, so the flag is what says it was timed, not the durations
	if !result.Timings.Complete {
		t.Fatal("expected the exchange to be reported as complete")
	}
	if result.Timings.Connect <= 0 {
		t.Fatalf("expected a positive connect, got %s", result.Timings.Connect)
	}
	// httptest serves on a loopback address, so there is no name to resolve
	if result.Timings.DNS != 0 {
		t.Fatalf("expected no dns phase for an address literal, got %s", result.Timings.DNS)
	}
	if result.Timings.Reused {
		t.Fatal("expected the first request to open its own connection")
	}
	if got := result.RequestID(); got != "req_abc123" {
		t.Fatalf("expected req_abc123, got %q", got)
	}
}

func TestDoReportsAPooledConnection(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"message":"pong"}`))
	}))
	defer srv.Close()

	c, err := New(srv.URL)
	if err != nil {
		t.Fatalf("failed to create client: %s", err)
	}

	if _, err := c.Ping(t.Context()); err != nil {
		t.Fatalf("first ping failed: %s", err)
	}
	result, err := c.Ping(t.Context())
	if err != nil {
		t.Fatalf("second ping failed: %s", err)
	}

	if !result.Timings.Reused {
		t.Fatal("expected the second request to ride on the pooled connection")
	}
	if result.Timings.Connect != 0 {
		t.Fatalf("expected no connect phase on a pooled connection, got %s", result.Timings.Connect)
	}
}

func TestRequestIDIsEmptyWhenTheAPISendsNone(t *testing.T) {
	r := Response{Header: http.Header{}}
	if got := r.RequestID(); got != "" {
		t.Fatalf("expected an empty request id, got %q", got)
	}
}

func TestTimingsAreIncompleteUntilTheResponseIsRead(t *testing.T) {
	if (Timings{}).Complete {
		t.Fatal("expected the zero value to report as incomplete")
	}

	// every duration can legitimately be zero on a clock too coarse to see them
	var tr tracer
	tr.bodyRead()
	if !tr.result().Complete {
		t.Fatal("expected a read response to report as complete")
	}
}
