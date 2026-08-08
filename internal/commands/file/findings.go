package file

import (
	"cmp"
	"slices"
	"sync"
)

// findingsReport collects the results that carry findings during a directory
// run, so they can be listed once the progress bar is out of the way. A
// directory run reports only counts while it works, and a count alone does not
// tell you which file the malware was in.
//
// Results arrive from one goroutine per in-flight file, hence the lock.
type findingsReport struct {
	mu      sync.Mutex
	results []resultRecord
}

// add records a result if it carries findings.
func (r *findingsReport) add(record *resultRecord) {
	if len(record.findings) == 0 {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.results = append(r.results, *record)
}

// sorted returns the collected results ordered by path, so that a run over the
// same tree prints the same report regardless of the order files finished in.
func (r *findingsReport) sorted() []resultRecord {
	r.mu.Lock()
	defer r.mu.Unlock()

	sorted := slices.Clone(r.results)
	slices.SortFunc(sorted, func(a, b resultRecord) int {
		return cmp.Compare(a.path, b.path)
	})
	return sorted
}
