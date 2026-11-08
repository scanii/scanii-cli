//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	defaultPort = "4101"
	// the local server's engine flags content that is exactly this string
	malwareContent = "38DCC0C9-7FB6-4D0D-9C37-288A380C6BB9"
	malwareFinding = "content.malicious.local-test-file"
	cleanContent   = "hello from the scanii-cli e2e suite\n"
	buildVersion   = "0.0.0-e2e"
)

var idLine = regexp.MustCompile(`(?m)^\s*id:\s+(\S+)\s*$`)

// suite is one sc binary, one running sc server and a home directory of their own.
type suite struct {
	ctx      context.Context
	bin      string
	home     string
	port     string
	key      string
	secret   string
	endpoint string // what a profile points at: localhost makes the client use plain http
}

func TestEndToEnd(t *testing.T) {
	s := setup(t)

	// stops the run at the first failed step: later steps use what earlier ones created
	step := func(name string, f func(t *testing.T)) {
		t.Helper()
		if !t.Run(name, f) {
			t.FailNow()
		}
	}

	step("version", func(t *testing.T) {
		out := s.sc(t, 0, "version")
		mustContain(t, out, "Scanii CLI", "Version:", buildVersion)
	})

	step("no profile", func(t *testing.T) {
		out := s.sc(t, 1, "ping")
		mustContain(t, out, "no default profile configured")
	})

	step("profile create", func(t *testing.T) {
		out := s.sc(t, 0, "profile", "create", "default", "--endpoint", s.endpoint, "--credentials", s.key+":"+s.secret)
		mustContain(t, out, `Profile "default" saved`)
		out = s.sc(t, 0, "profile", "list")
		mustContain(t, out, "default", "endpoint="+s.endpoint, "key="+s.key)
	})

	step("ping", func(t *testing.T) {
		out := s.sc(t, 0, "ping")
		mustContain(t, out, "Ping successful", s.endpoint)
	})

	step("account", func(t *testing.T) {
		out := s.sc(t, 0, "account")
		mustContain(t, out, "Account information", "Balance:")
	})

	dir := t.TempDir()
	clean := writeFile(t, filepath.Join(dir, "clean.txt"), cleanContent)
	malware := writeFile(t, filepath.Join(dir, "sub", "malware"), malwareContent)
	writeFile(t, filepath.Join(dir, "empty"), "")

	var cleanID, malwareID string

	step("process clean file", func(t *testing.T) {
		out := s.sc(t, 0, "files", "process", clean)
		mustContain(t, out, "checksum/sha1:  "+sha1Hex(cleanContent), "findings:       none",
			"Files with findings: 0, unable to process: 0 and successfully processed: 1")
		mustNotContain(t, out, "content.malicious")
		cleanID = resultID(t, out)
	})

	step("process malware file", func(t *testing.T) {
		out := s.sc(t, 0, "files", "process", malware)
		mustContain(t, out, "checksum/sha1:  "+sha1Hex(malwareContent), "findings:       "+malwareFinding,
			"Files with findings: 1, unable to process: 0 and successfully processed: 1")
		malwareID = resultID(t, out)
	})

	step("process directory", func(t *testing.T) {
		out := s.sc(t, 0, "files", "process", dir)
		mustContain(t, out, "Skipping 1 empty file(s)", "## Files with findings", "findings:       "+malwareFinding,
			"Files with findings: 1, unable to process: 0 and successfully processed: 2")
	})

	step("process missing path", func(t *testing.T) {
		out := s.sc(t, 1, "files", "process", filepath.Join(dir, "does-not-exist"))
		mustContain(t, out, "failed to stat path")
	})

	step("retrieve", func(t *testing.T) {
		out := s.sc(t, 0, "files", "retrieve", malwareID)
		mustContain(t, out, "id:             "+malwareID, "findings:       "+malwareFinding)
	})

	step("trace", func(t *testing.T) {
		out := s.sc(t, 0, "files", "trace", malwareID)
		mustContain(t, out, "id:             "+malwareID)
	})

	step("async then retrieve", func(t *testing.T) {
		out := s.sc(t, 0, "files", "async", malware)
		mustContain(t, out, "location:")
		id := resultID(t, out)
		out = s.sc(t, 0, "files", "retrieve", "--wait", "30", id)
		mustContain(t, out, "Result found", "findings:       "+malwareFinding)
	})

	step("fetch", func(t *testing.T) {
		location := fmt.Sprintf("http://127.0.0.1:%s/static/samples/malware", s.port)
		out := s.sc(t, 0, "files", "fetch", "--wait", "30", location)
		mustContain(t, out, "Request accepted", "Result found", "findings:       "+malwareFinding)
	})

	step("delete", func(t *testing.T) {
		out := s.sc(t, 0, "files", "delete", cleanID)
		mustContain(t, out, "Processing result "+cleanID+" deleted")
		out = s.sc(t, 1, "files", "retrieve", cleanID)
		mustContain(t, out, "status code 404")
	})

	step("delete trace", func(t *testing.T) {
		out := s.sc(t, 0, "files", "delete-trace", malwareID)
		mustContain(t, out, "Processing trace "+malwareID+" deleted")
		out = s.sc(t, 1, "files", "trace", malwareID)
		mustContain(t, out, "no trace exists")
	})

	step("auth token", func(t *testing.T) {
		out := s.sc(t, 0, "auth-token", "create", "--timeout", "600")
		mustContain(t, out, "expires:")
		token := resultID(t, out)

		out = s.sc(t, 0, "auth-token", "retrieve", token)
		mustContain(t, out, "id:             "+token)

		// a token authenticates on its own: the id as the key and no secret. The
		// authentication checks use account, not ping: sc ping reports success
		// whatever status the API answers with.
		s.sc(t, 0, "profile", "create", "token", "--endpoint", s.endpoint, "--credentials", token+":")
		out = s.sc(t, 0, "-p", "token", "account")
		mustContain(t, out, "Account information")

		out = s.sc(t, 0, "auth-token", "delete", token)
		mustContain(t, out, "Token "+token+" deleted")
		out = s.sc(t, 1, "-p", "token", "account")
		mustContain(t, out, "status: 401")
	})

	step("wrong credentials", func(t *testing.T) {
		s.sc(t, 0, "profile", "create", "wrong", "--endpoint", s.endpoint, "--credentials", s.key+":not-the-secret")
		out := s.sc(t, 1, "-p", "wrong", "account")
		mustContain(t, out, "status: 401")
		out = s.sc(t, 1, "-p", "wrong", "files", "process", clean)
		mustContain(t, out, "status 401", "unable to process: 1")
	})

	step("profile delete", func(t *testing.T) {
		out := s.sc(t, 0, "profile", "delete", "wrong")
		mustContain(t, out, `Profile "wrong" deleted`)
		out = s.sc(t, 1, "-p", "wrong", "ping")
		mustContain(t, out, `profile "wrong" not found`)
	})
}

// setup builds sc, starts sc server on SC_E2E_PORT and waits until it answers
// a ping with this run's credentials. The server is stopped when the test ends,
// failed or not.
func setup(t *testing.T) *suite {
	t.Helper()

	// everything the suite starts dies before go test's own timeout would
	// abandon it: CommandContext kills on cancel
	ctx := context.Background()
	if deadline, ok := t.Deadline(); ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(ctx, deadline.Add(-10*time.Second))
		t.Cleanup(cancel)
	}

	port := os.Getenv("SC_E2E_PORT")
	if port == "" {
		port = defaultPort
	}

	s := &suite{
		ctx:      ctx,
		home:     t.TempDir(),
		port:     port,
		key:      "e2e-" + randomHex(t, 6),
		secret:   randomHex(t, 16),
		endpoint: "localhost:" + port,
	}
	s.bin = build(t, ctx)

	// listen on loopback only; the random credentials keep a ping from passing
	// against some other server that already holds the port
	addr := "127.0.0.1:" + port
	server := exec.CommandContext(ctx, s.bin, "server", "--address", addr, "--key", s.key, "--secret", s.secret,
		"--data", filepath.Join(s.home, "data"))
	server.Env = s.env()
	var log syncBuffer
	server.Stdout = &log
	server.Stderr = &log
	if err := server.Start(); err != nil {
		t.Fatalf("starting sc server: %v", err)
	}
	exited := make(chan error, 1)
	go func() { exited <- server.Wait() }()
	t.Cleanup(func() {
		_ = server.Process.Kill()
		<-exited
		if t.Failed() {
			t.Logf("sc server output:\n%s", log.String())
		}
	})

	ready := time.Now().Add(30 * time.Second)
	for {
		select {
		case err := <-exited:
			// put back for the cleanup, which also prints the server's output
			exited <- err
			t.Fatalf("sc server exited before it was ready: %v", err)
		default:
		}
		if s.serverReady(addr) {
			return s
		}
		if time.Now().After(ready) {
			t.Fatalf("sc server did not answer on %s within 30s", addr)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func (s *suite) serverReady(addr string) bool {
	ctx, cancel := context.WithTimeout(s.ctx, 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr+"/v2.2/ping", http.NoBody)
	if err != nil {
		return false
	}
	req.SetBasicAuth(s.key, s.secret)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// build compiles sc into a temporary directory, stamping a known version.
func build(t *testing.T, ctx context.Context) string {
	t.Helper()
	gobin, err := exec.LookPath("go")
	if err != nil {
		t.Fatalf("go not on PATH: %v", err)
	}
	bin := filepath.Join(t.TempDir(), "sc")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	ldflags := "-X github.com/uvasoftware/scanii-cli/internal/buildinfo.version=" + buildVersion
	cmd := exec.CommandContext(ctx, gobin, "build", "-ldflags="+ldflags, "-o", bin, "./cmd/sc")
	cmd.Dir = ".."
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	return bin
}

// env isolates sc from the developer's own profiles: they live under the
// user's home directory, which is HOME on Unix and USERPROFILE on Windows.
func (s *suite) env() []string {
	return append(os.Environ(),
		"HOME="+s.home,
		"USERPROFILE="+s.home,
		"GOPS_CONFIG_DIR="+filepath.Join(s.home, "gops"),
		"NO_COLOR=1",
		"TERM=dumb",
	)
}

// sc runs the binary and fails the test unless it exits with wantCode. It
// returns stdout and stderr together.
func (s *suite) sc(t *testing.T, wantCode int, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(s.ctx, 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, s.bin, args...)
	cmd.Env = s.env()
	cmd.Dir = s.home
	out, err := cmd.CombinedOutput()
	code := 0
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("sc %s: %v", strings.Join(args, " "), err)
		}
		code = exitErr.ExitCode()
	}
	if code != wantCode {
		t.Fatalf("sc %s: exit code %d, want %d; output:\n%s", strings.Join(args, " "), code, wantCode, out)
	}
	// Windows consoles aside, sc prints \n; normalize so assertions read the same everywhere
	return strings.ReplaceAll(string(out), "\r\n", "\n")
}

func resultID(t *testing.T, out string) string {
	t.Helper()
	m := idLine.FindStringSubmatch(out)
	if m == nil {
		t.Fatalf("no id: line in output:\n%s", out)
	}
	return m[1]
}

func mustContain(t *testing.T, out string, want ...string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(out, w) {
			t.Errorf("output lacks %q:\n%s", w, out)
		}
	}
	if t.Failed() {
		t.FailNow()
	}
}

func mustNotContain(t *testing.T, out, unwanted string) {
	t.Helper()
	if strings.Contains(out, unwanted) {
		t.Fatalf("output contains %q:\n%s", unwanted, out)
	}
}

func writeFile(t *testing.T, path, content string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func sha1Hex(content string) string {
	sum := sha1.Sum([]byte(content))
	return hex.EncodeToString(sum[:])
}

func randomHex(t *testing.T, n int) string {
	t.Helper()
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(b)
}

// syncBuffer collects the server's output, which arrives on two pipes at once.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
