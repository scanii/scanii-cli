package file

import (
	"cmp"
	"slices"
	"sync"
)

// failureReport collects the files a run could not process, so they can be
// listed once the progress bar is out of the way. A directory run reports only
// counts while it works, and "unable to process: 6" does not tell you which six
// files to retry.
//
// Results arrive from one goroutine per in-flight file, hence the lock.
type failureReport struct {
	mu      sync.Mutex
	results []resultRecord
}

// add records a result if it failed.
func (r *failureReport) add(record *resultRecord) {
	if record.err == nil {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.results = append(r.results, *record)
}

// sorted returns the collected failures ordered by path, so that a run over the
// same tree prints the same report regardless of the order files finished in.
func (r *failureReport) sorted() []resultRecord {
	r.mu.Lock()
	defer r.mu.Unlock()

	sorted := slices.Clone(r.results)
	slices.SortFunc(sorted, func(a, b resultRecord) int {
		return cmp.Compare(a.path, b.path)
	})
	return sorted
}
