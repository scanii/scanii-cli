package terminal

import (
	"strings"
	"sync"
	"testing"
)

func TestProgressReportsBytes(t *testing.T) {
	out := captureOut(func() {
		p := NewProgress("Uploading", 1000)
		p.Add(400)
		p.Done()
	})

	// non-TTY prints only the final line
	if strings.Count(out, "\n") != 1 {
		t.Fatalf("expected a single line, got %q", out)
	}
	if !strings.Contains(out, "Uploading") {
		t.Fatalf("expected the label, got %q", out)
	}
	if !strings.Contains(out, "40% (400 B/1 KB)") {
		t.Fatalf("expected byte formatted stats, got %q", out)
	}
}

func TestProgressDoneIsIdempotent(t *testing.T) {
	out := captureOut(func() {
		p := NewProgress("Uploading", 10)
		p.Done()
		p.Done()
		p.Add(10)
		p.Done()
	})

	if strings.Count(out, "\n") != 1 {
		t.Fatalf("expected exactly one final line, got %q", out)
	}
}

func TestProgressSetLabel(t *testing.T) {
	out := captureOut(func() {
		p := NewProgress("Files 0/6", 100)
		p.SetLabel("Files 6/6")
		p.Done()
	})

	if !strings.Contains(out, "Files 6/6") {
		t.Fatalf("expected the updated label, got %q", out)
	}
}

func TestProgressZeroTotal(t *testing.T) {
	out := captureOut(func() {
		p := NewProgress("Uploading", 0)
		p.Add(10)
		p.Done()
	})

	if out != "" {
		t.Fatalf("expected no output for a zero total, got %q", out)
	}
}

func TestProgressClampsOvershoot(t *testing.T) {
	// a file that grows between stat and read must not panic or exceed 100%
	out := captureOut(func() {
		p := NewProgress("Uploading", 100)
		p.Add(250)
		p.Done()
	})

	if !strings.Contains(out, "100%") {
		t.Fatalf("expected the bar to clamp at 100%%, got %q", out)
	}
}

func TestProgressCurrent(t *testing.T) {
	p := NewProgress("Uploading", 100)
	p.Add(30)
	p.Add(20)
	if p.Current() != 50 {
		t.Fatalf("expected 50 bytes, got %d", p.Current())
	}
}

func TestProgressConcurrentAdd(t *testing.T) {
	// directory mode reports bytes from one goroutine per in-flight file
	captureOut(func() {
		p := NewProgress("Files", 10_000)
		var wg sync.WaitGroup
		for range 10 {
			wg.Go(func() {
				for range 100 {
					p.Add(1)
				}
			})
		}
		wg.Wait()
		p.Done()

		if p.Current() != 1000 {
			t.Errorf("expected 1000 bytes, got %d", p.Current())
		}
	})
}
