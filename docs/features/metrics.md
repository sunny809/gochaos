# Metrics & Prometheus Export

> gmock exposes eight operational counters covering request throughput, fault
> injection, stub registration, and admin API usage. These metrics are available
> in two formats: JSON (for programmatic consumption) and Prometheus text format
> (for monitoring stacks). Both formats are zero-dependency -- no Prometheus
> client library is required.

## Why Export Metrics?

A mock server in a CI pipeline is invisible unless it surfaces data. Without
metrics, you cannot tell:

- Did the test actually send requests to the mock, or did it miss the port?
- Did the chaos injection fire at the expected rate, or did the activation
  window slip?
- Did the SUT retry correctly, or did it give up after the first fault?

The eight counters answer these questions. Export them to Prometheus and
visualize with the provided Grafana dashboard, or read them as JSON in tests.

---

## The Eight Metrics

| Metric | Prometheus Name | Type | Description |
|--------|----------------|------|-------------|
| Total requests | `gochaos_requests_total` | counter | Every incoming request, regardless of match |
| Matched requests | `gochaos_requests_matched` | counter | Requests that matched a registered stub |
| Unmatched requests | `gochaos_requests_unmatched` | counter | Requests that returned 404 (no matching stub) |
| Faults injected | `gochaos_faults_injected` | counter | Fault injection events (all 7 types) |
| Delays applied | `gochaos_delays_applied` | counter | Responses with a configured delay |
| Near-miss queries | `gochaos_nearmiss_queries` | counter | Calls to `POST /__admin/nearmiss` or `s.NearMiss()` |
| Stubs registered | `gochaos_stubs_registered` | gauge | Current count of registered stubs |
| Admin operations | `gochaos_admin_operations` | counter | Admin API calls (CRUD, reset, log queries) |

### Counter vs Gauge

- **Counters** (`counter`): Monotonically increase. Use `rate()` in PromQL to
  measure throughput.
- **Gauge** (`gauge`): Reflects the current value, which may go up or down.
  `gochaos_stubs_registered` decreases when stubs are deleted.

Naming convention: all Prometheus metric names use the `gochaos_` prefix for
easy discovery across monitoring stacks.

---

## Exposing Metrics

Metrics are available in two ways. Both are always available unless noted.

### 1. Admin API (always-on, no configuration needed)

```
GET /__admin/metrics           # JSON format
GET /__admin/metrics/prometheus # Prometheus text format
```

The JSON endpoint returns a map:

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

The Prometheus endpoint returns exposition format v0.0.4:

```
# HELP gochaos_requests_total Total requests received
# TYPE gochaos_requests_total counter
gochaos_requests_total 1000
...
```

**Content-Type**: `text/plain; version=0.0.4` (canonical Prometheus media type).
The admin path uses the same internal `WritePrometheus` method as the user-facing path.

### 2. User-Facing Path (opt-in via `WithPrometheusEndpoint`)

Configure a custom path that does not collide with your application routes:

```go
server := gmock.NewServer(
    gmock.WithPort(8080),
    gmock.WithPrometheusEndpoint("/metrics"),
)
```

When set, `GET /metrics` returns the same Prometheus-format metrics as the
admin path. This is useful when Prometheus scrapes gmock on the same port as
your application and you control the path namespace.

**Path resolution rules**:

| `WithPrometheusEndpoint` | User-facing path | Admin path |
|--------------------------|------------------|------------|
| Not set | None | `/__admin/metrics/prometheus` (always) |
| `"/metrics"` | `GET /metrics` | `/__admin/metrics/prometheus` (always) |
| `"/custom/metrics"` | `GET /custom/metrics` | `/__admin/metrics/prometheus` (always) |

If the configured path collides with a mock route (e.g., `"/api/data"`), the
Prometheus endpoint takes priority: requests to that path return metrics, not
mock responses. This is intentional -- the user explicitly configured it.

---

## Using Metrics in Prometheus

### Scrape Configuration

Configure Prometheus to scrape the always-on admin path:

```yaml
# prometheus.yml
scrape_configs:
  - job_name: gmock
    metrics_path: /__admin/metrics/prometheus
    static_configs:
      - targets:
          - gmock:8080
```

If you configured a user-facing path with `WithPrometheusEndpoint("/metrics")`:

```yaml
  - job_name: gmock
    metrics_path: /metrics
    static_configs:
      - targets:
          - gmock:8080
```

### Useful PromQL Queries

```promql
# Request rate (1-minute rolling average)
rate(gochaos_requests_total[1m])

# Match rate percentage
gochaos_requests_matched / gochaos_requests_total * 100

# Fault injection rate
rate(gochaos_faults_injected[1m])

# Delay rate
rate(gochaos_delays_applied[1m])

# Near-miss query rate (spikes indicate configuration drift)
rate(gochaos_nearmiss_queries[1m])

# Current stub count
gochaos_stubs_registered
```

### Grafana Dashboard

A pre-built Grafana dashboard (`gmock-overview.json`) is available in the
[gochaos/prometheus](https://github.com/sunny809/prometheus) repository. It
includes seven panels:

| Panel | Type | PromQL |
|-------|------|--------|
| Request Rate | Time series | `rate(gochaos_requests_total[1m])` |
| Match Rate | Stat | `gochaos_requests_matched / ... * 100` |
| Fault Injection Rate | Time series | `rate(gochaos_faults_injected[1m])` |
| Fault Type Distribution | Pie chart | `gochaos_faults_injected` |
| Delay Trend | Time series | `rate(gochaos_delays_applied[1m])` |
| Near-Miss Diagnostics | Time series | `rate(gochaos_nearmiss_queries[1m])` |
| Registered Stubs | Stat | `gochaos_stubs_registered` |

---

## Go Library API

### WritePrometheus

```go
func (m *Metrics) WritePrometheus(w io.Writer)
```

Writes all eight metrics in Prometheus exposition format v0.0.4 to the given
writer. Each metric includes `# HELP` and `# TYPE` headers. This method is used
internally by both the admin endpoint and the user-facing path.

**Implementation notes**:

- Zero-dependency: uses only `fmt.Fprintf` and `expvar.Int` values.
- Zero-valued metrics are still emitted (correct Prometheus behavior).
- Uses `Snapshot()` internally -- values are point-in-time atomic reads.
- Not safe for concurrent writes to the same writer (caller is responsible).

### WithPrometheusEndpoint

```go
func WithPrometheusEndpoint(path string) Option
```

Registers a Prometheus metrics handler at the given path. The path should be an
absolute URL path (e.g., `"/metrics"`). When not set, no user-facing endpoint
is registered. The admin endpoint `/__admin/metrics/prometheus` is always
available regardless.

```go
// Enable Prometheus at /metrics
server := gmock.NewServer(
    gmock.WithPort(8080),
    gmock.WithPrometheusEndpoint("/metrics"),
)
```

---

## Docker Compose Example

A full monitoring stack is available in the
[gochaos/prometheus](https://github.com/sunny809/prometheus) repository:

```yaml
# docker-compose.yml (simplified)
services:
  gmock:
    image: ghcr.io/sunny809/gochaos:latest
    ports:
      - "8080:8080"

  prometheus:
    image: prom/prometheus:latest
    ports:
      - "9090:9090"
    volumes:
      - ./prometheus.yml:/etc/prometheus/prometheus.yml:ro

  grafana:
    image: grafana/grafana:latest
    ports:
      - "3000:3000"
    environment:
      - GF_AUTH_ANONYMOUS_ENABLED=true
      - GF_AUTH_ANONYMOUS_ORG_ROLE=Admin
    volumes:
      - ./provisioning/grafana:/etc/grafana/provisioning:ro
      - ../../dashboards:/var/lib/grafana/dashboards:ro
```

Start with:

```bash
cd examples/docker-compose
docker compose up -d
```

| Service | URL |
|---------|-----|
| gmock | http://localhost:8080 |
| Prometheus | http://localhost:9090 |
| Grafana | http://localhost:3000 |

---

## Scrape Target Decision

The Prometheus scrape target is `GET /__admin/metrics/prometheus` (always-on
admin path), NOT a user-configured path. This decision means:

- Docker Compose works out of the box -- no gmock configuration needed.
- No `--prometheus-metrics` CLI flag exists. The admin path is always available.
- Users who want a custom path for their own Prometheus setup can use
  `WithPrometheusEndpoint`, but the Docker Compose example does not depend on it.

---

## Related

- [Verification & Request Log](verification.md) -- assert metric values in tests
- [Fault Injection](fault-injection.md) -- what each fault type does
- [Admin API Reference](../admin-api.md) -- all `/__admin/` endpoints
