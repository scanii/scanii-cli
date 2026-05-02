# Scanii CLI

The Scanii CLI (`sc`) helps you build, test, and manage your [Scanii](https://www.scanii.com) integration right from the terminal.

**With the CLI, you can:**

- Interact with the Scanii API: scan files, manage auth tokens, and check account info
- Start a local server that simulates the Scanii API for integration testing
- Process entire directories of files with concurrent workers and progress tracking

## Installation

### Binary releases

Pre-built binaries for macOS, Windows, and Linux are available on the [releases page](https://github.com/scanii/scanii-cli/releases).

On macOS, you may need to remove the quarantine attribute before running:

```shell
xattr -d com.apple.quarantine /path/to/sc
```

### Docker

A container image is published to the GitHub Container Registry:

```shell
docker run ghcr.io/scanii/scanii-cli:latest
```

Previous versions are listed [here](https://github.com/scanii/scanii-cli/pkgs/container/scanii-cli).

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

Example output:

```
Using endpoint: localhost:4000 and API key: key
success in 4.4ms
Credentials worked against http://localhost:4000/v2.2/
Processing directory:  .
Found 39 file(s)
Processing files 100% |████████████████████████████████████████| (39/39, 918 it/s)

Completed in 150ms
Files with findings: 4, unable to process: 0 and successfully processed: 39
Files with findings:
------
  path:           samples/eicar.txt
  id:             fd33128a8da445d3b8308fe6d1588829
  checksum/sah1:  3395856ce81f2b7382dee72602f798b642f14140
  content type:   text/plain; charset=utf-8
  content length: 68 B
  creation date:  2024-02-08T13:38:02.074502Z
  findings:       content.malicious.eicar-test-signature
  metadata:       none
------
```

### 6. Manage auth tokens

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
API Key:      key
API Secret:   secret
Engine Rules: 5
Address:      http://localhost:4000

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
| `-w, --callback-wait` | `100ms` | Delay before firing callbacks |

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
  "rules": [
    {
      "format": "sha256",
      "content": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
      "result": "your.custom.finding"
    }
  ]
}
```

Supported hash formats are `sha1` and `sha256`. Generate a hash for your test file with:

```shell
shasum -a 256 /path/to/your/test-file
```

### Callbacks

The local server supports callbacks. When a `callback` URL is included in an async or fetch request, the server POSTs a JSON payload to that URL containing the processing result (id, findings, checksum, content_type, content_length, creation_date, and metadata). The callback fires after a configurable delay (default 100ms, controlled by `--callback-wait`).

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
