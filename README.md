# Scanii CLI

The Scanii CLI (`sc`) helps you build, test, and manage your [Scanii](https://www.scanii.com) integration right from the terminal.

**With the CLI, you can:**

- Interact with the Scanii API: scan files, manage auth tokens, and check account info
- Start a local server that simulates the Scanii API for integration testing
- Process single files or entire directories with concurrent workers and byte-level upload progress

## Installation

### Homebrew (macOS, Linux)

```shell
brew trust --formula scanii/tap/scanii-cli
brew install scanii/tap/scanii-cli
```

From Homebrew 6.0, formulae in third-party taps must be trusted before Homebrew
will load them, so without the first command the install fails with `Refusing to
load formula scanii/tap/scanii-cli from untrusted tap scanii/tap`. Trust is
recorded locally in `~/.homebrew/trust.json` — it is your decision to make, and
nobody can grant it on your behalf. Use `brew trust scanii/tap` instead to trust
everything in the tap rather than just this formula.

`brew trust` prompts for nothing, so it drops straight into a CI script. It has
existed since Homebrew 5.1.15; on anything older, skip it and just install.

Trust persists, so upgrades need nothing extra:

```shell
brew upgrade scanii-cli
```

### Shell installer (macOS, Linux)

```shell
curl -fsSL https://raw.githubusercontent.com/scanii/scanii-cli/main/install.sh | sh
```

The installer downloads the matching release archive from GitHub, verifies it against `checksums.txt`, and installs `sc` to `~/.local/bin` (override with `SCANII_CLI_BIN_DIR`). Pin a specific version with `SCANII_CLI_VERSION=1.6.0`.

### Docker

A container image is published to the GitHub Container Registry:

```shell
docker run ghcr.io/scanii/scanii-cli:latest
```

Previous versions are listed [here](https://github.com/scanii/scanii-cli/pkgs/container/scanii-cli).

### Binary releases

Pre-built binaries for macOS, Windows, and Linux are available on the [releases page](https://github.com/scanii/scanii-cli/releases).

On macOS, you may need to remove the quarantine attribute before running:

```shell
xattr -d com.apple.quarantine /path/to/sc
```

## Quick start

### 1. Configure a profile

Set up your API credentials and endpoint:

```shell
sc profile create --endpoint api-us1.scanii.com --credentials YOUR_KEY:YOUR_SECRET
```

This creates a `default` profile stored in `~/.config/scanii-cli/`. You can create named profiles for different environments:

```shell
sc profile create staging --endpoint localhost:4000 --credentials key:secret
```

List configured profiles:

```shell
sc profile list
```

### 2. Test connectivity

```shell
sc ping
```

Use a non-default profile with the `-p` flag:

```shell
sc -p staging ping
```

### 3. Scan a file

Synchronous scan (blocks until the result is ready):

```shell
sc files process /path/to/file.pdf
```

Progress is measured in bytes as they go out on the wire, so a large file
advances smoothly while it uploads:

```
Using endpoint: api-us1.scanii.com and API key: mykey
Processing file /path/to/backup.tar
Uploading backup.tar  [█████████████████░░░░░░░░░░░░░]  58% (183.2 MB/314.6 MB)
```

Once the last byte is sent, the wait is on the server, so the bar gives way to a
spinner until the result lands:

```
⠹ Analyzing backup.tar
```

```
# /path/to/backup.tar:

  id:             ff03467da11f417aa99845c91793ce0c
  checksum/sha1:  0c4f4f069728d13bf481738ac926381477fb8975
  content type:   application/octet-stream
  content length:  314.6 MB
  creation date:   Sat, 08 Aug 2026 08:41:43 EDT
  findings:       none
  metadata:       none

✔ Completed in 4.2 s, 1 file(s) analyzed. Throughput 74.9 MB/s
✔ Files with findings: 0, unable to process: 0 and successfully processed: 1
```

Every request records the `X-Scanii-Request-Id` the API returned — the id
support needs to look a specific scan up on the server side. A request that
failed quotes it in the error; otherwise `--perf` prints it, and `-v` logs it
for every request:

```
2026-08-08 08:41:43.508 DEBUG 24068 internal/commands/file/common.go:38                : processed file path=/path/to/backup.tar status=201 request_id=req_9f3a1c
```

Asynchronous scan (returns immediately with a pending result ID):

```shell
sc files async /path/to/file.pdf
```

Retrieve the result of an async scan:

```shell
sc files retrieve RESULT_ID
```

Retrieve the processing trace for a result:

```shell
sc files trace RESULT_ID
```

### 4. Scan a remote URL

Submit a URL for server-side fetch and scan:

```shell
sc files fetch https://example.com/document.pdf
```

Wait for the result instead of returning immediately:

```shell
sc files fetch --wait 30 https://example.com/document.pdf
```

### 5. Scan an entire directory

Process all files in a directory with concurrent workers:

```shell
sc files process /path/to/directory
```

Skip hidden files and attach metadata:

```shell
sc files process --ignore-hidden --metadata env=production,scan_type=nightly /path/to/directory
```

Empty files are skipped rather than uploaded — the API rejects empty content, so
sending it only buys a `400` — and the CLI reports how many it passed over:

```
Processing recursive directory /path/to/directory with ~12 files | ~50.3 MB
Skipping 3 empty file(s)
```

Directories are tracked the same way. The bar fills with the bytes uploaded so
far across every file, and the label counts the files that have come back:

```
Using endpoint: api-us1.scanii.com and API key: mykey
Processing recursive directory /path/to/directory with ~12 files | ~50.3 MB
Files 7/12  [████████████████████████░░░░░░░░░░░░░░░░░░]  58% (29.7 MB/50.3 MB)
```

On completion, any file that came back with findings is listed, so a directory
scan tells you *where* the malware was and not just how much of it there was:

```
Using endpoint: api-us1.scanii.com and API key: mykey
Processing recursive directory /path/to/directory with ~12 files | ~50.3 MB
Files 12/12  [████████████████████████████████████████]  100% (50.3 MB/50.3 MB)

## Files with findings

# /path/to/directory/quarantine/sample.txt:

  id:             e353d97476fe40c6abd20418efe82b96
  checksum/sha1:  7da9d3b0c68b1d0543acb65af4220a4745607557
  content type:   text/plain; charset=utf-8
  content length:  36 B
  creation date:   Sat, 08 Aug 2026 09:07:46 EDT
  findings:       content.malicious.eicar-test-signature
  metadata:       none

✔ Completed in 3.1 s, 12 file(s) analyzed. Throughput 16.2 MB/s
✔ Files with findings: 1, unable to process: 0 and successfully processed: 12
```

Files the API rejected are reported as they happen, with the reason the API gave
and the request id to quote in a support ticket. When more than a handful fail,
the ones that scrolled past are listed again at the end of the run:

```
error: /path/to/directory/huge.iso — status 413: File is too large (request id req_9f3a1c)

✔ Completed in 3.1 s, 11 file(s) analyzed. Throughput 16.2 MB/s
warning: Files with findings: 1, unable to process: 1 and successfully processed: 11
error: 1 of 12 file(s) could not be processed
```

`sc files process` and `sc files async` exit non-zero if any file could not be
processed, so a scan that half-failed does not pass for a clean one in CI.

### 6. Measure where the time went

`--perf` prints a breakdown of the API requests a command made, which is how to
tell a slow network apart from a slow scan. It works on every `sc files`
command:

```shell
sc files process --perf /path/to/backup.tar
sc files retrieve --perf RESULT_ID
sc files fetch --perf --wait 30 https://example.com/document.pdf
```

```
## Performance
  request id:          req_9f3a1c
  dns:                 24 ms
  tcp connect:         31 ms
  tls handshake:       58 ms
  request transfer:    3.4 s
  server processing:   612 ms
  response transfer:   1.2 ms
  total:               4.1 s
  client overhead:     104 ms
  connection:          new
```

The phases run in that order and, give or take the wait for a free connection,
add up to `total`:

| Phase | What it covers |
|-------|----------------|
| `dns` | Resolving the endpoint's host name |
| `tcp connect` | Opening the socket |
| `tls handshake` | Negotiating TLS |
| `request transfer` | Sending the request — for a scan, the upload |
| `server processing` | The API's own work, from the last request byte to the first response byte |
| `response transfer` | Reading the response body |
| `total` | The whole exchange |
| `client overhead` | The rest of the run — reading and hashing the file, building the request, printing the result |

A phase that did not happen reads `n/a`: a pooled connection resolves no name
and shakes no hands, and a plaintext endpoint never reaches the TLS phase. So
does a phase that finished inside the clock's resolution, which on Windows is a
millisecond — not a distinction that matters for the latencies this is for.

A command that makes more than one request reports the mean instead, and counts
how many of those requests rode on a connection that was already open. A
directory scan sends one request per file; `retrieve --wait` and `fetch --wait`
each poll until the result lands:

```
## Performance (mean of 128 requests)
  dns:                 1 ms
  tcp connect:         2 ms
  tls handshake:       12 ms
  request transfer:    184 ms
  server processing:   397 ms
  response transfer:   1.1 ms
  total:               597 ms
  connections:         96 of 128 reused
```

`connections` is worth watching on a directory scan. The pool is sized to
`--concurrency`, so once the first wave of files has opened its connections the
rest of the run should reuse them; a low reuse count means the run is paying a
connect and a TLS handshake per file, and none of that shows up in the API's own
timings.

### 7. Manage auth tokens

Create a short-lived auth token (default timeout: 300 seconds):

```shell
sc auth-token create --timeout 600
```

Retrieve or revoke a token:

```shell
sc auth-token retrieve TOKEN_ID
sc auth-token delete TOKEN_ID
```

## Local server

The local server is the primary way to integration-test code that talks to the Scanii API. It implements the full v2.2 API surface including file processing, auth tokens, callbacks, and fetch-by-URL -- all without requiring real credentials or network access to Scanii servers.

### Starting the server

```shell
sc server
```

Output:

```
Scanii local server starting
API Key:       key
API Secret:    secret
Engine Rules:  5
Callback Wait: 100ms
Address:       http://localhost:4000

Sample usage: curl -u key:secret http://localhost:4000/v2.2/ping

We also provide fake sample files you can use to trigger findings:
  content.image.nsfw.nudity:         http://localhost:4000/static/samples/image.jpg
  content.en.language.nsfw.0:        http://localhost:4000/static/samples/language.txt
  content.malicious.local-test-file: http://localhost:4000/static/samples/malware
```

Default credentials are `key` / `secret`. Override them with flags:

```shell
sc server --key my-key --secret my-secret --address 0.0.0.0:8080
```

### Server options

| Flag | Default | Description |
|------|---------|-------------|
| `-a, --address` | `localhost:4000` | Listen address |
| `-k, --key` | `key` | API key |
| `-s, --secret` | `secret` | API secret |
| `-e, --engine` | built-in | Path to a custom engine rules JSON file |
| `-d, --data` | temp dir | Directory for storing processing results |
| `-w, --callback-wait` | `100ms` | Delay before firing callbacks; overrides `callback_wait` in the engine config |

### API endpoints

All endpoints are under the `/v2.2/` prefix and require HTTP Basic Auth:

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/v2.2/ping` | Health check |
| `GET` | `/v2.2/account.json` | Account info (returns mock data) |
| `POST` | `/v2.2/files` | Synchronous file scan |
| `POST` | `/v2.2/files/async` | Async file scan (returns pending ID) |
| `POST` | `/v2.2/files/fetch` | Fetch remote URL and scan |
| `GET` | `/v2.2/files/{id}` | Retrieve scan result |
| `GET` | `/v2.2/files/{id}/trace` | Retrieve processing trace |
| `POST` | `/v2.2/auth/tokens` | Create auth token |
| `GET` | `/v2.2/auth/tokens/{id}` | Retrieve auth token |
| `DELETE` | `/v2.2/auth/tokens/{id}` | Delete auth token |

Static sample files are served without authentication under `/static/`.

### curl examples

**Ping:**

```shell
curl -u key:secret http://localhost:4000/v2.2/ping
```

```json
{"key":"key","message":"pong"}
```

**Synchronous file scan:**

```shell
curl -u key:secret -F "file=@test.pdf" http://localhost:4000/v2.2/files
```

```json
{
  "id": "fd33128a8da445d3b8308fe6d1588829",
  "checksum": "da39a3ee5e6b4b0d3255bfef95601890afd80709",
  "content_length": 1024,
  "content_type": "application/pdf",
  "findings": [],
  "metadata": {},
  "creation_date": "2024-02-08T13:38:02.074502Z"
}
```

**Scan with metadata and callback:**

```shell
curl -u key:secret \
  -F "file=@test.pdf" \
  -F "metadata[env]=staging" \
  -F "metadata[ticket]=JIRA-123" \
  -F "callback=https://your-app.example.com/webhook" \
  http://localhost:4000/v2.2/files/async
```

**Fetch and scan a remote URL:**

```shell
curl -u key:secret \
  -d "location=http://localhost:4000/static/eicar.txt" \
  http://localhost:4000/v2.2/files/fetch
```

**Scan the EICAR test file (triggers a malware finding):**

```shell
curl -u key:secret \
  -F "file=@-" \
  http://localhost:4000/v2.2/files < <(curl -s http://localhost:4000/static/eicar.txt)
```

**Create and retrieve an auth token:**

```shell
# Create a token valid for 600 seconds
curl -u key:secret -d "timeout=600" http://localhost:4000/v2.2/auth/tokens

# Retrieve it
curl -u key:secret http://localhost:4000/v2.2/auth/tokens/TOKEN_ID

# Delete it
curl -u key:secret -X DELETE http://localhost:4000/v2.2/auth/tokens/TOKEN_ID
```

### How the engine works

The local server does not perform real content analysis. Instead, it computes SHA-1 and SHA-256 hashes of uploaded content and matches them against a static rule database. The [built-in rules](internal/engine/default.json) include signatures for:

| Sample file | Finding | Trigger |
|-------------|---------|---------|
| `/static/eicar.txt` | `content.malicious.eicar-test-signature` | Standard EICAR test string |
| `/static/samples/image.jpg` | `content.image.nsfw.nudity` | Sample NSFW image |
| `/static/samples/language.txt` | `content.en.language.nsfw.0` | Sample unsafe-language text |
| `/static/samples/malware` | `content.malicious.local-test-file` | Generic malware test file |

Any file that does not match a known signature returns an empty findings list.

### Custom engine rules

For more sophisticated testing, provide your own rules file:

```shell
sc server --engine /path/to/rules.json
```

The JSON format is:

```json
{
  "callback_wait": "100ms",
  "rules": [
    {
      "format": "sha256",
      "content": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
      "result": "your.custom.finding"
    }
  ]
}
```

`callback_wait` is optional and must be a duration string such as `"100ms"` or
`"2s"`; `--callback-wait` overrides it when passed.

Supported hash formats are `sha1` and `sha256`. Generate a hash for your test file with:

```shell
shasum -a 256 /path/to/your/test-file
```

The rules in your file replace the built-in ones rather than adding to them, so
`Engine Rules:` in the startup banner reflects your file alone. A config that is
missing, unreadable, or not a valid rules file stops the server from starting
instead of quietly falling back to the built-in rules:

```
% sc server --engine foo
error: opening engine config: open foo: no such file or directory
```

### Callbacks

The local server supports callbacks. When a `callback` URL is included in an async or fetch request, the server POSTs a JSON payload to that URL containing the processing result (id, findings, checksum, content_type, content_length, creation_date, and metadata). The callback fires after a configurable delay: 100ms by default, `callback_wait` in an `--engine` config, or `--callback-wait` which overrides both. The effective value is shown as `Callback Wait:` in the startup banner.

Callbacks are fire-and-forget: if the target URL is unreachable, the delivery fails silently and the server continues operating normally.

## Using the Docker image in CI

The Docker image is the simplest way to run the local server as a service in CI pipelines for integration testing.

### GitHub Actions

Use the `services` block to start the local server alongside your test job:

```yaml
jobs:
  test:
    runs-on: ubuntu-latest
    services:
      scanii:
        image: ghcr.io/scanii/scanii-cli:latest
        ports:
          - 4000:4000
        options: >-
          --health-cmd "wget -qO- http://localhost:4000/v2.2/ping || exit 1"
          --health-interval 5s
          --health-timeout 3s
          --health-retries 5
    env:
      SCANII_ENDPOINT: http://localhost:4000
      SCANII_KEY: key
      SCANII_SECRET: secret
    steps:
      - uses: actions/checkout@v4

      - name: Run integration tests
        run: make test-integration
```

If you need the local server across a matrix of operating systems (including macOS and Windows where Docker `services` are not available), download the binary from GitHub Releases instead:

```yaml
jobs:
  test:
    runs-on: ${{ matrix.os }}
    strategy:
      matrix:
        os: [ubuntu-latest, macos-latest, windows-latest]
    steps:
      - uses: actions/checkout@v4

      - name: Download scanii-cli
        shell: bash
        run: |
          case "${{ runner.os }}" in
            Linux)   OS=linux;   ARCH=amd64; EXT=tar.gz ;;
            macOS)   OS=darwin;  ARCH=amd64; EXT=tar.gz ;;
            Windows) OS=windows; ARCH=amd64; EXT=zip    ;;
          esac
          gh release download --repo scanii/scanii-cli \
            --pattern "scanii-cli-*-${OS}-${ARCH}.${EXT}" \
            --dir /tmp
          # Extract and add to PATH
          if [ "$EXT" = "tar.gz" ]; then
            tar -xzf /tmp/scanii-cli-*.${EXT} -C /tmp
          else
            unzip /tmp/scanii-cli-*.${EXT} -d /tmp
          fi

      - name: Start local server
        shell: bash
        run: |
          /tmp/scanii-cli-*/sc server &
          # Wait for the server to be ready
          for i in $(seq 1 30); do
            curl -sf http://localhost:4000/v2.2/ping && break
            sleep 1
          done

      - name: Run integration tests
        run: make test-integration
```

### GitLab CI

```yaml
test:
  image: your-app-image:latest
  services:
    - name: ghcr.io/scanii/scanii-cli:latest
      alias: scanii
      command: ["server", "--address", "0.0.0.0:4000"]
  variables:
    SCANII_ENDPOINT: http://scanii:4000
    SCANII_KEY: key
    SCANII_SECRET: secret
  script:
    - make test-integration
```

### Docker Compose

For local development, add the server to your `docker-compose.yml`:

```yaml
services:
  scanii:
    image: ghcr.io/scanii/scanii-cli:latest
    command: ["server", "--address", "0.0.0.0:4000"]
    ports:
      - "4000:4000"
    healthcheck:
      test: ["CMD", "wget", "-qO-", "http://localhost:4000/v2.2/ping"]
      interval: 5s
      timeout: 3s
      retries: 5

  your-app:
    build: .
    depends_on:
      scanii:
        condition: service_healthy
    environment:
      SCANII_ENDPOINT: http://scanii:4000
      SCANII_KEY: key
      SCANII_SECRET: secret
```

### Test credentials

When using the local server (Docker or binary), the default credentials are:

| Setting | Value |
|---------|-------|
| Endpoint | `http://localhost:4000` |
| API Key | `key` |
| API Secret | `secret` |

## Global flags

| Flag | Description |
|------|-------------|
| `-v, --verbose` | Enable debug logging |
| `-p, --profile NAME` | Use a named profile (default: `default`) |

## All commands

| Command | Description |
|---------|-------------|
| `sc profile create [name]` | Create or update a profile |
| `sc profile list [name]` | List profiles or show details of one |
| `sc profile delete <name>` | Delete a profile |
| `sc ping` | Test API connectivity |
| `sc account` | Show account information |
| `sc files process <path>` | Synchronous file/directory scan |
| `sc files async <path>` | Asynchronous file/directory scan |
| `sc files fetch <url>` | Fetch and scan a remote URL |
| `sc files retrieve <id>` | Retrieve a scan result |
| `sc files trace <id>` | Retrieve the processing trace for a scan result |
| `sc auth-token create` | Create a temporary auth token |
| `sc auth-token retrieve <id>` | Retrieve token details |
| `sc auth-token delete <id>` | Revoke a token |
| `sc server` | Start the local server |
| `sc version` | Display version and build info |

Run `sc help` or `sc <command> --help` for detailed usage of any command.

## Known limitations

- The local server engine does not perform real content analysis; it matches files by hash only
- Requests that fail to download a remote URL via `/files/fetch` record an error but the result is still stored

## License

See [LICENSE](LICENSE) for details.
