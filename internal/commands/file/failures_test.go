package file

import (
	"errors"
	"fmt"
	"sync"
	"testing"
)

func TestFailureReportOnlyKeepsFailures(t *testing.T) {
	report := &failureReport{}
	report.add(&resultRecord{path: "clean.txt"})
	report.add(&resultRecord{path: "rejected.exe", err: errors.New("status 413")})

	got := report.sorted()
	if len(got) != 1 {
		t.Fatalf("expected 1 result, got %d: %+v", len(got), got)
	}
	if got[0].path != "rejected.exe" {
		t.Fatalf("expected rejected.exe, got %s", got[0].path)
	}
}

func TestFailureReportSortsByPath(t *testing.T) {
	report := &failureReport{}
	for _, path := range []string{"c.txt", "a.txt", "b.txt"} {
		report.add(&resultRecord{path: path, err: errors.New("status 500")})
	}

	got := report.sorted()
	for i, want := range []string{"a.txt", "b.txt", "c.txt"} {
		if got[i].path != want {
			t.Fatalf("expected %s at position %d, got %s", want, i, got[i].path)
		}
	}
}

func TestFailureReportConcurrentAdd(t *testing.T) {
	// results arrive from one goroutine per in-flight file
	report := &failureReport{}

	var wg sync.WaitGroup
	for i := range 50 {
		wg.Go(func() {
			report.add(&resultRecord{path: fmt.Sprintf("file-%02d.txt", i), err: errors.New("status 429")})
		})
	}
	wg.Wait()

	got := report.sorted()
	if len(got) != 50 {
		t.Fatalf("expected 50 results, got %d", len(got))
	}
	for i, result := range got {
		if want := fmt.Sprintf("file-%02d.txt", i); result.path != want {
			t.Fatalf("expected %s at position %d, got %s", want, i, result.path)
		}
	}
}

func TestFailureReportSortedIsASnapshot(t *testing.T) {
	report := &failureReport{}
	report.add(&resultRecord{path: "a.txt", err: errors.New("status 500")})

	snapshot := report.sorted()
	report.add(&resultRecord{path: "b.txt", err: errors.New("status 500")})

	if len(snapshot) != 1 {
		t.Fatalf("expected the snapshot to be unaffected, got %d results", len(snapshot))
	}
}
