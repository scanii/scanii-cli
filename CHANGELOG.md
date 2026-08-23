# Changelog

## [1.8.0]

### Added

- `--perf` on `sc files process`. Prints a timing breakdown of the API requests a scan made — DNS, TCP connect, TLS handshake, request transfer, server processing, response transfer, total, and the run's own overhead — so that a slow network can be told apart from a slow scan. A directory scan makes one request per file and reports the mean, plus how many of those requests reused a pooled connection. Phases that did not happen read `n/a` rather than `0 s`, as does a phase that finished inside the clock's resolution.
- The `X-Scanii-Request-Id` of every file request is logged at info level, whether or not `--perf` was passed. It is the id support needs to look a scan up on the server side.
- `client.Timings`, captured with `net/http/httptrace` on every request and carried on `client.Response`, along with `Response.RequestID()` and the `client.RequestIDHeader` constant.

### Changed

- The console log handler is now installed on every run at info level, rather than only under `--verbose`, so that info-level output is styled like the rest of the CLI instead of falling through to slog's default handler. `--verbose` still selects debug level.
- The log handler only emits ANSI styling when the destination is a terminal, so a piped or redirected run writes plain text. The fixed-width source column is likewise printed only when source reporting is on, instead of padding to fifty blanks.
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
