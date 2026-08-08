package buildinfo

import (
	"regexp"
	"strings"
	"testing"
)

const testVersion = "1.4.2"

func TestVersionIsNeverEmpty(t *testing.T) {
	if Version() == "" {
		t.Fatal("expected Version() to never be empty")
	}
}

func TestVersionPrefersLdflag(t *testing.T) {
	original := version
	t.Cleanup(func() { version = original })

	version = testVersion
	if Version() != testVersion {
		t.Fatalf("expected Version() %q, got %q", testVersion, Version())
	}
}

func TestVersionFallsBackToVcsRevision(t *testing.T) {
	// the test binary is built with -buildvcs, so with no ldflag set we expect
	// the short revision rather than the "dev" placeholder
	if version != "" {
		t.Skip("binary built with a version ldflag")
	}
	v := Version()
	if v == devVersion {
		t.Fatalf("expected a vcs revision, got %q", v)
	}
	if len(strings.TrimSuffix(v, "-dirty")) != 7 {
		t.Fatalf("expected a 7 character short revision, got %q", v)
	}
}

func TestDateIsNeverEmpty(t *testing.T) {
	if Date() == "" {
		t.Fatal("expected Date() to never be empty")
	}
}

func TestDatePrefersLdflag(t *testing.T) {
	original := date
	t.Cleanup(func() { date = original })

	date = "2026-08-08T00:00:00Z"
	if Date() != "2026-08-08T00:00:00Z" {
		t.Fatalf("expected Date() '2026-08-08T00:00:00Z', got %q", Date())
	}
}

func TestUserAgentCarriesVersion(t *testing.T) {
	original := version
	t.Cleanup(func() { version = original })

	version = testVersion
	ua := UserAgent()

	if !strings.HasPrefix(ua, "scanii-cli/"+testVersion+" ") {
		t.Fatalf("expected user agent to carry the version, got %q", ua)
	}

	pattern := regexp.MustCompile(`^scanii-cli/\S+ \(go\S+; \w+/\w+\)$`)
	if !pattern.MatchString(ua) {
		t.Fatalf("expected user agent to match %s, got %q", pattern, ua)
	}
}

func TestUserAgentVersionIsNeverEmpty(t *testing.T) {
	// regression guard for https://github.com/scanii/scanii-cli/issues/85
	ua := UserAgent()

	product, _, found := strings.Cut(ua, " ")
	if !found {
		t.Fatalf("expected a comment section in the user agent, got %q", ua)
	}
	if !strings.HasPrefix(product, "scanii-cli/") {
		t.Fatalf("expected user agent to start with 'scanii-cli/', got %q", ua)
	}
	if strings.TrimPrefix(product, "scanii-cli/") == "" {
		t.Fatalf("expected a non-empty version in the user agent, got %q", ua)
	}
}
