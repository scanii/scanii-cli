package file

import (
	"context"
	"strings"
	"testing"
)

func TestCallFileTraceEmptyID(t *testing.T) {
	c, err := ts.Profile.Client()
	if err != nil {
		t.Fatalf("failed to create client: %s", err)
	}

	_, err = callFileTrace(context.Background(), c, "")
	if err == nil {
		t.Fatalf("expected error for empty id")
	}
}

func TestCallFileTraceUnknownID(t *testing.T) {
	c, err := ts.Profile.Client()
	if err != nil {
		t.Fatalf("failed to create client: %s", err)
	}

	_, err = callFileTrace(context.Background(), c, "doesnotexist")
	if err == nil {
		t.Fatalf("expected error for unknown id")
	}
	if !strings.Contains(err.Error(), "doesnotexist") {
		t.Fatalf("expected error to mention id, got %s", err)
	}
}

func TestCallFileTraceKnownID(t *testing.T) {
	c, err := ts.Profile.Client()
	if err != nil {
		t.Fatalf("failed to create client: %s", err)
	}

	// process a file first to get an id
	processed, err := runLocationProcess(
		context.Background(),
		c,
		"http://"+ts.Endpoint+"/static/eicar.txt",
		"",
		"",
	)
	if err != nil {
		t.Fatalf("failed to process file: %s", err)
	}
	if processed.id == "" {
		t.Fatalf("expected processed result to have an id")
	}

	trace, err := callFileTrace(context.Background(), c, processed.id)
	if err != nil {
		t.Fatalf("failed to retrieve trace: %s", err)
	}
	if trace.id != processed.id {
		t.Fatalf("expected trace id %s, got %s", processed.id, trace.id)
	}
	if len(trace.events) < 2 {
		t.Fatalf("expected at least 2 trace events, got %d", len(trace.events))
	}
	for i, ev := range trace.events {
		if ev.timestamp == "" {
			t.Errorf("event[%d]: empty timestamp", i)
		}
		if ev.message == "" {
			t.Errorf("event[%d]: empty message", i)
		}
	}
}
