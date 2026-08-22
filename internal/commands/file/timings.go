package file

import (
	"context"
	"crypto/tls"
	"log/slog"
	"net/http/httptrace"
	"strings"
	"sync"
	"time"
)

// requestTimings records where a single request spent its wall clock.
//
// It exists for the case where the server reports a fast request and the client
// disagrees — the load balancer and the application both reporting half a second
// while the CLI's own timer reports twenty. The gap is always in one of the
// phases below, and without them there is nothing to look at.
//
// Every milestone is an offset from the moment the request started, so the line
// reads as a timeline: the phase that jumps is the one that cost the time. A
// milestone left at zero is one that never happened, which is itself the answer
// when a request hangs partway.
type requestTimings struct {
	start time.Time

	// The hooks fire on whichever goroutine drives the connection, so the fields
	// they write are guarded.
	mu            sync.Mutex
	dnsDone       time.Duration
	connected     time.Duration
	tlsDone       time.Duration
	gotConn       time.Duration
	wroteRequest  time.Duration
	firstByte     time.Duration
	reusedConn    bool
	connectTries  int
	connectErrors []string
}

// newRequestTimings returns a context that records the connection lifecycle of the
// request it is used for. Call log when the request returns.
//
// Returns a nil *requestTimings and ctx unchanged unless debug logging is on, so
// tracing costs nothing in a normal run. log is safe to call on the nil value.
func newRequestTimings(ctx context.Context) (context.Context, *requestTimings) {
	if !slog.Default().Enabled(ctx, slog.LevelDebug) {
		return ctx, nil
	}

	t := &requestTimings{start: time.Now()}

	return httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{
		DNSDone: func(info httptrace.DNSDoneInfo) {
			t.mark(&t.dnsDone)
			if info.Err != nil {
				slog.Debug("http: dns failed", "at", t.since(), "error", info.Err)
			}
		},
		ConnectStart: func(_, _ string) {
			t.mu.Lock()
			defer t.mu.Unlock()
			t.connectTries++
		},
		ConnectDone: func(network, addr string, err error) {
			t.mark(&t.connected)
			if err == nil {
				return
			}
			// A failed attempt here is usually the IPv6 half of happy eyeballs; it
			// is only interesting when it is slow rather than refused outright.
			t.mu.Lock()
			defer t.mu.Unlock()
			t.connectErrors = append(t.connectErrors, network+" "+addr+": "+err.Error())
		},
		TLSHandshakeDone: func(_ tls.ConnectionState, err error) {
			t.mark(&t.tlsDone)
			if err != nil {
				slog.Debug("http: tls failed", "at", t.since(), "error", err)
			}
		},
		GotConn: func(info httptrace.GotConnInfo) {
			t.mu.Lock()
			defer t.mu.Unlock()
			t.gotConn = time.Since(t.start)
			t.reusedConn = info.Reused
		},
		WroteRequest:         func(httptrace.WroteRequestInfo) { t.mark(&t.wroteRequest) },
		GotFirstResponseByte: func() { t.mark(&t.firstByte) },
	}), t
}

func (t *requestTimings) since() time.Duration { return time.Since(t.start) }

func (t *requestTimings) mark(field *time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()
	*field = time.Since(t.start)
}

// log emits one line describing where the request's time went. The milestones are
// offsets from the start of the request, not durations of each phase, so a gap
// between two adjacent numbers is the cost of whatever sits between them.
func (t *requestTimings) log(path string, err error) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	slog.Debug("http: time to response",
		"path", path,
		"total", time.Since(t.start),
		"dns_done", t.dnsDone,
		"connected", t.connected,
		"tls_done", t.tlsDone,
		"conn_ready", t.gotConn,
		"request_written", t.wroteRequest,
		"first_byte", t.firstByte,
		"conn_reused", t.reusedConn,
		"connect_attempts", t.connectTries,
		"connect_errors", strings.Join(t.connectErrors, "; "),
		"error", err,
	)
}
