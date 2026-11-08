// Package e2e holds the end-to-end suite: it builds sc, starts sc server in the
// background and drives real sc commands against it, asserting on their output
// and exit codes. The tests sit behind the e2e build tag so that go test ./...
// stays a unit-test run; make test/e2e runs them.
//
// SC_E2E_PORT picks the server's port (default 4101, away from the 4000 a
// developer's own sc server is likely to hold).
package e2e
