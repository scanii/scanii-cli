package file

import (
	"context"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/uvasoftware/scanii-cli/internal/commands/profile"
)

func newTestService(t *testing.T) *service {
	t.Helper()
	svc, err := newService(ts.Profile)
	if err != nil {
		t.Fatalf("failed to create service: %s", err)
	}
	return svc
}

func TestServiceProcessSyncSingleFile(t *testing.T) {
	svc := newTestService(t)

	stream := make(chan string, 1)
	stream <- fakeMalwareSample
	close(stream)

	var results []resultRecord
	var mu sync.Mutex

	err := svc.process(context.Background(), stream, processOptions{maxConcurrency: 1, metadata: map[string]string{"m1": "v1"}}, func(r resultRecord) {
		mu.Lock()
		results = append(results, r)
		mu.Unlock()
	})
	if err != nil {
		t.Fatalf("process failed: %s", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	r := &results[0]
	if r.err != nil {
		t.Fatalf("expected no error, got %s", r.err)
	}
	checkResponseContent(t, r)
	if r.metadata["m1"] != "v1" {
		t.Fatalf("expected metadata m1=v1, got %s", r.metadata["m1"])
	}
}

func TestServiceProcessAsyncSingleFile(t *testing.T) {
	svc := newTestService(t)

	stream := make(chan string, 1)
	stream <- fakeMalwareSample
	close(stream)

	var results []resultRecord
	var mu sync.Mutex

	err := svc.process(context.Background(), stream, processOptions{maxConcurrency: 1, async: true, metadata: map[string]string{"m1": "v1"}}, func(r resultRecord) {
		mu.Lock()
		results = append(results, r)
		mu.Unlock()
	})
	if err != nil {
		t.Fatalf("process failed: %s", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	r := results[0]
	if r.err != nil {
		t.Fatalf("expected no error, got %s", r.err)
	}
	if r.id == "" {
		t.Fatalf("expected result to have an id")
	}
	if r.location == "" {
		t.Fatalf("expected result to have a location")
	}
}

func TestServiceProcessMultipleFiles(t *testing.T) {
	svc := newTestService(t)

	stream := make(chan string, 3)
	stream <- fakeMalwareSample
	stream <- fakeMalwareSample
	stream <- fakeMalwareSample
	close(stream)

	var results []resultRecord
	var mu sync.Mutex

	err := svc.process(context.Background(), stream, processOptions{maxConcurrency: 2}, func(r resultRecord) {
		mu.Lock()
		results = append(results, r)
		mu.Unlock()
	})
	if err != nil {
		t.Fatalf("process failed: %s", err)
	}

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	for i, r := range results {
		if r.err != nil {
			t.Fatalf("result %d: expected no error, got %s", i, r.err)
		}
		checkResponseContent(t, &r)
	}
}

func TestServiceProcessNonExistentFile(t *testing.T) {
	svc := newTestService(t)

	stream := make(chan string, 1)
	stream <- "testdata/does_not_exist.txt"
	close(stream)

	var results []resultRecord
	var mu sync.Mutex

	err := svc.process(context.Background(), stream, processOptions{maxConcurrency: 1}, func(r resultRecord) {
		mu.Lock()
		results = append(results, r)
		mu.Unlock()
	})
	if err != nil {
		t.Fatalf("process failed: %s", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	if results[0].err == nil {
		t.Fatalf("expected an error for non-existent file")
	}
}

// newTestServiceWithBadCredentials returns a service pointed at the mock server
// with credentials it will reject, which is the cheapest way to drive a real API
// error response — status, body and request id included.
func newTestServiceWithBadCredentials(t *testing.T) *service {
	t.Helper()
	svc, err := newService(&profile.Profile{
		CreatedAt:   time.Now(),
		Credentials: "bad:credentials",
		Endpoint:    ts.Endpoint,
	})
	if err != nil {
		t.Fatalf("failed to create service: %s", err)
	}
	return svc
}

func TestServiceProcessAPIError(t *testing.T) {
	run := func(t *testing.T, async bool) resultRecord {
		t.Helper()
		svc := newTestServiceWithBadCredentials(t)

		stream := make(chan string, 1)
		stream <- fakeMalwareSample
		close(stream)

		var results []resultRecord
		var mu sync.Mutex

		err := svc.process(context.Background(), stream, processOptions{maxConcurrency: 1, async: async}, func(r resultRecord) {
			mu.Lock()
			results = append(results, r)
			mu.Unlock()
		})
		if err != nil {
			t.Fatalf("process failed: %s", err)
		}
		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}
		return results[0]
	}

	t.Run("sync", func(t *testing.T) {
		r := run(t, false)

		if r.err == nil {
			t.Fatal("expected an error for a rejected request")
		}
		// the API's own message, not just the status code
		if !strings.Contains(r.err.Error(), "401") || !strings.Contains(r.err.Error(), "could not authenticate") {
			t.Fatalf("expected the api error message, got %q", r.err)
		}
		if !strings.Contains(r.err.Error(), "request id ") {
			t.Fatalf("expected the request id in the error, got %q", r.err)
		}
		// a rejected file has no checksum to verify against
		if strings.Contains(r.err.Error(), "checksum") {
			t.Fatalf("expected no checksum verification on an api error, got %q", r.err)
		}
		if r.checksum != "" || r.id != "" {
			t.Fatalf("expected an empty result, got checksum %q and id %q", r.checksum, r.id)
		}
	})

	t.Run("async", func(t *testing.T) {
		r := run(t, true)

		if r.err == nil {
			t.Fatal("expected an error for a rejected request")
		}
		if !strings.Contains(r.err.Error(), "401") || !strings.Contains(r.err.Error(), "could not authenticate") {
			t.Fatalf("expected the api error message, got %q", r.err)
		}
		if r.id != "" || r.location != "" {
			t.Fatalf("expected an empty result, got id %q and location %q", r.id, r.location)
		}
	})
}

func TestServiceRetrieve(t *testing.T) {
	svc := newTestService(t)

	// first process a file to get an id
	stream := make(chan string, 1)
	stream <- fakeMalwareSample
	close(stream)

	var results []resultRecord
	var mu sync.Mutex

	err := svc.process(context.Background(), stream, processOptions{maxConcurrency: 1, metadata: map[string]string{"m1": "v1"}}, func(r resultRecord) {
		mu.Lock()
		results = append(results, r)
		mu.Unlock()
	})
	if err != nil {
		t.Fatalf("process failed: %s", err)
	}

	if len(results) != 1 || results[0].id == "" {
		t.Fatalf("expected a result with an id")
	}

	// now retrieve it
	retrieved, err := svc.retrieve(context.Background(), results[0].id)
	if err != nil {
		t.Fatalf("retrieve failed: %s", err)
	}

	checkResponseContent(t, retrieved)
	if retrieved.metadata["m1"] != "v1" {
		t.Fatalf("expected metadata m1=v1, got %s", retrieved.metadata["m1"])
	}
}

func TestServiceRetrieveAsync(t *testing.T) {
	svc := newTestService(t)

	// process a file async
	stream := make(chan string, 1)
	stream <- fakeMalwareSample
	close(stream)

	var results []resultRecord
	var mu sync.Mutex

	err := svc.process(context.Background(), stream, processOptions{maxConcurrency: 1, async: true, metadata: map[string]string{"m1": "v1"}}, func(r resultRecord) {
		mu.Lock()
		results = append(results, r)
		mu.Unlock()
	})
	if err != nil {
		t.Fatalf("process failed: %s", err)
	}

	if len(results) != 1 || results[0].id == "" {
		t.Fatalf("expected a result with an id")
	}

	// retrieve the async result
	retrieved, err := svc.retrieve(context.Background(), results[0].id)
	if err != nil {
		t.Fatalf("retrieve failed: %s", err)
	}

	checkResponseContent(t, retrieved)
}

func TestServiceProcessReportsUploadedBytes(t *testing.T) {
	svc := newTestService(t)

	stream := make(chan string, 1)
	stream <- fakeMalwareSample
	close(stream)

	var reported atomic.Uint64
	var results []resultRecord
	var mu sync.Mutex

	err := svc.process(context.Background(), stream, processOptions{
		maxConcurrency: 1,
		onBytes:        func(n uint64) { reported.Add(n) },
	}, func(r resultRecord) {
		mu.Lock()
		results = append(results, r)
		mu.Unlock()
	})
	if err != nil {
		t.Fatalf("process failed: %s", err)
	}

	info, err := os.Stat(fakeMalwareSample)
	if err != nil {
		t.Fatalf("failed to stat sample: %s", err)
	}

	// every byte of the file is reported exactly once, which is what makes the
	// progress bar track the upload rather than guessing at it
	if got, want := reported.Load(), uint64(info.Size()); got != want { //nolint:gosec
		t.Fatalf("expected %d bytes reported, got %d", want, got)
	}
	if len(results) != 1 || results[0].err != nil {
		t.Fatalf("expected one successful result, got %+v", results)
	}
}
