# Feature Documentation

> Comprehensive documentation for every gochaos feature. Each document is self-contained
> so you can learn how to use a feature without reading the source code.

## Core Features

| Document | Description | Use Case |
|----------|-------------|----------|
| [Getting Started](getting-started.md) | First mock server in 5 minutes | New users |
| [Stub Matching](stub-matching.md) | Request pattern matching (8 dimensions) | API mocking |
| [Response Delays](response-delays.md) | 5 delay distributions: fixed, random, lognormal, timeout, dribble | Latency testing |
| [Fault Injection](fault-injection.md) | 7 fault types with activation modes | Chaos/resilience testing |
| [Advanced Chaos](advanced-chaos.md) | Phase 1: 7 fault types, 5 delay distributions, 3 activation modes, seedable RNG | Production-grade chaos engineering |
| [Fault Timeline](fault-timeline.md) | Declare / record / replay ordered fault bursts as a versioned artifact | Deterministic chaos in CI |
| [Chaos Report](chaos-report.md) | Fault-log snapshot as JUnit XML / JSON evidence | CI dashboards & artifacts |
| [Near-Miss Diagnostics](near-miss-diagnostics.md) | Why a stub didn't match — per-dimension breakdown | 404 debugging |
| [Response Templating](response-templating.md) | Dynamic responses with templates | Dynamic mocking |
| [CORS Support](cors.md) | Cross-Origin Resource Sharing | Browser-based testing |
| [Verification & Request Log](verification.md) | Asserting and inspecting requests | Test verification |
| [Gzip Compression](gzip-compression.md) | Automatic gzip response compression | Performance testing |
| [YAML / JSON Stubs](yaml-stubs.md) | Loading stubs from files | CI pipelines |

## External References

| Document | Description |
|----------|-------------|
| [Go Library API](../go-library-api.md) | Complete Go API reference (types, options, methods) |
| [CLI Reference](../cli.md) | All CLI commands and flags |
| [Admin API](../admin-api.md) | REST API reference with curl examples |

## Quick Navigation

### For New Users

1. [Getting Started](getting-started.md) — First mock server
2. [Stub Matching](stub-matching.md) — How request matching works
3. [YAML / JSON Stubs](yaml-stubs.md) — Loading stubs from files

### For Chaos/Resilience Testers

1. [Advanced Chaos](advanced-chaos.md) — 7 fault types, 5 delay distributions, activation modes, seedable RNG
2. [Fault Injection](fault-injection.md) — Network fault simulation (baseline 3 types)
3. [Response Delays](response-delays.md) — Latency simulation (baseline 2 types)
4. [Response Templating](response-templating.md) — Dynamic responses

### For CI Maintainers

1. [CLI Reference](../cli.md) — CLI usage
2. [Admin API](../admin-api.md) — REST API
3. [YAML / JSON Stubs](yaml-stubs.md) — File-based stubs
