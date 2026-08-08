package terminal

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// redrawInterval bounds how often a Progress repaints, so that a fast upload or
// a directory of thousands of small files does not flood the terminal.
const redrawInterval = 100 * time.Millisecond

// Progress is a progress bar measured in bytes. It is safe for concurrent use:
// bytes may be reported from many upload goroutines while the label is updated
// from another. Nothing is drawn until Add reports the first bytes, and the
// zero total case draws nothing at all.
type Progress struct {
	total   uint64
	current atomic.Uint64

	mu       sync.Mutex
	label    string
	lastDraw time.Time
	finished bool
}

// NewProgress creates a progress bar for total bytes. A zero total is valid and
// renders nothing.
func NewProgress(label string, total uint64) *Progress {
	return &Progress{label: label, total: total}
}

// Add records n more bytes of progress and repaints, at most once per
// redrawInterval.
func (p *Progress) Add(n uint64) {
	current := p.current.Add(n)

	p.mu.Lock()
	defer p.mu.Unlock()
	p.draw(current, false)
}

// SetLabel replaces the label and repaints, at most once per redrawInterval.
func (p *Progress) SetLabel(label string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.label = label
	p.draw(p.current.Load(), false)
}

// Done paints the bar one final time and terminates the line. It is idempotent,
// so a caller that finishes the bar early can safely defer another call.
func (p *Progress) Done() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.finished {
		return
	}
	p.finished = true
	p.draw(p.current.Load(), true)
}

// Current returns the bytes reported so far.
func (p *Progress) Current() uint64 {
	return p.current.Load()
}

// draw repaints the bar. The caller must hold p.mu.
func (p *Progress) draw(current uint64, final bool) {
	if p.total == 0 || p.finished && !final {
		return
	}
	if !final && time.Since(p.lastDraw) < redrawInterval {
		return
	}
	p.lastDraw = time.Now()

	current = min(current, p.total)
	stats := fmt.Sprintf("%d%% (%s/%s)", percentOf(current, p.total), FormatBytes(current), FormatBytes(p.total))
	renderProgress(p.label, current, p.total, stats, final)
}
