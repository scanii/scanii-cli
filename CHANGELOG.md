# Changelog

## [1.11.1]

### Added

- `DELETE /v2.2/files/{id}` support in the mock server and client. Hard-deletes a previously processed file result and its trace, returning `204 No Content` on success.
- `sc files delete <id>` — delete a previously created processing result and its trace through the CLI.

## [1.10.0]

### Added

- `--perf` on every `sc files` command — `process`, `async`, `fetch`, `retrieve` and `trace`. Prints a timing breakdown of the API requests the command made — DNS, TCP connect, TLS handshake, request transfer, server processing, response transfer, total, and the run's own overhead — so that a slow network can be told apart from a slow scan. A command that makes more than one request — a directory scan, or a `--wait` poll — reports the mean, plus how many of those requests reused a pooled connection. Phases that did not happen read `n/a` rather than `0 s`, as does a phase that finished inside the clock's resolution.
- The `X-Scanii-Request-Id` of every file request is logged at debug level, whether or not the request succeeded. It is the id support needs to look a scan up on the server side; `--perf` prints it too, and a failed request already quoted it in its error.
- `client.Timings`, captured with `net/http/httptrace` on every request and carried on `client.Response`, along with `Response.RequestID()` and the `client.RequestIDHeader` constant.
- `profile.WithMaxConcurrency`, which sizes the connection pool of the client a profile builds. Commands that make a single request are unaffected and keep the transport's default.

### Fixed

- The connection pool is now sized to `--concurrency` rather than left at Go's default of two idle connections per host. A run with more requests in flight than that was closing connections as fast as it opened them and paying a TCP connect — and against a real endpoint a TLS handshake — for files that should have reused one. Over 200 files at `--concurrency 64`, connections opened dropped from 118 to 45. None of that time appears in the API's own timings, which is what made it worth finding.

### Changed

- The default `--concurrency` for `sc files process` and `sc files async` is now a flat 32, rather than `32 × NumCPU` — 320 requests in flight on a ten-core machine. Scanning is network-bound, so the figure belongs to the link and not to the machine: measured against a 60ms endpoint, wall clock stops improving past 16 on a 20Mbit/s link, and past 16 for megabyte files on a 100Mbit/s one. Only a fast link full of small files still gains beyond 32, and there 32 lands within a fifth of the best time while opening a sixth of the connections. Raise it with `--concurrency` on a link that can take it.
- The console log handler only emits ANSI styling when the destination is a terminal, so `sc -v ... > log.txt` writes plain text rather than escape codes. Its fixed-width source column is likewise printed only when source reporting is on, instead of padding to fifty blanks.
- `Client.do` returns a `*Response` and the body rather than a status, headers and body triple, so that the timings ride along with the rest of the response metadata.

## [1.7.1]

### Fixed

- Republish of the 1.7.0 release. The 1.7.0 tag pushed an empty release because `goreleaser-action@v5` with `version: latest` locks to goreleaser v1.x, which rejected the v2 config schema introduced in 1.7.0. Bumped the action to `@v6` pinned to `~> v2`. No source or behavior changes; same install paths as 1.7.0.

## [1.7.0]

### Added

- Homebrew install path. Goreleaser now publishes the formula to `scanii/homebrew-tap` on every release: `brew install scanii/tap/scanii-cli`.
- Shell installer at `install.sh`. POSIX-compatible script that detects OS/arch, fetches the matching release archive, verifies the SHA-256, and installs `sc` to `~/.local/bin` (override via `SCANII_CLI_BIN_DIR`, pin a version via `SCANII_CLI_VERSION`): `curl -fsSL https://raw.githubusercontent.com/scanii/scanii-cli/main/install.sh | sh`.

### Changed

- `.goreleaser.yaml` now pins the build matrix explicitly to `amd64`+`arm64` across all OSes, so the shell installer can rely on a stable archive-name contract. The `386` archives (linux-386, windows-386) are no longer produced — no GitHub-hosted runner uses them and they had no known consumers.

## [1.6.0]

### Added

- CORS support on the mock server, matching `api.scanii.com` in production (`Allow-Origin: *`, `Allow-Methods: GET, POST, HEAD, OPTIONS, DELETE`, `Allow-Headers: Authorization, User-Agent`, `Max-Age: 300`). OPTIONS preflight short-circuits with 200, no credentials required. Lets browser-based clients call the mock from a different origin.

## [1.5.0]

### Added

- `sc files trace <id>` command — wraps the `GET /v2.2/files/{id}/trace` endpoint and prints the events as a `timestamp / message` table.
- `Client.RetrieveTrace(ctx, id)` in `internal/client`.

## [1.4.0]

### Added

- `GET /v2.2/files/{id}/trace` mock-server endpoint.
- `location` field support for `POST /v2.2/files`.

## [1.3.1]

### Added

- `/healthcheck` route used by the Docker container health check.

### Changed

- Improved terminal output formatting and warning labels.

## [1.3.0]

### Changed

- Docker image improvements.

## [1.2.0]

### Removed

- Dependabot config.

### Changed

- Docker image namespace updated in the Goreleaser config.

## [1.1.1]

### Changed

- Expanded README with usage examples and a CI guide.

## [1.1.0]

### Added

- Callback delivery support in the local server.
- Embedded test asset fixtures.

### Fixed

- Panic when `/tmp` does not exist inside the Docker container.

## [1.0.0]

First stable release published under the `scanii/scanii-cli` repo.

### Added

- Multiple profile support — named profiles via `sc profile create [name]` and `-p, --profile` global flag.

## [0.1.x and earlier]

Pre-1.0 releases lived under `uvasoftware/scanii-cli`. See the [GitHub releases page](https://github.com/scanii/scanii-cli/releases?q=v0.) for details.
