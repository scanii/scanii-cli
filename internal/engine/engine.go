package engine

import (
	"bytes"
	"crypto/sha1" //nolint want "crypto/sha1 is not recommended"[:<gosec>]
	"crypto/sha256"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gabriel-vasile/mimetype"
)

//go:embed default.json
var defaultConfig string

type Engine struct {
	config *Config
	// callbackWait is held separately, in nanoseconds, because the callback
	// runner reads it from its own goroutine every time it delivers.
	callbackWait  atomic.Int64
	callbackQueue chan callbackItem
}

type Rule struct {
	Format  string `json:"format"`
	Content string `json:"content"`
	Result  string `json:"result"`
}
type Config struct {
	Rules        []Rule    `json:"rules"`
	CallbackWait *Duration `json:"callback_wait"`
}

// Duration is a time.Duration that reads from JSON as a duration string, so a
// config says "100ms" rather than 100000000. A bare number would be nanoseconds,
// which is a trap: "callback_wait": 100 means 100ns, not 100ms.
type Duration time.Duration

func (d *Duration) UnmarshalJSON(data []byte) error {
	var text string
	if err := json.Unmarshal(data, &text); err != nil {
		return fmt.Errorf(`callback_wait must be a duration string such as "100ms": %w`, err)
	}

	parsed, err := time.ParseDuration(text)
	if err != nil {
		return fmt.Errorf("callback_wait %q is not a valid duration: %w", text, err)
	}

	*d = Duration(parsed)
	return nil
}

func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Duration(d).String())
}

func New() (*Engine, error) {

	engine := &Engine{
		config: &Config{
			Rules: make([]Rule, 0),
		},
	}
	slog.Debug("loading default config")
	err := engine.LoadConfig(strings.NewReader(defaultConfig))
	if err != nil {
		return nil, err
	}

	engine.callbackQueue = engine.newRunner()

	return engine, nil
}

func (e *Engine) LoadConfig(reader io.Reader) error {
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	err := decoder.Decode(e.config)
	if err != nil {
		return err
	}

	if e.config.CallbackWait != nil {
		e.SetCallbackWait(time.Duration(*e.config.CallbackWait))
	}
	return nil
}

// SetCallbackWait overrides how long the engine waits before delivering a
// callback. It takes effect for callbacks that have not been delivered yet,
// which is what lets --callback-wait win over a config file loaded before it.
func (e *Engine) SetCallbackWait(wait time.Duration) {
	e.callbackWait.Store(int64(wait))
}

// CallbackWait returns the delay applied before delivering a callback.
func (e *Engine) CallbackWait() time.Duration {
	return time.Duration(e.callbackWait.Load())
}

func (e *Engine) RuleCount() int {
	return len(e.config.Rules)

}

type Result struct {
	ID            string
	Sha1          string
	Sha256        string
	ContentLength uint64
	Findings      []string
	ContentType   string
	CreationDate  string
	Metadata      map[string]string
	Error         string
}

func (e *Engine) Process(contents io.Reader) (Result, error) {
	result := Result{
		CreationDate: time.Now().UTC().Format(time.RFC3339Nano),
	}

	result.Findings = []string{}
	s1 := sha1.New() //nolint:gosec
	s2 := sha256.New()
	dest := io.MultiWriter(s1, s2)

	// detecting mime type
	mime, recycledInput, err := recycleReader(contents)
	if err != nil {
		return result, err
	}

	i, err := io.Copy(dest, recycledInput)
	if err != nil {
		return result, err
	}

	result.Sha1 = fmt.Sprintf("%x", s1.Sum(nil))
	result.Sha256 = fmt.Sprintf("%x", s2.Sum(nil))
	result.ContentLength = uint64(i) //nolint:gosec // G115: io.Copy never returns negative values

	result.ContentType = mime

	appendIfMissing := func(slice []string, s string) []string {
		for _, ele := range slice {
			if ele == s {
				return slice
			}
		}
		return append(slice, s)
	}

	// looking for matches in the rules:
	for _, rule := range e.config.Rules {
		switch rule.Format {
		case "sha1":
			if result.Sha1 == rule.Content {
				result.Findings = appendIfMissing(result.Findings, rule.Result)
			}
		case "sha256":
			if result.Sha256 == rule.Content {
				result.Findings = appendIfMissing(result.Findings, rule.Result)
			}
		}
	}

	return result, nil

}

func (e *Engine) QueueCallback(c string, r *Result) {
	e.callbackQueue <- callbackItem{
		result:      r,
		destination: c,
	}
}

// recycleReader returns the MIME type of input and a new reader
// containing the whole data from input.
func recycleReader(input io.Reader) (mimeType string, recycled io.Reader, err error) {
	// header will store the bytes mimetype uses for detection.
	header := bytes.NewBuffer(nil)

	// After DetectReader, the data read from input is copied into header.
	mtype, err := mimetype.DetectReader(io.TeeReader(input, header))
	if err != nil {
		return
	}

	// Concatenate back the header to the rest of the file.
	// recycled now contains the complete, original data.
	recycled = io.MultiReader(header, input)

	return mtype.String(), recycled, err
}
