package buildinfo

import (
	"regexp"
	"runtime/debug"
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

func TestResolveVersion(t *testing.T) {
	revision := debug.BuildSetting{Key: "vcs.revision", Value: "60bc33653ac789d9e138b0908403976b5f994023"}
	dirty := debug.BuildSetting{Key: "vcs.modified", Value: "true"}
	clean := debug.BuildSetting{Key: "vcs.modified", Value: "false"}

	tests := []struct {
		name string
		bi   *debug.BuildInfo
		want string
	}{
		{
			name: "module version",
			bi:   &debug.BuildInfo{Main: debug.Module{Version: testVersion}},
			want: testVersion,
		},
		{
			name: "module pseudo version",
			bi: &debug.BuildInfo{
				Main:     debug.Module{Version: "v0.0.0-20260808122403-f0b7c8384bac"},
				Settings: []debug.BuildSetting{revision, clean},
			},
			want: "v0.0.0-20260808122403-f0b7c8384bac",
		},
		{
			name: "clean checkout falls back to the short revision",
			bi: &debug.BuildInfo{
				Main:     debug.Module{Version: "(devel)"},
				Settings: []debug.BuildSetting{revision, clean},
			},
			want: "60bc336",
		},
		{
			name: "dirty checkout is marked",
			bi: &debug.BuildInfo{
				Main:     debug.Module{Version: "(devel)"},
				Settings: []debug.BuildSetting{revision, dirty},
			},
			want: "60bc336-dirty",
		},
		{
			name: "short revision is not truncated",
			bi: &debug.BuildInfo{
				Settings: []debug.BuildSetting{{Key: "vcs.revision", Value: "60bc"}, dirty},
			},
			want: "60bc-dirty",
		},
		{
			name: "no version information at all",
			bi:   &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}},
			want: devVersion,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := resolveVersion(tc.bi); got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

func TestResolveDate(t *testing.T) {
	bi := &debug.BuildInfo{Settings: []debug.BuildSetting{{Key: "vcs.time", Value: "2025-03-06T15:18:47Z"}}}
	if got := resolveDate(bi); got != "2025-03-06T15:18:47Z" {
		t.Fatalf("expected the vcs time, got %q", got)
	}

	if got := resolveDate(&debug.BuildInfo{}); got != unknownDate {
		t.Fatalf("expected %q, got %q", unknownDate, got)
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
