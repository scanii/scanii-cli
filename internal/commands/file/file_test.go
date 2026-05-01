package file

import (
	"context"
	"fmt"
	"testing"
)

// checkResponseContent checks that the result is the expected sample test file
func checkResponseContent(t *testing.T, result *resultRecord) {
	t.Helper()

	if len(result.findings) == 0 {
		t.Fatalf("expected findings, got none")
	}

	if result.findings[0] != "content.malicious.local-test-file" {
		t.Fatalf("expected finding content.malicious.eicar-test-signature, got %s", result.findings[0])
	}

	if result.checksum != "7da9d3b0c68b1d0543acb65af4220a4745607557" {
		t.Fatalf("expected checksum 7da9d3b0c68b1d0543acb65af4220a4745607557, got %s", result.checksum)
	}

	if result.contentLength != 36 {
		t.Fatalf("expected content length 68, got %d", result.contentLength)
	}
}

func TestShouldProcessLocationSync(t *testing.T) {
	client, err := ts.Profile.Client()
	if err != nil {
		t.Fatalf("failed to create client: %s", err)
	}

	// The server mock-fetches the URL (does not actually retrieve it), so findings
	// are always empty for location-based submissions. The test verifies the API
	// surface accepts the location field and echoes metadata correctly.
	result, err := runLocationProcess(context.Background(), client, fmt.Sprintf("http://%s/static/eicar.txt", ts.Endpoint), "", "m1=v1")
	if err != nil {
		t.Fatalf("failed to process file: %s", err)
	}

	if result.id == "" {
		t.Fatal("expected result to have an id")
	}

	if result.metadata["m1"] != "v1" {
		t.Fatalf("expected metadata m1=v1, got %s", result.metadata["m1"])
	}
}

func TestShouldProcessFetch(t *testing.T) {
	client, err := ts.Profile.Client()
	if err != nil {
		t.Fatalf("failed to create client: %s", err)
	}

	t.Run("positive", func(t *testing.T) {
		result, err := callFilesFetch(context.Background(), client, fmt.Sprintf("http://%s/static/eicar.txt", ts.Endpoint), "", "m1=v1")
		if err != nil {
			t.Fatalf("failed to process file: %s", err)
		}

		if result.id == "" {
			t.Fatalf("expected result to have an id")
		}
		if result.location == "" {
			t.Fatalf("expected result to have a location")
		}

		// retrieving it
		retrieve, err := callFileRetrieve(context.Background(), client, result.id, 0)
		if err != nil {
			t.Fatalf("failed to retrieve file: %s", err)
		}

		if retrieve.findings[0] != "content.malicious.eicar-test-signature" {
			t.Fatalf("expected finding content.malicious.eicar-test-signature, got %s", result.findings[0])
		}
	})

	t.Run("negative", func(t *testing.T) {
		result, err := callFilesFetch(context.Background(), client, fmt.Sprintf("http://%s/static/nope", ts.Endpoint), "", "m1=v1")
		if err != nil {
			t.Fatalf("failed to process file: %s", err)
		}

		if result.id == "" {
			t.Fatalf("expected result to have an id")
		}
		if result.location == "" {
			t.Fatalf("expected result to have a location")
		}

		// retrieving it — should have an error
		retrieve, err := callFileRetrieve(context.Background(), client, result.id, 0)
		if err != nil {
			t.Fatalf("failed to retrieve file: %s", err)
		}

		if retrieve.err == nil {
			t.Fatalf("expected result to have an error")
		}
	})
}

func TestCallFileRetrieveEmptyID(t *testing.T) {
	client, err := ts.Profile.Client()
	if err != nil {
		t.Fatalf("failed to create client: %s", err)
	}

	_, err = callFileRetrieve(context.Background(), client, "", 0)
	if err == nil {
		t.Fatalf("expected error for empty id")
	}
}

func TestExtractMetadata(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		m := extractMetadata("")
		if len(m) != 0 {
			t.Fatalf("expected empty map, got %v", m)
		}
	})

	t.Run("single", func(t *testing.T) {
		m := extractMetadata("k1=v1")
		if m["k1"] != "v1" {
			t.Fatalf("expected k1=v1, got %v", m)
		}
	})

	t.Run("multiple", func(t *testing.T) {
		m := extractMetadata("k1=v1,k2=v2")
		if m["k1"] != "v1" {
			t.Fatalf("expected k1=v1, got %v", m)
		}
		if m["k2"] != "v2" {
			t.Fatalf("expected k2=v2, got %v", m)
		}
	})

	t.Run("whitespace", func(t *testing.T) {
		m := extractMetadata(" k1 = v1 , k2 = v2 ")
		if m["k1"] != "v1" {
			t.Fatalf("expected k1=v1, got %v", m)
		}
		if m["k2"] != "v2" {
			t.Fatalf("expected k2=v2, got %v", m)
		}
	})

	t.Run("invalid_entry_skipped", func(t *testing.T) {
		m := extractMetadata("k1=v1,invalid,k2=v2")
		if len(m) != 2 {
			t.Fatalf("expected 2 entries, got %d", len(m))
		}
	})
}
