# Changelog

All notable changes to `scanii-cli` are documented here. Versions follow [SemVer](https://semver.org).

## v1.5.0 — 2026-05-02

### Added

- `sc files trace <id>` command — wraps the `GET /v2.2/files/{id}/trace` endpoint and prints the events as a `timestamp / message` table.
- `Client.RetrieveTrace(ctx, id)` in `internal/client`.

## v1.4.0 — 2026-05-01

### Added

- `GET /v2.2/files/{id}/trace` mock-server endpoint.
- `location` field support for `POST /v2.2/files`.

## v1.3.1 — 2026-04-30

### Added

- `/healthcheck` route used by the Docker container health check.

### Changed

- Improved terminal output formatting and warning labels.

## v1.3.0 — 2026-04-24

### Changed

- Docker image improvements.

## v1.2.0 — 2026-04-24

### Removed

- Dependabot config.

### Changed

- Docker image namespace updated in the Goreleaser config.

## v1.1.1 — 2026-04-24

### Changed

- Expanded README with usage examples and a CI guide.

## v1.1.0 — 2026-04-24

### Added

- Callback delivery support in the local server.
- Embedded test asset fixtures.

### Fixed

- Panic when `/tmp` does not exist inside the Docker container.

## v1.0.0 — 2026-04-23

First stable release published under the `scanii/scanii-cli` repo.

### Added

- Multiple profile support — named profiles via `sc profile create [name]` and `-p, --profile` global flag.

## v0.1.x and earlier

Pre-1.0 releases lived under `uvasoftware/scanii-cli`. See the [GitHub releases page](https://github.com/scanii/scanii-cli/releases?q=v0.) for details.
