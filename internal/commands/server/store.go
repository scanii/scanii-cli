package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"reflect"
	"regexp"
)

// kind namespaces the records the server keeps, so that an id handed out for
// one kind of record can never be used to reach another: a processing result
// id is not a token, and a token id does not retrieve a result.
type kind string

const (
	kindResult kind = ""
	kindTrace  kind = "trace-"
	kindToken  kind = "token-"
)

// idPattern matches the ids generateID hands out: a UUID with its dashes
// removed. Anything else, including ids carrying a kind prefix or a path
// separator, is treated as not found before the filesystem is touched.
var idPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

// errInvalidID is returned by save for an id generateID could not have made.
// load reports such ids as fs.ErrNotExist instead, which is what they are.
var errInvalidID = errors.New("invalid id")

// store keeps records as JSON files in a single directory. Every access goes
// through an os.Root, so no name can resolve outside that directory, and names
// are built only from a kind and a validated id.
type store struct {
	root *os.Root
}

// newStore opens dir as the store's root. The root stays open for the life of
// the server.
func newStore(dir string) (store, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return store{}, fmt.Errorf("opening storage directory: %w", err)
	}
	return store{root: root}, nil
}

func validID(id string) bool {
	return idPattern.MatchString(id)
}

func fileName(k kind, id string) string {
	return string(k) + id + ".json"
}

func (s store) load(k kind, id string, v any) error {
	// same rules as json.unmarshal
	if v == nil {
		return fmt.Errorf("v cannot be nil")
	}
	if reflect.ValueOf(v).Kind() != reflect.Ptr {
		return fmt.Errorf("v must be a pointer")
	}
	if !validID(id) {
		return os.ErrNotExist
	}

	name := fileName(k, id)
	slog.Debug("loading record", "name", name)
	js, err := s.root.ReadFile(name)
	if err != nil {
		return err
	}

	return json.Unmarshal(js, v)
}

func (s store) save(k kind, id string, v any) error {
	if !validID(id) {
		return errInvalidID
	}

	js, err := json.Marshal(v)
	if err != nil {
		return err
	}

	name := fileName(k, id)
	if err := s.root.WriteFile(name, js, 0o600); err != nil {
		return err
	}

	slog.Info("saved record", "name", name)
	return nil
}

// remove deletes a record, reporting whether it existed. An invalid id is
// reported as not found.
func (s store) remove(k kind, id string) (bool, error) {
	if !validID(id) {
		return false, nil
	}

	if err := s.root.Remove(fileName(k, id)); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
