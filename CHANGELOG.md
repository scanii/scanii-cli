# Changelog

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
