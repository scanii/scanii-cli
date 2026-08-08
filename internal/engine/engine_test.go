package engine

import (
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	engine, err := New()
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(engine.config.Rules) == 0 {
		t.Fatalf("ruleset was not loaded")
	}
}

func TestRuleCount(t *testing.T) {
	engine, err := New()
	if err != nil {
		t.Fatal(err.Error())
	}
	if engine.RuleCount() == 0 {
		t.Fatalf("expected non-zero rule count")
	}
	if engine.RuleCount() != len(engine.config.Rules) {
		t.Fatalf("RuleCount() mismatch: got %d, want %d", engine.RuleCount(), len(engine.config.Rules))
	}
}

func TestLoadConfigCustomRules(t *testing.T) {
	engine := &Engine{config: &Config{Rules: make([]Rule, 0)}}
	config := `{"rules": [{"format": "sha1", "content": "abc123", "result": "test.finding"}]}`
	err := engine.LoadConfig(strings.NewReader(config))
	if err != nil {
		t.Fatal(err.Error())
	}
	if engine.RuleCount() != 1 {
		t.Fatalf("expected 1 rule, got %d", engine.RuleCount())
	}
	if engine.config.Rules[0].Result != "test.finding" {
		t.Fatalf("expected rule result test.finding, got %s", engine.config.Rules[0].Result)
	}
}

func TestLoadConfigInvalidJSON(t *testing.T) {
	engine := &Engine{config: &Config{Rules: make([]Rule, 0)}}
	err := engine.LoadConfig(strings.NewReader("not json"))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestProcessEmptyInput(t *testing.T) {
	engine, err := New()
	if err != nil {
		t.Fatal(err.Error())
	}
	result, err := engine.Process(strings.NewReader(""))
	if err != nil {
		t.Fatal(err.Error())
	}
	if result.ContentLength != 0 {
		t.Fatalf("expected content length 0, got %d", result.ContentLength)
	}
	if len(result.Findings) != 0 {
		t.Fatalf("expected no findings, got %v", result.Findings)
	}
}

func TestIdentifyEicar(t *testing.T) {
	engine, err := New()
	if err != nil {
		t.Fatal(err.Error())
	}

	result, err := engine.Process(strings.NewReader("X5O!P%@AP[4\\PZX54(P^)7CC)7}$EICAR-STANDARD-ANTIVIRUS-TEST-FILE!$H+H*"))
	if err != nil {
		return
	}

	if result.ContentLength == 0 {
		t.Fatalf("content length was not calculated")
	}

	if result.Findings[0] != "content.malicious.eicar-test-signature" {
		t.Fatalf("eicar file was not identified")
	}

	if result.ContentType != "text/plain; charset=utf-8" {
		t.Fatalf("content type was not identifiedd, got %s", result.ContentType)
	}

}

type junkReader struct {
	length    uint64
	readIndex uint64
}

func (r *junkReader) Read(p []byte) (n int, err error) {
	if r.readIndex >= r.length {
		return 0, io.EOF
	}
	for i := range p {
		p[i] = 7
	}

	r.readIndex += uint64(len(p))
	return len(p), nil
}

func TestEngine_Process(t *testing.T) {
	r := &junkReader{
		length: 11 * 1024 * 1024,
	}

	engine, err := New()
	if err != nil {
		t.Fatal(err.Error())
	}

	_, err = engine.Process(r)
	if err != nil {
		t.Fatal(err.Error())
	}
}

func TestSamples(t *testing.T) {
	tests := []struct {
		file            string
		expectedFinding string
	}{
		{"testdata/image.jpg", "content.image.nsfw.nudity"},
		{"testdata/language.txt", "content.en.language.nsfw.0"},
	}
	for _, test := range tests {
		t.Run(test.file, func(t *testing.T) {
			if open, err := os.Open(test.file); err != nil {
				t.Fatalf("failed to open file: %s", err)
			} else {
				defer open.Close()
				engine, err := New()
				if err != nil {
					t.Fatal(err.Error())
				}
				result, err := engine.Process(open)
				if err != nil {
					t.Fatal(err.Error())
				}
				if len(result.Findings) == 0 {
					t.Fatalf("expected findings, got none")
				}
				if result.Findings[0] != test.expectedFinding {
					t.Fatalf("expected finding %s, got %s", test.expectedFinding, result.Findings[0])
				}
			}

		})
	}
}

func BenchmarkEngine(b *testing.B) {
	// 11 mb of junk
	r := &junkReader{
		length: 10 * 1024 * 1024,
	}
	engine, err := New()
	if err != nil {
		b.Fatal(err.Error())
	}

	for i := 0; i < b.N; i++ {
		_, err = engine.Process(r)
		if err != nil {
			b.Fatal(err.Error())
		}
	}
}

func TestLoadConfigCallbackWait(t *testing.T) {
	tests := []struct {
		name    string
		config  string
		want    time.Duration
		wantErr string
	}{
		{
			name:   "duration string",
			config: `{"callback_wait": "2s", "rules": []}`,
			want:   2 * time.Second,
		},
		{
			name:   "sub second duration",
			config: `{"callback_wait": "250ms", "rules": []}`,
			want:   250 * time.Millisecond,
		},
		{
			// a bare number used to be read as nanoseconds, so "100" quietly meant 100ns
			name:    "bare number is rejected",
			config:  `{"callback_wait": 100, "rules": []}`,
			wantErr: "duration string",
		},
		{
			name:    "unparseable duration",
			config:  `{"callback_wait": "soon", "rules": []}`,
			wantErr: "not a valid duration",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			engine, err := New()
			if err != nil {
				t.Fatalf("New failed: %s", err)
			}

			err = engine.LoadConfig(strings.NewReader(tc.config))
			if tc.wantErr != "" {
				if err == nil {
					t.Fatal("expected an error")
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("expected the error to mention %q, got %q", tc.wantErr, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("LoadConfig failed: %s", err)
			}
			if engine.CallbackWait() != tc.want {
				t.Fatalf("expected a callback wait of %s, got %s", tc.want, engine.CallbackWait())
			}
		})
	}
}

func TestDefaultConfigCallbackWait(t *testing.T) {
	engine, err := New()
	if err != nil {
		t.Fatalf("New failed: %s", err)
	}

	// regression guard: default.json used to say 100, which is 100ns not 100ms
	if got := engine.CallbackWait(); got != 100*time.Millisecond {
		t.Fatalf("expected the built-in callback wait to be 100ms, got %s", got)
	}
}

func TestSetCallbackWaitOverridesConfig(t *testing.T) {
	engine, err := New()
	if err != nil {
		t.Fatalf("New failed: %s", err)
	}

	if err := engine.LoadConfig(strings.NewReader(`{"callback_wait": "5s", "rules": []}`)); err != nil {
		t.Fatalf("LoadConfig failed: %s", err)
	}

	engine.SetCallbackWait(time.Second)
	if got := engine.CallbackWait(); got != time.Second {
		t.Fatalf("expected the override to win, got %s", got)
	}
}
