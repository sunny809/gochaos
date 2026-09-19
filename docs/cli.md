# CLI Reference

> gochaos runs as a standalone HTTP mock server via the `gmock` CLI.
>
> For CI pipelines, Docker usage, and non-Go teams, this is the primary interface.

## Installation

```bash
# From source
go install github.com/sunny809/gochaos/cmd/gmock@latest

# Or download a pre-built binary from GitHub Releases
# https://github.com/sunny809/gochaos/releases
```

Verify the installation:

```bash
gmock --version
# Output: gmock version dev (or the installed version)
```

---

## Command Tree

```
gmock                 Root — show help / version
├── start             Start the HTTP mock server
├── stub              Manage stubs on a running server
│   ├── list          List all registered stubs
│   ├── create        Create a stub from a JSON/YAML file
│   ├── get           Get a stub by ID
│   └── delete        Delete a stub (or --all)
├── reset             Reset all stubs and request log
├── requests          View the request log
└── report            Export chaos evidence (JUnit XML or JSON)
```

---

## `gmock start` — Start the Server

Start the HTTP mock server. Runs until interrupted (Ctrl-C).

### Usage

```bash
gmock start [flags]
```

### Flags

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--port` | `-p` | `8080` | HTTP listen port. Use `0` for a random port. |
| `--admin-port` | | `0` (shared) | Separate admin API port. `0` = admin shares the main port under `/__admin/`. |
| `--stubs` | `-s` | | Stub file(s) to load at startup (`.yaml` or `.json`). Can be specified multiple times. |
| `--proxy-url` | | | Upstream URL for proxy fallback (requests with no matching stub are forwarded). |
| `--record` | | `false` | Enable recording of proxied request/response pairs. |
| `--verbose` | `-v` | `false` | Enable debug-level logging. |
| `--cors` | | `false` | Enable CORS with permissive defaults (allow all origins). |
| `--max-requests` | | `1000` | Maximum number of requests to keep in the ring buffer log. |

### Examples

```bash
# Start on port 8080 with no stubs (add stubs via Admin API)
gmock start

# Start on port 9090 with stubs from a YAML file
gmock start --port 9090 --stubs ./stubs.yaml

# Start with multiple stub files
gmock start --stubs ./api-stubs.yaml --stubs ./fault-stubs.json

# Start with separate admin port (recommended for production)
gmock start --port 8080 --admin-port 8081

# Start with CORS enabled
gmock start --port 8080 --cors

# Start with proxy fallback and recording
gmock start --port 8080 --proxy-url https://api.example.com --record

# Start in verbose mode for debugging
gmock start -v

# Start on a random port (useful for CI)
gmock start --port 0
```

### Output

```
gmock listening at http://[::]:8080
admin API at http://[::]:8080/__admin/
Press Ctrl-C to stop
```

---

## `gmock stub` — Manage Stubs

Manage stubs on a running gmock server via the Admin API.

### Global Flag

| Flag | Default | Description |
|------|---------|-------------|
| `--admin-url` | `http://localhost:8080` | Base URL of the running gmock server admin API. |

### Subcommands

#### `gmock stub list`

List all registered stubs in priority order.

```bash
gmock stub list
gmock stub list --admin-url http://localhost:9090
```

**Output**: Pretty-printed JSON array of all stubs.

#### `gmock stub create <file>`

Create a stub from a JSON or YAML file.

```bash
gmock stub create ./my-stub.json
gmock stub create ./stubs.yaml
```

**File format** (JSON):

```json
{
  "name": "get-users",
  "request": {
    "method": "GET",
    "urlPath": "/api/users"
  },
  "response": {
    "status": 200,
    "headers": { "Content-Type": "application/json" },
    "body": "{\"users\":[]}"
  }
}
```

**File format** (YAML):

```yaml
name: get-users
request:
  method: GET
  urlPath: /api/users
response:
  status: 200
  headers:
    Content-Type: application/json
  body: '{"users":[]}'
```

**Output**: `created stub: <uuid>`

#### `gmock stub get <id>`

Get a stub by its UUID.

```bash
gmock stub get abc123-def456
```

#### `gmock stub delete [id]`

Delete a stub by ID, or delete all stubs with `--all`.

```bash
# Delete a specific stub
gmock stub delete abc123-def456

# Delete all stubs
gmock stub delete --all
```

---

## `gmock reset` — Reset Server State

Reset all stubs, request log, fault log, callback log, timeline state, and
metrics on the running server. The timeline's time-keyed triggers re-baseline
to the reset moment, so a re-run starts a fresh epoch.

```bash
gmock reset
gmock reset --admin-url http://localhost:9090
```

**Output**: `server state reset`

---

## `gmock requests` — View Request Log

View the request log on a running server.

```bash
gmock requests
gmock requests --admin-url http://localhost:9090
gmock requests --filter unmatched
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--admin-url` | `http://localhost:8080` | Base URL of the running gmock server admin API. |
| `--filter` | | Filter: `matched`, `unmatched`, or empty for all. |

---

## `gmock report` — Export Chaos Evidence

Export the fault-injection log (every fault and delay actually injected) as
JUnit XML or JSON. See [chaos-report.md](features/chaos-report.md) for the
full semantics.

```bash
# JUnit XML for the CI artifact upload
gmock report --format junit > chaos-report.xml

# JSON, pretty-printed, for local inspection
gmock report --format json

# Against a server not on localhost:8080
gmock report --admin-url http://mock:8080 --format junit
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--admin-url` | `http://localhost:8080` | Base URL of the running gmock server admin API. |
| `--format` | `json` | Report format: `json` or `junit`. |

---

## Typical Workflows

### Quick Start (CLI)

```bash
# 1. Start server
gmock start --port 8080 &
sleep 1

# 2. Create a stub
cat > stub.json << 'EOF'
{
  "request": { "method": "GET", "urlPath": "/api/health" },
  "response": { "status": 200, "body": "{\"status\":\"ok\"}" }
}
EOF
gmock stub create stub.json

# 3. Test it
curl http://localhost:8080/api/health

# 4. View the log
gmock requests
```

### CI Pipeline Integration

```yaml
# .github/workflows/test.yml
steps:
  - name: Start gmock server
    run: |
      nohup gmock start --port 1080 --stubs ./test/stubs.yaml &
      echo "Waiting for gmock..."
      until curl -s http://localhost:1080/__admin/health; do sleep 0.5; done
  - name: Run integration tests
    run: |
      BASE_URL=http://localhost:1080 make test-integration
```

### Testing Retry Logic with Fault Injection

```bash
# Start server with fault stubs
gmock start --port 8080 --stubs ./fault-stubs.json

# Fault stubs file (fault-stubs.json):
# [
#   {
#     "name": "error-fault",
#     "request": { "method": "GET", "urlPath": "/api/unstable" },
#     "response": { "fault": { "type": "error" } }
#   }
# ]
```

---

## Exit Codes

| Code | Meaning |
|------|---------|
| `0` | Success (server stopped gracefully) |
| `1` | Error (failed to start, connect, or parse flags) |

---

## Environment

No environment variables are required. All configuration is via flags.
