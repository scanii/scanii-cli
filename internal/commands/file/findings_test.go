package file

import (
	"fmt"
	"sync"
	"testing"
)

func TestFindingsReportOnlyKeepsResultsWithFindings(t *testing.T) {
	report := &findingsReport{}
	report.add(&resultRecord{path: "clean.txt"})
	report.add(&resultRecord{path: "empty-findings.txt", findings: []string{}})
	report.add(&resultRecord{path: "malware.exe", findings: []string{"content.malicious.eicar-test-signature"}})

	got := report.sorted()
	if len(got) != 1 {
		t.Fatalf("expected 1 result, got %d: %+v", len(got), got)
	}
	if got[0].path != "malware.exe" {
		t.Fatalf("expected malware.exe, got %s", got[0].path)
	}
}

func TestFindingsReportSortsByPath(t *testing.T) {
	report := &findingsReport{}
	for _, path := range []string{"c.txt", "a.txt", "b.txt"} {
		report.add(&resultRecord{path: path, findings: []string{"finding"}})
	}

	got := report.sorted()
	for i, want := range []string{"a.txt", "b.txt", "c.txt"} {
		if got[i].path != want {
			t.Fatalf("expected %s at position %d, got %s", want, i, got[i].path)
		}
	}
}

func TestFindingsReportConcurrentAdd(t *testing.T) {
	// results arrive from one goroutine per in-flight file
	report := &findingsReport{}

	var wg sync.WaitGroup
	for i := range 50 {
		wg.Go(func() {
			report.add(&resultRecord{path: fmt.Sprintf("file-%02d.txt", i), findings: []string{"finding"}})
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

func TestFindingsReportSortedIsASnapshot(t *testing.T) {
	report := &findingsReport{}
	report.add(&resultRecord{path: "a.txt", findings: []string{"finding"}})

	snapshot := report.sorted()
	report.add(&resultRecord{path: "b.txt", findings: []string{"finding"}})

	if len(snapshot) != 1 {
		t.Fatalf("expected the snapshot to be unaffected, got %d results", len(snapshot))
	}
}
