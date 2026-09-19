# ADR-007: Stay on Go — No JVM Rewrite

## Status

Accepted

## Context

Question evaluated in September 2026: *if the repo were rewritten using the latest
JDK features, would it achieve Go-like performance with lower maintenance
complexity?*

The project's positioning explicitly trades on being a **zero-JVM, ~15MB-image**
alternative to WireMock. Any move to the JVM must be weighed against that
differentiator, not just against raw throughput.

### Empirical measurements (same machine, i5-8350U laptop, September 2026)

| Metric | Go (gmock, full pipeline) | OpenJDK 25.0.4 full (CDS) | jlink minimal runtime (2 modules) |
|--------|---------------------------|---------------------------|-----------------------------------|
| Binary/image size | **15M** (single file) | ~300MB+ | 42M |
| Time-to-ready | **25 ms** | 116 ms | 272 ms |
| RSS | **11.8 MB** | 46.7 MB | 45.8 MB |
| Sustained throughput | ~3.5K req/s (full pipeline incl. matching + templating + logging + metrics) | untested (trivial handler would be higher) | — |

Go's number is the full functional pipeline (per-request stub matching, JSONPath,
templating, fault bookkeeping); the JVM numbers are a trivial 20-line
virtual-thread server — the most optimistic possible comparison for the JVM.
The jlink image is slower than the full JDK because jlink images do not carry the
default CDS archive; closing that gap adds an AppCDS build step.

## Decision

**Remain on Go. Do not rewrite the project in Java.**

If JVM-ecosystem coverage is ever wanted (e.g., WireMock-API-compatible chaos
tooling for Java teams), build it as a **sibling project** that reuses the same
chaos API and design, never as a rewrite of this repo.

## Consequences

**Positive:**

- Preserves the zero-JVM / 15MB / milliseconds-startup differentiator that the
  positioning and comparison tables are built on.
- Sustained-throughput parity is achievable on the JVM only for the metric that
  least matters to this tool (steady-state req/s); the metrics that matter most to
  a CI-gated test tool — startup time, image size, memory footprint, cold-start
  latency — are order-of-magnitude wins for Go.
- Go's maintenance story (single binary, `go build`, built-in `go test -race`,
  no build-system/GC/JVM-flag surface) is already the low-complexity option for a
  tool of this scale; modern JDK language ergonomics (records, pattern matching,
  virtual threads) do not offset the added toolchain weight.

**Negative:**

- JVM-ecosystem users who prefer a JVM-native tool will still choose WireMock;
  reaching them requires the sibling-project path, which is real additional work.

## Alternatives Considered

- **Full Java rewrite using latest JDK features**: Rejected. Sustained throughput
  can match, but startup/memory/image metrics regress by an order of magnitude
  and the core differentiator disappears.
- **Sibling JVM project sharing the chaos API**: Preferred if JVM coverage is ever
  pursued. Keeps this repo's identity intact while reaching the Java ecosystem.
- **WireMock API compatibility (already planned as v1.1 work)**: The existing
  roadmap answer to JVM-ecosystem reach — achieve interoperability on the API
  surface without adopting the JVM runtime.
