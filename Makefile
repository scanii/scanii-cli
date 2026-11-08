# https://www.alexedwards.net/blog/a-time-saving-makefile-for-your-go-projects
# Change these variables as necessary.
MAIN_PACKAGE_PATH := ./cmd/sc
BINARY_NAME := sc

# ==================================================================================== #
# HELPERS
# ==================================================================================== #

## help: print this help message
.PHONY: help
help:
	@echo 'Usage:'
	@sed -n 's/^##//p' ${MAKEFILE_LIST} | column -t -s ':' |  sed -e 's/^/ /'

.PHONY: confirm
confirm:
	@echo -n 'Are you sure? [y/N] ' && read ans && [ $${ans:-N} = y ]

.PHONY: no-dirty
no-dirty:
	git diff --exit-code


# ==================================================================================== #
# QUALITY CONTROL
# ==================================================================================== #

## tidy: format code and tidy the module and the tools module
.PHONY: tidy
tidy:
	go fmt ./...
	go mod tidy -v
	go -C tools mod tidy

# Analysis tools are pinned in tools/go.mod (tool directives, checksummed in tools/go.sum) and run
# with go tool, so nothing is installed or fetched @latest. tools/go.mod needs Go 1.27, so lint does
# too. gosec is pinned next to golang.org/x/tools v0.52.0: the x/tools its own release pins cannot
# read Go 1.27 export data and silently analyzes nothing. Never set GOOS or GOARCH for go tool: it
# would cross-compile the tool itself.
GOTOOL = tool -modfile=$(CURDIR)/tools/go.mod

# e2e/ is behind the e2e build tag; the analyzers read it too.
TAGS = -tags=e2e

# gosec rule classes that are false positives here: G401/G505 (SHA-1 is the checksum the Scanii API
# reports and a rule format of the local server's engine, never a secret). Individually accepted
# sites carry `// #nosec Gxxx -- reason`.
GOSEC_EXCLUDE = G401,G505

# The dependency license allowlist as SPDX identifiers, the one jackfruit, noni and breadfruit share:
# permissive licenses plus the file-scoped weak copyleft CDDL and MPL. GPL, AGPL, LGPL and EPL are out
# by policy (Rafael, 2026-11-08); the go-licenses v1 check this replaces (--disallowed_types
# restricted) let AGPL, EPL and unidentifiable licenses through. A dependency, test-only ones included,
# with an unlisted or unidentifiable license fails lint: review it, then extend the list. Every
# license a LICENSE file declares must be listed, so an "MIT or GPL" dependency fails too. This
# repository publishes its own LICENSE (Apache-2.0), so nothing is ignored. The klog flags drop
# go-licenses' warnings about assembly files.
LICENSES_ALLOWED = 0BSD,Apache-2.0,BSD-2-Clause,BSD-3-Clause,CC0-1.0,MIT,MIT-0,CDDL-1.0,MPL-1.1,MPL-2.0
LICENSE_CHECK = go-licenses check ./... --allowed_licenses=$(LICENSES_ALLOWED) --include_tests --logtostderr=false --stderrthreshold=ERROR --log_file=/dev/null

## lint: gofmt, go vet, staticcheck, govulncheck, gosec and the license allowlist, what CI's lint check runs
.PHONY: lint
lint:
	@unformatted=$$(gofmt -l .); if [ -n "$$unformatted" ]; then echo "not gofmt-formatted (run make tidy):"; echo "$$unformatted"; exit 1; fi
	go vet $(TAGS) ./...
	go $(GOTOOL) staticcheck $(TAGS) ./...
	go $(GOTOOL) govulncheck $(TAGS) ./...
	go $(GOTOOL) gosec -quiet $(TAGS) -exclude=$(GOSEC_EXCLUDE) ./...
	go $(GOTOOL) $(LICENSE_CHECK)

# tools/go.mod needs Go 1.27 while CI's test check runs the Go 1.26 the release builds with, so
# test/ci lets go fetch the 1.27 toolchain for the tools module's tidy check.
test/ci: export GOTOOLCHAIN := auto

## test/ci: module hygiene (both modules) and the race suite, what CI's test check runs on Linux, macOS and Windows
.PHONY: test/ci
test/ci:
	go mod tidy -diff
	go -C tools mod tidy -diff
	go mod verify
	go test -race -vet=off ./...

## test/e2e: build sc, start sc server on SC_E2E_PORT (default 4101) and run real sc commands against it
.PHONY: test/e2e
test/e2e:
	go test $(TAGS) -count=1 -v -timeout 5m ./e2e

## audit: lint plus module hygiene and the race suite
.PHONY: audit
audit: lint test/ci


# ==================================================================================== #
# DEVELOPMENT
# ==================================================================================== #

## test: run the unit tests verbosely (make test/e2e runs the end-to-end suite)
.PHONY: test
test:
	go test -v -race -buildvcs ./...

## test/cover: run all tests and display coverage
.PHONY: test/cover
test/cover:
	go test -v -race -buildvcs -coverprofile=/tmp/coverage.out ./...
	go tool cover -html=/tmp/coverage.out

## build: build the application
.PHONY: build
build:
	rm -rf dist/*
	go build -o=./dist/${BINARY_NAME} ${MAIN_PACKAGE_PATH}

## build/release: build the application in release mode stripping debug information
.PHONY: build/release
build/release:
	rm -rf dist/*
	go build -ldflags="-s -w" -o=./dist/${BINARY_NAME} ${MAIN_PACKAGE_PATH}

## run: run the  application
.PHONY: run
run:
	go run ${MAIN_PACKAGE_PATH}


# ==================================================================================== #
# OPERATIONS
# ==================================================================================== #

## release-local: creates a new LOCAL release
.PHONY: release-local
release-local: tidy audit no-dirty
	go run github.com/goreleaser/goreleaser@latest

## push: push changes to the remote Git repository
.PHONY: push
push: tidy audit no-dirty
	git push

