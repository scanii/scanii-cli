package server

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func newTestStore(t *testing.T, dir string) store {
	t.Helper()
	s, err := newStore(dir)
	if err != nil {
		t.Fatalf("newStore: %s", err)
	}
	t.Cleanup(func() { _ = s.root.Close() })
	return s
}

func TestStoreSaveAndLoad(t *testing.T) {
	s := newTestStore(t, t.TempDir())

	type payload struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}

	id := generateID()
	err := s.save(kindResult, id, payload{Name: "hello", Value: 42})
	if err != nil {
		t.Fatalf("save failed: %s", err)
	}

	var loaded payload
	err = s.load(kindResult, id, &loaded)
	if err != nil {
		t.Fatalf("load failed: %s", err)
	}

	if loaded.Name != "hello" || loaded.Value != 42 {
		t.Fatalf("loaded data mismatch: %+v", loaded)
	}
}

func TestStoreKindsAreSeparate(t *testing.T) {
	s := newTestStore(t, t.TempDir())

	id := generateID()
	if err := s.save(kindResult, id, map[string]string{"a": "b"}); err != nil {
		t.Fatalf("save failed: %s", err)
	}

	var v map[string]string
	for _, k := range []kind{kindTrace, kindToken} {
		if err := s.load(k, id, &v); !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("load %q: want not exist, got %v", k, err)
		}
	}
}

func TestStoreLoadNonExistent(t *testing.T) {
	s := newTestStore(t, t.TempDir())

	var v map[string]string
	err := s.load(kindResult, generateID(), &v)
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("expected not exist for a missing key, got %v", err)
	}
}

func TestStoreLoadNilTarget(t *testing.T) {
	s := newTestStore(t, t.TempDir())

	err := s.load(kindResult, generateID(), nil)
	if err == nil {
		t.Fatal("expected error for nil target")
	}
}

func TestStoreLoadNonPointer(t *testing.T) {
	s := newTestStore(t, t.TempDir())

	err := s.load(kindResult, generateID(), "not a pointer")
	if err == nil {
		t.Fatal("expected error for non-pointer target")
	}
}

func TestStoreRemove(t *testing.T) {
	s := newTestStore(t, t.TempDir())

	id := generateID()
	err := s.save(kindResult, id, map[string]string{"a": "b"})
	if err != nil {
		t.Fatalf("save failed: %s", err)
	}

	removed, err := s.remove(kindResult, id)
	if err != nil {
		t.Fatalf("remove failed: %s", err)
	}
	if !removed {
		t.Fatal("expected removed=true")
	}

	// second remove should return false
	removed, err = s.remove(kindResult, id)
	if err != nil {
		t.Fatalf("remove failed: %s", err)
	}
	if removed {
		t.Fatal("expected removed=false for already-removed key")
	}
}

func TestStoreRemoveNonExistent(t *testing.T) {
	s := newTestStore(t, t.TempDir())

	removed, err := s.remove(kindResult, generateID())
	if err != nil {
		t.Fatalf("remove failed: %s", err)
	}
	if removed {
		t.Fatal("expected removed=false for non-existent key")
	}
}

// invalidIDs are names generateID cannot produce. Each must be refused before
// the filesystem is touched.
var invalidIDs = []string{
	"",
	"test1",
	"../outside",
	"../../outside",
	"sub/" + "0123456789abcdef0123456789abcdef",
	"..\\outside",
	"/etc/passwd",
	"token-0123456789abcdef0123456789abcdef",
	"trace-0123456789abcdef0123456789abcdef",
	"0123456789ABCDEF0123456789ABCDEF",
	"0123456789abcdef0123456789abcde",
	"0123456789abcdef0123456789abcdef0",
	"0123456789abcdef0123456789abcdef\n",
}

func TestStoreRejectsInvalidIDs(t *testing.T) {
	parent := t.TempDir()
	data := filepath.Join(parent, "data")
	if err := os.Mkdir(data, 0o700); err != nil {
		t.Fatal(err)
	}
	// a file outside the data directory that a traversal would reach
	outside := filepath.Join(parent, "outside.json")
	if err := os.WriteFile(outside, []byte(`{"a":"b"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	s := newTestStore(t, data)
	for _, id := range invalidIDs {
		var v map[string]string
		if err := s.load(kindResult, id, &v); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("load(%q): want not exist, got %v", id, err)
		}
		removed, err := s.remove(kindResult, id)
		if err != nil || removed {
			t.Errorf("remove(%q): want (false, nil), got (%v, %v)", id, removed, err)
		}
		if err := s.save(kindResult, id, v); !errors.Is(err, errInvalidID) {
			t.Errorf("save(%q): want errInvalidID, got %v", id, err)
		}
	}

	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("file outside the data directory was touched: %v", err)
	}
	entries, err := os.ReadDir(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("data directory: want empty, got %d entries", len(entries))
	}
}

// TestStoreStaysInsideRoot covers the Root itself: a valid name that is a
// symlink pointing outside the data directory is not followed.
func TestStoreStaysInsideRoot(t *testing.T) {
	parent := t.TempDir()
	data := filepath.Join(parent, "data")
	if err := os.Mkdir(data, 0o700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(parent, "outside.json")
	if err := os.WriteFile(outside, []byte(`{"a":"b"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	id := generateID()
	if err := os.Symlink(outside, filepath.Join(data, fileName(kindResult, id))); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	s := newTestStore(t, data)
	var v map[string]string
	if err := s.load(kindResult, id, &v); err == nil {
		t.Fatalf("load followed a symlink out of the data directory: %v", v)
	}
	if err := s.save(kindResult, id, map[string]string{"x": "y"}); err == nil {
		t.Fatal("save wrote through a symlink out of the data directory")
	}
	got, err := os.ReadFile(outside)
	if err != nil || string(got) != `{"a":"b"}` {
		t.Fatalf("file outside the data directory changed: %q, %v", got, err)
	}
}
