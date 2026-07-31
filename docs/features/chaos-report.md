# Chaos Report: Fault-Log Snapshot as CI Evidence

> The fault-injection log records every fault and delay gmock actually
> injected — stub-driven (probability / nth_request / time_window) and
> timeline-driven alike. The chaos report exports that log as CI-visible
> evidence: JUnit XML that renders as a failing test suite in dashboards, or
> JSON for scripts. The log is a ring buffer (bounded by `WithMaxRequests`,
> default 1000), so the report is a snapshot of recent chaos — not a full
> audit trail.

## What It Is

Every injection is recorded with:

| Field | Meaning |
|-------|---------|
| `stubId` | Stub that served the request (empty when timeline fired with no stub match) |
| `faultType` | `error`, `empty`, `connection_reset`, `malformed`, `random_data`, `slow_close`, `rate_limit`, or `delay` |
| `activatedAt` | RFC 3339 timestamp of the injection |
| `requestMethod` / `requestPath` | The request the injection hit |
| `activationMode` | `probability`, `nth_request`, `time_window`, `combined`, `always`, `rate_limit`, `timeline` |
| `timelineEvent` | 1-based index of the timeline event that injected this entry (0 = not timeline-driven) |
| `delayMs` | Configured delay value for `delay` entries |

`server.Reset()` clears the log along with all other state.

## JUnit XML

```
GET /__admin/report?format=junit
```

One failing `<testcase>` per injection, so the report reads as chaos evidence
in CI dashboards: **tests == injections**. `tests` and `failures` both equal
the number of logged entries.

```xml
<?xml version="1.0" encoding="UTF-8"?>
<testsuite name="gmock-chaos" tests="2" failures="2">
  <testcase name="error" classname="stub-1">
    <failure message="fault injected on GET /api/payments at 2026-07-31T12:00:00Z" type="error">error</failure>
  </testcase>
  <testcase name="connection_reset" classname="stub-2">
    <failure message="fault injected on GET /api/inventory at 2026-07-31T12:00:01Z" type="connection_reset">connection_reset</failure>
  </testcase>
</testsuite>
```

- `<testcase name>` — the fault type (or `delay`).
- `<testcase classname>` — the stub ID.
- `<failure message>` — request line + injection timestamp.

In a CI dashboard the suite shows as failing with one test per injection —
which is the point: a chaos run is *supposed* to fail, and now the failures
are structured evidence instead of log noise.

## JSON

```
GET /__admin/report?format=json
```

Structured envelope for scripts (`jq`, `yq`, assertions). `entries` is always
an array, even when empty.

```json
{
  "suite": "gmock-chaos",
  "entries": [
    {
      "stubId": "stub-1",
      "faultType": "error",
      "activatedAt": "2026-07-31T12:00:00Z",
      "requestMethod": "GET",
      "requestPath": "/api/payments",
      "activationMode": "timeline",
      "timelineEvent": 1
    }
  ]
}
```

The default format (no `?format=` param) is `json`. An invalid format returns
400.

## CLI

`gmock report` proxies the admin endpoint to stdout:

```bash
# JUnit XML for the CI artifact upload
gmock report --format junit > chaos-report.xml

# JSON, pretty-printed, for local inspection
gmock report --format json

# Against a server not on localhost:8080
gmock report --admin-url http://mock:8080 --format junit
```

| Flag | Default | Description |
|------|---------|-------------|
| `--admin-url` | `http://localhost:8080` | Base URL of the gmock admin API |
| `--format` | `json` | `json` or `junit` |

## Example: Uploading the Report in CI

```yaml
# .github/workflows/chaos.yml
jobs:
  chaos:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'

      - name: Run chaos experiment (record + replay)
        run: go test ./examples/tutorial/04-timeline-replay/... -v

      - name: Start gmock
        run: gmock start &

      - name: Export chaos evidence
        run: gmock report --format junit > chaos-report.xml

      - name: Upload report as test artifact
        uses: actions/upload-artifact@v4
        with:
          name: chaos-report
          path: chaos-report.xml

      - name: Fail loudly (chaos is evidence, not noise)
        run: test -s chaos-report.xml
```

See [Tutorial 4](../tutorials/04-timeline-replay.md) for the record → replay →
report loop, and [fault-timeline.md](fault-timeline.md) for the timeline API
that the report documents.
