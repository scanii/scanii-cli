package file

import (
	"sync"
	"testing"
	"time"

	"github.com/uvasoftware/scanii-cli/internal/client"
)

func TestPerfReportSkipsUnmeasuredRequests(t *testing.T) {
	report := &perfReport{}
	// a request that never reached the API carries nothing to average
	report.add(&resultRecord{path: "unreachable.txt"})

	if _, requests := report.mean(); requests != 0 {
		t.Fatalf("expected 0 requests, got %d", requests)
	}
}

func TestPerfReportMean(t *testing.T) {
	report := &perfReport{}
	report.add(&resultRecord{
		requestID: "req_one",
		timings: client.Timings{
			Complete:         true,
			DNS:              10 * time.Millisecond,
			Connect:          2 * time.Millisecond,
			TLS:              20 * time.Millisecond,
			RequestTransfer:  100 * time.Millisecond,
			ServerProcessing: 40 * time.Millisecond,
			ResponseTransfer: 8 * time.Millisecond,
			Total:            180 * time.Millisecond,
			Reused:           true,
		},
	})
	report.add(&resultRecord{
		requestID: "req_two",
		timings: client.Timings{
			Complete:         true,
			DNS:              20 * time.Millisecond,
			Connect:          4 * time.Millisecond,
			TLS:              40 * time.Millisecond,
			RequestTransfer:  200 * time.Millisecond,
			ServerProcessing: 60 * time.Millisecond,
			ResponseTransfer: 12 * time.Millisecond,
			Total:            340 * time.Millisecond,
		},
	})

	mean, requests := report.mean()
	if requests != 2 {
		t.Fatalf("expected 2 requests, got %d", requests)
	}

	for _, tc := range []struct {
		phase string
		got   time.Duration
		want  time.Duration
	}{
		{"dns", mean.DNS, 15 * time.Millisecond},
		{"connect", mean.Connect, 3 * time.Millisecond},
		{"tls", mean.TLS, 30 * time.Millisecond},
		{"request transfer", mean.RequestTransfer, 150 * time.Millisecond},
		{"server processing", mean.ServerProcessing, 50 * time.Millisecond},
		{"response transfer", mean.ResponseTransfer, 10 * time.Millisecond},
		{"total", mean.Total, 260 * time.Millisecond},
	} {
		if tc.got != tc.want {
			t.Errorf("expected a %s mean of %s, got %s", tc.phase, tc.want, tc.got)
		}
	}

	if report.reused != 1 {
		t.Fatalf("expected 1 reused connection, got %d", report.reused)
	}
}

func TestPerfReportEmpty(t *testing.T) {
	report := &perfReport{}

	mean, requests := report.mean()
	if requests != 0 {
		t.Fatalf("expected 0 requests, got %d", requests)
	}
	if mean.Total != 0 {
		t.Fatalf("expected a zero mean, got %+v", mean)
	}
}

func TestPerfReportConcurrentAdd(t *testing.T) {
	// results arrive from one goroutine per in-flight file
	report := &perfReport{}

	var wg sync.WaitGroup
	for range 50 {
		wg.Go(func() {
			report.add(&resultRecord{timings: client.Timings{Complete: true, Total: 10 * time.Millisecond}})
		})
	}
	wg.Wait()

	mean, requests := report.mean()
	if requests != 50 {
		t.Fatalf("expected 50 requests, got %d", requests)
	}
	if mean.Total != 10*time.Millisecond {
		t.Fatalf("expected a 10ms mean total, got %s", mean.Total)
	}
}

func TestPerfDurationReportsAMissingPhaseAsNotApplicable(t *testing.T) {
	if got := perfDuration(0); got != "n/a" {
		t.Fatalf("expected n/a, got %q", got)
	}
	if got := perfDuration(250 * time.Millisecond); got != "250 ms" {
		t.Fatalf("expected 250 ms, got %q", got)
	}
}
