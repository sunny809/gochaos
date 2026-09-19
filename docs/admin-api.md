# Admin REST API Reference

> gochaos exposes a RESTful admin API for managing stubs, viewing request logs,
> checking server health, and resetting server state.
>
> The admin API is mounted under the `/__admin/` prefix. By default it shares the
> main server port. Use `--admin-port` to separate it onto its own port.

## Base URL

| Mode | Admin URL |
|------|-----------|
| Shared port (default) | `http://localhost:8080/__admin/` |
| Separate port | `http://localhost:8081/__admin/` |

---

## Endpoints

### Health Check

```
GET /__admin/health
```

Check if the server is running and get basic statistics.

**Response** `200 OK`:

```json
{
  "status": "ok",
  "stubCount": 5,
  "requestCount": 42
}
```

**Example**:

```bash
curl http://localhost:8080/__admin/health
```

---

### Liveness Probe

```
GET /__admin/health/live
```

Kubernetes liveness probe. Returns 200 OK if the HTTP server is running and
accepting connections. No dependency checks — pure "is the process alive" signal.

**Response** `200 OK`:

```json
{
  "status": "alive"
}
```

**Example**:

```bash
curl http://localhost:8080/__admin/health/live
```

---

### Readiness Probe

```
GET /__admin/health/ready
```

Kubernetes readiness probe. Returns 200 OK when the server can serve requests
(stub registry initialized). Returns 503 Service Unavailable during shutdown.

**Response** `200 OK`:

```json
{
  "status": "ready",
  "stubCount": 5
}
```

**Response** `503 Service Unavailable` (during shutdown):

```json
{
  "status": "not ready — shutting down"
}
```

**Example**:

```bash
curl http://localhost:8080/__admin/health/ready
```

---

### Server Metrics

```
GET /__admin/metrics
```

Returns internal server metrics as a JSON map of counter names to values. All
counters are monotonic (never reset) except `stubs_registered` which reflects
the current stub count.

**Response** `200 OK`:

```json
{
  "requests_total": 1000,
  "requests_matched": 850,
  "requests_unmatched": 150,
  "faults_injected": 42,
  "faults_delayed": 310,
  "nearmiss_queries": 5,
  "stubs_registered": 23,
  "admin_operations": 89
}
```

**Counters**:

| Counter | Description |
|---------|-------------|
| `requests_total` | Total requests received |
| `requests_matched` | Requests that matched a stub |
| `requests_unmatched` | Requests that returned 404 |
| `faults_injected` | Faults injected (all types) |
| `faults_delayed` | Responses with configured delay |
| `nearmiss_queries` | Near-miss diagnostic API calls |
| `stubs_registered` | Currently registered stubs |
| `admin_operations` | Admin API calls |

**Example**:

```bash
curl http://localhost:8080/__admin/metrics
```

---

### Prometheus Metrics

```
GET /__admin/metrics/prometheus
```

Returns all server metrics in Prometheus text format (exposition format v0.0.4).
This endpoint is **always available** regardless of whether `WithPrometheusEndpoint`
is configured. The user-facing path (e.g., `/metrics`) is an additional option.

**Response** `200 OK`:

```
# HELP gochaos_requests_total Total requests received
# TYPE gochaos_requests_total counter
gochaos_requests_total 42
# HELP gochaos_requests_matched Requests that matched a stub
# TYPE gochaos_requests_matched counter
gochaos_requests_matched 40
# HELP gochaos_requests_unmatched Requests that did not match any stub
# TYPE gochaos_requests_unmatched counter
gochaos_requests_unmatched 2
# HELP gochaos_faults_injected Total faults injected
# TYPE gochaos_faults_injected counter
gochaos_faults_injected 7
# HELP gochaos_delays_applied Delays applied to responses
# TYPE gochaos_delays_applied counter
gochaos_delays_applied 15
# HELP gochaos_nearmiss_queries Near-miss diagnostic queries
# TYPE gochaos_nearmiss_queries counter
gochaos_nearmiss_queries 0
# HELP gochaos_stubs_registered Currently registered stub count
# TYPE gochaos_stubs_registered gauge
gochaos_stubs_registered 5
# HELP gochaos_admin_operations Admin API operations
# TYPE gochaos_admin_operations counter
gochaos_admin_operations 10
```

**Metrics**:

| Prometheus Name | Type | Description |
|----------------|------|-------------|
| `gochaos_requests_total` | counter | Total requests received |
| `gochaos_requests_matched` | counter | Requests that matched a stub |
| `gochaos_requests_unmatched` | counter | Requests that did not match any stub |
| `gochaos_faults_injected` | counter | Faults injected (all types) |
| `gochaos_delays_applied` | counter | Responses with configured delay |
| `gochaos_nearmiss_queries` | counter | Near-miss diagnostic API calls |
| `gochaos_stubs_registered` | gauge | Currently registered stubs |
| `gochaos_admin_operations` | counter | Admin API operations |

**Notes**:

- The Content-Type is `text/plain; version=0.0.4` (the canonical Prometheus exposition format media type).
- Zero-valued metrics are still emitted. This is correct Prometheus behavior -- a zero counter means "no activity yet."
- All metric names begin with `gochaos_` for easy discovery in Prometheus and Grafana.
- Metrics are read at scrape time via atomic operations on `expvar.Int` -- no blocking, no stale snapshots.

**Example**:

```bash
curl http://localhost:8080/__admin/metrics/prometheus
```

See [Metrics](features/metrics.md) for the comprehensive guide, including how to enable a user-facing path with `WithPrometheusEndpoint`.

---

### Create Stub

```
POST /__admin/mappings
```

Register a new stub definition. Accepts JSON in the request body.

**Request Body** (`application/json`):

```json
{
  "name": "get-users",
  "request": {
    "method": "GET",
    "urlPath": "/api/users"
  },
  "response": {
    "status": 200,
    "headers": {
      "Content-Type": "application/json"
    },
    "body": "{\"users\":[{\"id\":1,\"name\":\"Alice\"}]}"
  }
}
```

**Response** `201 Created`:

Returns the full stub definition with a generated `id` field.

```json
{
  "id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
  "name": "get-users",
  "request": {
    "method": "GET",
    "urlPath": "/api/users"
  },
  "response": {
    "status": 200,
    "headers": {
      "Content-Type": "application/json"
    },
    "body": "{\"users\":[{\"id\":1,\"name\":\"Alice\"}]}"
  }
}
```

**Error** `400 Bad Request`:

```json
{
  "error": "invalid fault type \"INVALID\"; valid types: error, empty, connection_reset, malformed, random_data, slow_close, rate_limit"
}
```

> **Note**: The admin API supports all 7 fault types from Phase 1. See [Fault Injection](features/fault-injection.md) for details on each type.

**Examples**:

```bash
# Minimal stub
curl -X POST http://localhost:8080/__admin/mappings \
  -H 'Content-Type: application/json' \
  -d '{"request":{"method":"GET","urlPath":"/ping"},"response":{"status":200,"body":"pong"}}'

# Stub with regex path matching
curl -X POST http://localhost:8080/__admin/mappings \
  -H 'Content-Type: application/json' \
  -d '{"request":{"method":"GET","urlPathRegex":"^/api/users/\\d+$"},"response":{"status":200,"body":"{\"id\":1}"}}'

# Stub with query params
curl -X POST http://localhost:8080/__admin/mappings \
  -H 'Content-Type: application/json' \
  -d '{"request":{"method":"GET","urlPath":"/api/search","queryParams":{"q":".*","page":"1"}},"response":{"status":200,"body":"[]"}}'

# Stub with headers matching
curl -X POST http://localhost:8080/__admin/mappings \
  -H 'Content-Type: application/json' \
  -d '{"request":{"method":"GET","urlPath":"/api/admin","headers":{"X-API-Key":"secret.*"}},"response":{"status":200,"body":"\"admin data\""}}'

# Stub with body matching (exact match)
curl -X POST http://localhost:8080/__admin/mappings \
  -H 'Content-Type: application/json' \
  -d '{"request":{"method":"POST","urlPath":"/api/users","body":{"exactMatch":"{\"name\":\"Alice\"}"}},"response":{"status":201,"body":"{\"id\":1}"}}'

# Stub with body matching (JSONPath)
curl -X POST http://localhost:8080/__admin/mappings \
  -H 'Content-Type: application/json' \
  -d '{"request":{"method":"POST","urlPath":"/api/orders","body":{"jsonPath":"$.items[?(@.price > 100)]"}},"response":{"status":201,"body":"{\"status\":\"high-value-order\"}"}}'

# Stub with fault injection (error)
curl -X POST http://localhost:8080/__admin/mappings \
  -H 'Content-Type: application/json' \
  -d '{"request":{"method":"GET","urlPath":"/api/unstable"},"response":{"fault":{"type":"error"}}}'

# Stub with fault injection (empty response)
curl -X POST http://localhost:8080/__admin/mappings \
  -H 'Content-Type: application/json' \
  -d '{"request":{"method":"GET","urlPath":"/api/empty"},"response":{"fault":{"type":"empty"}}}'

# Stub with delay + fault combination
curl -X POST http://localhost:8080/__admin/mappings \
  -H 'Content-Type: application/json' \
  -d '{"request":{"method":"GET","urlPath":"/api/slow-error"},"response":{"fault":{"type":"error"},"delay":{"type":"fixed","value":1000}}}'

# Stub with priority (lower value = higher priority)
curl -X POST http://localhost:8080/__admin/mappings \
  -H 'Content-Type: application/json' \
  -d '{"priority":1,"request":{"method":"GET","urlPath":"/api/users/1"},"response":{"status":200,"body":"{\"id\":1,\"name\":\"VIP\"}"}}'
```

---

### List All Stubs

```
GET /__admin/mappings
```

Returns all registered stubs in priority order.

**Response** `200 OK`:

```json
{
  "mappings": [
    { "id": "...", "name": "stub-a", "request": {...}, "response": {...} },
    { "id": "...", "name": "stub-b", "request": {...}, "response": {...} }
  ],
  "meta": {
    "total": 2
  }
}
```

**Example**:

```bash
curl http://localhost:8080/__admin/mappings | jq .
```

---

### Get Stub by ID

```
GET /__admin/mappings/{id}
```

Returns a single stub definition by its UUID.

**Response** `200 OK`: Full stub definition object.

**Response** `404 Not Found`:

```json
{
  "error": "stub not found: non-existent-id"
}
```

**Example**:

```bash
curl http://localhost:8080/__admin/mappings/a1b2c3d4-e5f6-7890-abcd-ef1234567890
```

---

### Delete Stub by ID

```
DELETE /__admin/mappings/{id}
```

**Response** `204 No Content`: Stub removed successfully.

**Response** `404 Not Found`: No stub with that ID exists.

**Example**:

```bash
curl -X DELETE http://localhost:8080/__admin/mappings/a1b2c3d4-e5f6-7890-abcd-ef1234567890
```

---

### Delete All Stubs

```
DELETE /__admin/mappings
```

Removes all registered stubs. Does not clear the request log.

**Response** `204 No Content`

**Example**:

```bash
curl -X DELETE http://localhost:8080/__admin/mappings
```

---

### List Request Log

```
GET /__admin/requests
GET /__admin/requests?filter=matched
GET /__admin/requests?filter=unmatched
```

Returns logged requests in chronological order (oldest first).

**Query Parameters**:

| Parameter | Description |
|-----------|-------------|
| `filter` | Optional. `matched` — only matched requests. `unmatched` — only unmatched requests. Omit for all. |

**Response** `200 OK`:

```json
[
  {
    "method": "GET",
    "path": "/api/users",
    "queryString": "",
    "headers": {
      "Accept": ["*/*"],
      "User-Agent": ["curl/8.0.0"]
    },
    "body": "",
    "receivedAt": "2026-06-13T12:00:00Z"
  }
]
```

**Examples**:

```bash
# All requests
curl http://localhost:8080/__admin/requests

# Only matched requests
curl 'http://localhost:8080/__admin/requests?filter=matched'

# Only unmatched requests
curl 'http://localhost:8080/__admin/requests?filter=unmatched'
```

---

### Clear Request Log

```
DELETE /__admin/requests
```

Clears all logged requests. Does not affect registered stubs.

**Response** `204 No Content`

**Example**:

```bash
curl -X DELETE http://localhost:8080/__admin/requests
```

---

### Near-Miss Diagnostics

```
POST /__admin/nearmiss
```

Submit a request pattern to see which registered stubs come closest to matching.
Useful for debugging 404 responses — the engine compares your request against
all registered stubs and returns a per-dimension breakdown for the top near misses.

**Request Body** (`application/json`):

```json
{
  "method": "GET",
  "path": "/api/users",
  "headers": {"Accept": "application/json"}
}
```

**Response** `200 OK`:

```json
{
  "nearMisses": [
    {
      "stubId": "abc-123",
      "name": "get-user",
      "scoreBreakdown": {
        "method":  {"score": 1, "maxScore": 1, "matched": true},
        "path":    {"score": 0, "maxScore": 1, "matched": false},
        "headers": {"score": 1, "maxScore": 1, "matched": true}
      },
      "totalScore": 6,
      "maxPossibleScore": 7
    }
  ]
}
```

**Fields**:

| Field | Description |
|-------|-------------|
| `nearMisses` | Array of near-miss results, sorted by match quality (best first) |
| `stubId` | UUID of the near-missing stub |
| `name` | Human-readable name of the near-missing stub |
| `scoreBreakdown` | Per-dimension match scores: `method`, `path`, `headers`, `query`, `body`, `cookies` |
| `totalScore` | Total matched dimensions for this stub |
| `maxPossibleScore` | Maximum possible score for this stub |

**Example**:

```bash
curl -X POST http://localhost:8080/__admin/nearmiss \
  -H 'Content-Type: application/json' \
  -d '{"method":"GET","path":"/api/users"}'
```

> **Note**: Near-miss info is also automatically included in every 404 response body
> under the `nearMisses` field. See [Near-Miss Diagnostics](features/near-miss-diagnostics.md)
> for a complete guide.

---

### List Fault Injection Log

```
GET /__admin/fault-log
```

Returns all fault injection events in chronological order (oldest first). Each entry
records when a fault was injected, which stub triggered it, and the activation mode.

**Response** `200 OK`:

```json
{
  "entries": [
    {
      "stubId": "stub-123",
      "faultType": "connection_reset",
      "activatedAt": "2026-06-20T10:30:45Z",
      "requestMethod": "GET",
      "requestPath": "/api/downstream",
      "activationMode": "probability"
    }
  ],
  "count": 1
}
```

**Entry Fields**:

| Field | Description |
|-------|-------------|
| `stubId` | UUID of the stub that injected the fault (empty when a timeline event fired with no stub match) |
| `faultType` | Type of fault: `error`, `empty`, `connection_reset`, `malformed`, `random_data`, `slow_close`, `rate_limit`, or `delay` |
| `delayMs` | Configured delay value for `delay` entries |
| `activatedAt` | Timestamp when the fault was injected (UTC) |
| `requestMethod` | HTTP method of the triggering request |
| `requestPath` | Path of the triggering request |
| `activationMode` | How the fault was activated: `always`, `probability`, `nth_request`, `time_window`, `combined`, `rate_limit`, `timeline`; empty for unconditional delay-only stubs |
| `timelineEvent` | 1-based index of the timeline event that injected this entry (0 = not timeline-driven) |

**Example**:

```bash
curl http://localhost:8080/__admin/fault-log | jq .
```

---

### Clear Fault Injection Log

```
DELETE /__admin/fault-log
```

Clears all logged fault injection events. Does not affect registered stubs or request log.

**Response** `200 OK`:

```json
{
  "cleared": true,
  "count": 3
}
```

The `count` field indicates how many entries were cleared.

**Example**:

```bash
curl -X DELETE http://localhost:8080/__admin/fault-log
```

---

### Reset Server State

```
POST /__admin/reset
```

Resets all server state:
- Removes all registered stubs
- Clears the request log, fault injection log, and callback log
- Clears the loaded timeline's counters and recorded fires (the event table
  survives, so re-run starts from scratch)
- Re-baselines the time-keyed epoch to the reset moment (`timeMs` triggers
  re-arm from zero)
- Resets all metrics

**Response** `200 OK`

**Example**:

```bash
curl -X POST http://localhost:8080/__admin/reset
```

---

### Chaos Report

```
GET /__admin/report
GET /__admin/report?format=junit
GET /__admin/report?format=json
```

Exports the fault-injection log as CI-visible chaos evidence — JUnit XML that
renders as a failing test suite in dashboards, or JSON for scripts. One
failing `<testcase>` per injection. The default format is `json`; an invalid
format returns 400. See [chaos-report.md](features/chaos-report.md).

**Response** `200 OK` (JSON):

```json
{
  "suite": "gmock-chaos",
  "entries": [
    {
      "stubId": "stub-123",
      "faultType": "error",
      "activatedAt": "2026-06-20T10:30:45Z",
      "requestMethod": "GET",
      "requestPath": "/api/downstream",
      "activationMode": "probability"
    }
  ]
}
```

**Example**:

```bash
curl 'http://localhost:8080/__admin/report?format=junit' > chaos-report.xml
```

---

## Complete Request Pattern Reference

The following fields are available for matching incoming requests:

```json
{
  "method": "GET",
  "urlPath": "/api/users",
  "urlPathRegex": "^/api/users/\\d+$",
  "accept": "application/json",
  "headers": {
    "X-API-Key": "sk-.*",
    "Authorization": "Bearer .+"
  },
  "queryParams": {
    "page": "\\d+",
    "sort": "name"
  },
  "cookies": {
    "session_id": "sess-[a-z0-9]+"
  },
  "body": {
    "exactMatch": "{\"name\":\"Alice\"}",
    "regexMatch": "\"name\":\"[A-Z][a-z]+\"",
    "jsonPath": "$.users[?(@.age > 18)]"
  },
  "priority": 0
}
```

| Field | Type | Description |
|-------|------|-------------|
| `method` | string | HTTP method. Empty = any method. |
| `urlPath` | string | Exact path match. |
| `urlPathRegex` | string | Regex path match (compiled once at registration). |
| `accept` | string | Accept header with proper media type negotiation. |
| `headers` | map[string]string | Header name → regex pattern. |
| `queryParams` | map[string]string | Query param name → regex pattern. |
| `cookies` | map[string]string | Cookie name → regex pattern. |
| `body.exactMatch` | string | Exact body match. |
| `body.regexMatch` | string | Regex body match. |
| `body.jsonPath` | string | JSONPath body match (uses `github.com/PaesslerAG/jsonpath`). |
| `priority` | int | Lower values = higher priority. |

---

## Complete Response Definition Reference

```json
{
  "status": 200,
  "headers": {
    "Content-Type": "application/json"
  },
  "body": "{\"status\":\"ok\"}",
  "base64Body": "SGVsbG8gV29ybGQ=",
  "transformResponse": true,
  "fault": {
    "type": "error"
  },
  "delay": {
    "type": "fixed",
    "value": 500
  }
}
```

| Field | Type | Description |
|-------|------|-------------|
| `status` | int | HTTP status code. 0 defaults to 200. |
| `headers` | map[string]string | Response headers. |
| `body` | string | Response body string. |
| `base64Body` | string | Base64-encoded binary body (takes precedence over `body`). |
| `transformResponse` | bool | Enable template rendering (see Response Templating docs). |
| `fault.type` | string | `"error"`, `"empty"`, or `"connection_reset"`. |
| `delay.type` | string | `"fixed"` or `"random"`. |
| `delay.value` | int | Fixed delay in milliseconds. |
| `delay.min` | int | Minimum random delay in milliseconds. |
| `delay.max` | int | Maximum random delay in milliseconds. |

---

## Error Response Format

All error responses follow this format:

```json
{
  "error": "description of what went wrong"
}
```

| HTTP Status | Meaning |
|-------------|---------|
| `400` | Invalid stub definition (validation error) |
| `404` | Endpoint or stub not found |
| `405` | Method not allowed on this endpoint |
| `500` | Internal server error |

---

## Example Workflow

```bash
# 1. Start server (background)
gmock start --port 8080 &
sleep 1

# 2. Check health
curl -s http://localhost:8080/__admin/health | jq .

# 3. Create multiple stubs
curl -X POST http://localhost:8080/__admin/mappings \
  -H 'Content-Type: application/json' \
  -d '{"name":"health","request":{"method":"GET","urlPath":"/health"},"response":{"status":200,"body":"\"ok\""}}'

curl -X POST http://localhost:8080/__admin/mappings \
  -H 'Content-Type: application/json' \
  -d '{"name":"users","request":{"method":"GET","urlPath":"/api/users"},"response":{"status":200,"headers":{"Content-Type":"application/json"},"body":"[{\"id\":1,\"name\":\"Alice\"}]"}}'

# 4. List all stubs
curl -s http://localhost:8080/__admin/mappings | jq .

# 5. Test the mocked endpoints
curl -s http://localhost:8080/health
curl -s http://localhost:8080/api/users

# 6. View request log
curl -s http://localhost:8080/__admin/requests | jq .

# 7. Delete a specific stub
STUB_ID=$(curl -s http://localhost:8080/__admin/mappings | jq -r '.mappings[0].id')
curl -X DELETE "http://localhost:8080/__admin/mappings/$STUB_ID"

# 8. Reset everything
curl -X POST http://localhost:8080/__admin/reset
```
