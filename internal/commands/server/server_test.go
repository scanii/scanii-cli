package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/uvasoftware/scanii-cli/internal/engine"
)

// writeConfig writes contents to a temp file and returns its path.
func writeConfig(t *testing.T, name, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
		t.Fatalf("failed to write config: %s", err)
	}
	return path
}

func TestLoadEngineConfigRejectsBadInput(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		wantsErr string
	}{
		{
			name:     "missing file",
			path:     filepath.Join(t.TempDir(), "does-not-exist.json"),
			wantsErr: "opening engine config",
		},
		{
			name:     "not json at all",
			path:     writeConfig(t, "broken.json", "not json at all"),
			wantsErr: "loading engine config",
		},
		{
			name:     "valid json but not an engine config",
			path:     writeConfig(t, "wrong-shape.json", `{"nope": 1}`),
			wantsErr: "unknown field",
		},
		{
			name:     "empty file",
			path:     writeConfig(t, "empty.json", ""),
			wantsErr: "loading engine config",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			eng, err := engine.New()
			if err != nil {
				t.Fatalf("engine.New: %s", err)
			}

			err = loadEngineConfig(eng, tc.path)
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), tc.wantsErr) {
				t.Fatalf("expected the error to mention %q, got %q", tc.wantsErr, err)
			}
		})
	}
}

func TestLoadEngineConfigReplacesRules(t *testing.T) {
	eng, err := engine.New()
	if err != nil {
		t.Fatalf("engine.New: %s", err)
	}

	builtIn := eng.RuleCount()
	if builtIn == 0 {
		t.Fatal("expected the built-in config to carry rules")
	}

	path := writeConfig(t, "custom.json", `{
		"rules": [
			{"format": "sha1", "content": "7da9d3b0c68b1d0543acb65af4220a4745607557", "result": "content.custom.my-own-rule"}
		]
	}`)

	if err := loadEngineConfig(eng, path); err != nil {
		t.Fatalf("loadEngineConfig failed: %s", err)
	}
	if eng.RuleCount() != 1 {
		t.Fatalf("expected the custom config to replace the built-in rules, got %d rules", eng.RuleCount())
	}
}

func TestRunServerFailsOnBadEngineConfig(t *testing.T) {
	// the whole point of https://github.com/scanii/scanii-cli/issues/15: the
	// server used to ignore the flag and start with the built-in rules. It
	// returns before binding a port, so no address is needed.
	err := RunServer(&Flags{
		Address: "127.0.0.1:0",
		Key:     "key",
		Secret:  "secret",
		Data:    t.TempDir(),
		Engine:  filepath.Join(t.TempDir(), "does-not-exist.json"),
	})

	if err == nil {
		t.Fatal("expected RunServer to fail on a missing engine config")
	}
	if !strings.Contains(err.Error(), "opening engine config") {
		t.Fatalf("expected an engine config error, got %q", err)
	}
}

func TestCallbackWaitFlagOverridesEngineConfig(t *testing.T) {
	config := writeConfig(t, "slow-callbacks.json", `{"callback_wait": "5s", "rules": []}`)

	tests := []struct {
		name string
		flag *time.Duration
		want time.Duration
	}{
		{name: "config wins when the flag is not passed", flag: nil, want: 5 * time.Second},
		{name: "flag wins when passed", flag: ptr(2 * time.Second), want: 2 * time.Second},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			eng, err := engine.New()
			if err != nil {
				t.Fatalf("engine.New: %s", err)
			}

			if err := loadEngineConfig(eng, config); err != nil {
				t.Fatalf("loadEngineConfig failed: %s", err)
			}
			if tc.flag != nil {
				eng.SetCallbackWait(*tc.flag)
			}

			if got := eng.CallbackWait(); got != tc.want {
				t.Fatalf("expected a callback wait of %s, got %s", tc.want, got)
			}
		})
	}
}

func ptr[T any](v T) *T {
	return &v
}
