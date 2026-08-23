package profile

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/uvasoftware/scanii-cli/internal/buildinfo"
)

func TestSaveAndLoadConfig(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("USERPROFILE", tmpHome)

	c := &Profile{
		Credentials: "testkey:testsecret",
		Endpoint:    "api-us1.scanii.com",
	}

	err := save("testprofile", c)
	if err != nil {
		t.Fatalf("failed to save config: %s", err)
	}

	// verify file exists at expected path
	expectedPath := filepath.Join(tmpHome, ".config", "scanii-cli", "testprofile.json")
	if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
		t.Fatalf("expected config file at %s", expectedPath)
	}

	// load it back
	loaded, err := Load("testprofile")
	if err != nil {
		t.Fatalf("failed to load config: %s", err)
	}

	if loaded.Credentials != "testkey:testsecret" {
		t.Fatalf("expected Credentials 'testkey:testsecret', got %q", loaded.Credentials)
	}
	if loaded.APIKey() != "testkey" {
		t.Fatalf("expected apiKey 'testkey', got %q", loaded.APIKey())
	}
	if loaded.APISecret() != "testsecret" {
		t.Fatalf("expected apiSecret 'testsecret', got %q", loaded.APISecret())
	}
	if loaded.Endpoint != "api-us1.scanii.com" {
		t.Fatalf("expected Endpoint 'api-us1.scanii.com', got %q", loaded.Endpoint)
	}
	if loaded.CreatedAt.IsZero() {
		t.Fatalf("expected CreatedAt to be set")
	}
}

func TestLoadConfigNonExistent(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("USERPROFILE", tmpHome)

	_, err := Load("doesnotexist")
	if err == nil {
		t.Fatalf("expected error loading non-existent profile")
	}
}

func TestSaveConfigOverwrite(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("USERPROFILE", tmpHome)

	c1 := &Profile{
		Credentials: "key1:secret1",
		Endpoint:    "api-us1.scanii.com",
	}
	err := save("myprofile", c1)
	if err != nil {
		t.Fatalf("failed to save config: %s", err)
	}

	c2 := &Profile{
		Credentials: "key2:secret2",
		Endpoint:    "api-eu1.scanii.com",
	}
	err = save("myprofile", c2)
	if err != nil {
		t.Fatalf("failed to overwrite config: %s", err)
	}

	loaded, err := Load("myprofile")
	if err != nil {
		t.Fatalf("failed to load config: %s", err)
	}
	if loaded.Credentials != "key2:secret2" {
		t.Fatalf("expected Credentials 'key2:secret2', got %q", loaded.Credentials)
	}
	if loaded.Endpoint != "api-eu1.scanii.com" {
		t.Fatalf("expected Endpoint 'api-eu1.scanii.com', got %q", loaded.Endpoint)
	}
}

func TestMultipleProfiles(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("USERPROFILE", tmpHome)

	profiles := map[string]*Profile{
		"dev": {
			Credentials: "devkey:devsecret",
			Endpoint:    "localhost:4000",
		},
		"staging": {
			Credentials: "stagingkey:stagingsecret",
			Endpoint:    "api-eu1.scanii.com",
		},
		"prod": {
			Credentials: "prodkey:prodsecret",
			Endpoint:    "api-us1.scanii.com",
		},
	}

	for name, cfg := range profiles {
		if err := save(name, cfg); err != nil {
			t.Fatalf("failed to save profile %q: %s", name, err)
		}
	}

	for name, expected := range profiles {
		loaded, err := Load(name)
		if err != nil {
			t.Fatalf("failed to load profile %q: %s", name, err)
		}
		if loaded.Credentials != expected.Credentials {
			t.Fatalf("profile %q: expected Credentials %q, got %q", name, expected.Credentials, loaded.Credentials)
		}
		if loaded.Endpoint != expected.Endpoint {
			t.Fatalf("profile %q: expected Endpoint %q, got %q", name, expected.Endpoint, loaded.Endpoint)
		}
	}
}

func TestConfigDir(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("USERPROFILE", tmpHome)

	dir, err := ConfigDir()
	if err != nil {
		t.Fatalf("failed to get config dir: %s", err)
	}

	expected := filepath.Join(tmpHome, ".config", "scanii-cli")
	if dir != expected {
		t.Fatalf("expected config dir %q, got %q", expected, dir)
	}
}

func TestConfigPath(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("USERPROFILE", tmpHome)

	p, err := ConfigPath("myprofile")
	if err != nil {
		t.Fatalf("failed to get config path: %s", err)
	}

	expected := filepath.Join(tmpHome, ".config", "scanii-cli", "myprofile.json")
	if p != expected {
		t.Fatalf("expected config path %q, got %q", expected, p)
	}
}

func TestSaveConfigSetsCreatedAtTime(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("USERPROFILE", tmpHome)

	before := time.Now().Add(-time.Second)
	c := &Profile{
		Credentials: "k:s",
		Endpoint:    "api-us1.scanii.com",
	}
	err := save("timetest", c)
	if err != nil {
		t.Fatalf("failed to save: %s", err)
	}
	after := time.Now().Add(time.Second)

	loaded, err := Load("timetest")
	if err != nil {
		t.Fatalf("failed to load: %s", err)
	}

	if loaded.CreatedAt.Before(before) || loaded.CreatedAt.After(after) {
		t.Fatalf("CreatedAt time %v not in expected range [%v, %v]", loaded.CreatedAt, before, after)
	}
}

func TestConfigFilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix file permissions are not enforced on Windows")
	}
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("USERPROFILE", tmpHome)

	c := &Profile{
		Credentials: "k:s",
		Endpoint:    "api-us1.scanii.com",
	}
	err := save("permtest", c)
	if err != nil {
		t.Fatalf("failed to save: %s", err)
	}

	p, _ := ConfigPath("permtest")
	info, err := os.Stat(p)
	if err != nil {
		t.Fatalf("failed to stat: %s", err)
	}

	perm := info.Mode().Perm()
	if perm != 0600 {
		t.Fatalf("expected file permissions 0600, got %o", perm)
	}
}

func TestProfileDeleteCommand(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("USERPROFILE", tmpHome)

	c := &Profile{
		Credentials: "k:s",
		Endpoint:    "api-us1.scanii.com",
	}
	err := save("todelete", c)
	if err != nil {
		t.Fatalf("failed to save: %s", err)
	}

	// verify it exists
	_, err = Load("todelete")
	if err != nil {
		t.Fatalf("profile should exist: %s", err)
	}

	// delete via file removal (same as delete command logic)
	p, _ := ConfigPath("todelete")
	err = os.Remove(p)
	if err != nil {
		t.Fatalf("failed to delete: %s", err)
	}

	// verify it's gone
	_, err = Load("todelete")
	if err == nil {
		t.Fatalf("expected error after deletion")
	}
}

func TestDefaultProfileName(t *testing.T) {
	if DefaultProfileName != "default" {
		t.Fatalf("expected default profile name to be 'default', got %q", DefaultProfileName)
	}
}

func TestProfileCommandStructure(t *testing.T) {
	cmd := Command()

	if cmd.Use != "profile" {
		t.Fatalf("expected Use 'profile', got %q", cmd.Use)
	}

	subcommands := make(map[string]bool)
	for _, sub := range cmd.Commands() {
		subcommands[sub.Use] = true
	}

	expected := []string{"create [name]", "list [name]", "delete <name>"}
	for _, e := range expected {
		if !subcommands[e] {
			t.Fatalf("expected subcommand %q not found", e)
		}
	}
}

func TestJsonFormat(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("USERPROFILE", tmpHome)

	c := &Profile{
		Credentials: "sck_abc123:scks_secret456",
		Endpoint:    "api-us1.staging.scanii.com",
	}

	err := save("jsontest", c)
	if err != nil {
		t.Fatalf("failed to save: %s", err)
	}

	// read raw JSON and verify structure
	p, _ := ConfigPath("jsontest")
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("failed to read file: %s", err)
	}

	var parsed map[string]interface{}
	err = json.Unmarshal(raw, &parsed)
	if err != nil {
		t.Fatalf("failed to parse JSON: %s", err)
	}

	// verify expected keys exist
	expectedKeys := []string{"endpoint", "createdAt", "version", "credentials"}
	for _, key := range expectedKeys {
		if _, ok := parsed[key]; !ok {
			t.Fatalf("expected key %q in JSON output, got keys: %v", key, parsed)
		}
	}

	// verify no old keys exist
	oldKeys := []string{"apiKey", "apiSecret", "updated"}
	for _, key := range oldKeys {
		if _, ok := parsed[key]; ok {
			t.Fatalf("unexpected old key %q found in JSON output", key)
		}
	}

	// verify values
	if parsed["endpoint"] != "api-us1.staging.scanii.com" {
		t.Fatalf("expected endpoint 'api-us1.staging.scanii.com', got %v", parsed["endpoint"])
	}
	if parsed["credentials"] != "sck_abc123:scks_secret456" {
		t.Fatalf("expected credentials 'sck_abc123:scks_secret456', got %v", parsed["credentials"])
	}
	if parsed["version"] != nil {
		t.Fatalf("expected version to be null, got %v", parsed["version"])
	}
}

func TestClientSendsUserAgent(t *testing.T) {
	var got string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.UserAgent()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message":"pong","key":"testkey"}`))
	}))
	defer server.Close()

	// the localhost prefix is what makes Client() use http instead of https
	addr, ok := server.Listener.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("expected a tcp listener, got %T", server.Listener.Addr())
	}

	c := &Profile{
		Credentials: "testkey:testsecret",
		Endpoint:    fmt.Sprintf("localhost:%d", addr.Port),
	}

	apiClient, err := c.Client()
	if err != nil {
		t.Fatalf("failed to create client: %s", err)
	}

	if _, err := apiClient.Ping(context.Background()); err != nil {
		t.Fatalf("failed to ping: %s", err)
	}

	if got != buildinfo.UserAgent() {
		t.Fatalf("expected user agent %q, got %q", buildinfo.UserAgent(), got)
	}
	// regression guard for https://github.com/scanii/scanii-cli/issues/85
	if strings.TrimPrefix(strings.SplitN(got, " ", 2)[0], "scanii-cli/") == "" {
		t.Fatalf("expected a non-empty version in the user agent, got %q", got)
	}
}

func TestApiKeyAndSecretParsing(t *testing.T) {
	c := &Profile{
		Credentials: "mykey:mysecret",
	}
	if c.APIKey() != "mykey" {
		t.Fatalf("expected apiKey 'mykey', got %q", c.APIKey())
	}
	if c.APISecret() != "mysecret" {
		t.Fatalf("expected apiSecret 'mysecret', got %q", c.APISecret())
	}

	// credentials with no colon
	c2 := &Profile{
		Credentials: "onlykey",
	}
	if c2.APIKey() != "onlykey" {
		t.Fatalf("expected apiKey 'onlykey', got %q", c2.APIKey())
	}
	if c2.APISecret() != "" {
		t.Fatalf("expected apiSecret '', got %q", c2.APISecret())
	}

	// credentials with multiple colons (secret contains colon)
	c3 := &Profile{
		Credentials: "key:secret:with:colons",
	}
	if c3.APIKey() != "key" {
		t.Fatalf("expected apiKey 'key', got %q", c3.APIKey())
	}
	if c3.APISecret() != "secret:with:colons" {
		t.Fatalf("expected apiSecret 'secret:with:colons', got %q", c3.APISecret())
	}
}

func TestWithMaxConcurrency(t *testing.T) {
	tests := []struct {
		name string
		n    int
		want int
	}{
		{"sizes the pool to the limit", 64, 64},
		{"a single request still pools one", 1, 1},
		{"an unset limit keeps the default", 0, http.DefaultMaxIdleConnsPerHost},
		{"so does a nonsense one", -1, http.DefaultMaxIdleConnsPerHost},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := clientConfig{maxIdleConns: http.DefaultMaxIdleConnsPerHost}
			WithMaxConcurrency(tt.n)(&cfg)
			if cfg.maxIdleConns != tt.want {
				t.Fatalf("expected %d idle connections, got %d", tt.want, cfg.maxIdleConns)
			}
		})
	}
}
