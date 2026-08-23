package file

import (
	"fmt"
	"sync"
	"time"

	"github.com/uvasoftware/scanii-cli/internal/client"
	"github.com/uvasoftware/scanii-cli/internal/terminal"
)

// perfLabelWidth is wide enough for the longest phase label below.
const perfLabelWidth = 20

// perfReport accumulates the timing breakdown of the requests a run made, so
// that --perf can report where the wall clock actually went. A directory run
// makes one request per file, so the report averages: the interesting question
// over a thousand files is what a typical one cost, not what they cost summed.
//
// Results arrive from one goroutine per in-flight file, hence the lock.
type perfReport struct {
	mu        sync.Mutex
	requests  int
	reused    int
	requestID string
	total     client.Timings
}

// add records one request. A request that never reached the API carries nothing
// to report and is left out rather than averaged in as zeroes.
//
// A nil report drops what it is given, so that the commands which take one can
// be called without one.
func (p *perfReport) add(timings client.Timings, requestID string) {
	if p == nil || !timings.Complete {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	p.requests++
	p.requestID = requestID
	if timings.Reused {
		p.reused++
	}
	p.total.DNS += timings.DNS
	p.total.Connect += timings.Connect
	p.total.TLS += timings.TLS
	p.total.RequestTransfer += timings.RequestTransfer
	p.total.ServerProcessing += timings.ServerProcessing
	p.total.ResponseTransfer += timings.ResponseTransfer
	p.total.Total += timings.Total
}

// addResponse records the request that produced resp.
func (p *perfReport) addResponse(resp *client.Response) {
	p.add(resp.Timings, resp.RequestID())
}

// mean reports the average exchange, and how many exchanges it averages over.
func (p *perfReport) mean() (avg client.Timings, requests int) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.requests == 0 {
		return client.Timings{}, 0
	}

	n := time.Duration(p.requests)
	return client.Timings{
		DNS:              p.total.DNS / n,
		Connect:          p.total.Connect / n,
		TLS:              p.total.TLS / n,
		RequestTransfer:  p.total.RequestTransfer / n,
		ServerProcessing: p.total.ServerProcessing / n,
		ResponseTransfer: p.total.ResponseTransfer / n,
		Total:            p.total.Total / n,
	}, p.requests
}

// print writes the performance summary. elapsed is the wall clock of the whole
// run, which is only comparable to a single request — a directory run overlaps
// its requests, so its elapsed time is not their sum.
func (p *perfReport) print(elapsed time.Duration) {
	mean, requests := p.mean()

	p.mu.Lock()
	requestID, reused := p.requestID, p.reused
	p.mu.Unlock()

	if requests == 0 {
		terminal.Section("Performance")
		terminal.Warn("no request reached the API, there is nothing to report")
		return
	}

	if requests == 1 {
		terminal.Section("Performance")
		if requestID != "" {
			terminal.KeyValueW("request id:", requestID, perfLabelWidth)
		}
	} else {
		terminal.Section(fmt.Sprintf("Performance (mean of %s requests)", terminal.FormatNumber(int64(requests))))
	}

	terminal.KeyValueW("dns:", perfDuration(mean.DNS), perfLabelWidth)
	terminal.KeyValueW("tcp connect:", perfDuration(mean.Connect), perfLabelWidth)
	terminal.KeyValueW("tls handshake:", perfDuration(mean.TLS), perfLabelWidth)
	terminal.KeyValueW("request transfer:", perfDuration(mean.RequestTransfer), perfLabelWidth)
	terminal.KeyValueW("server processing:", perfDuration(mean.ServerProcessing), perfLabelWidth)
	terminal.KeyValueW("response transfer:", perfDuration(mean.ResponseTransfer), perfLabelWidth)
	terminal.KeyValueW("total:", perfDuration(mean.Total), perfLabelWidth)

	// the phases that establish a connection read as n/a on a pooled one, which
	// is worth explaining rather than leaving to look like a measurement failure
	if requests == 1 {
		// whatever the run spent outside the exchange — opening and hashing the
		// file, assembling the multipart body, printing the result — is time the
		// API never sees, and is the first thing to look at when the CLI feels
		// slower than the server says it was
		if overhead := elapsed - mean.Total; overhead > 0 {
			terminal.KeyValueW("client overhead:", perfDuration(overhead), perfLabelWidth)
		}
		if reused == 1 {
			terminal.KeyValueW("connection:", "reused", perfLabelWidth)
		} else {
			terminal.KeyValueW("connection:", "new", perfLabelWidth)
		}
		return
	}
	terminal.KeyValueW("connections:", fmt.Sprintf("%s of %s reused",
		terminal.FormatNumber(int64(reused)), terminal.FormatNumber(int64(requests))), perfLabelWidth)
}

// perfDuration reports a phase that did not happen as n/a rather than as "0 s".
// A pooled connection resolves no name, and reading that as an infinitely fast
// lookup is worse than reading nothing at all.
func perfDuration(d time.Duration) string {
	if d == 0 {
		return "n/a"
	}
	return terminal.FormatDuration(d)
}
