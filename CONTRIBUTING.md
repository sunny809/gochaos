# Contributing to gmock

Thanks for your interest in contributing to gmock!

## Getting Started

```bash
git clone https://github.com/sunny809/gochaos
cd gmock
go test -race ./...
```

Requires Go 1.22 or newer (we use the enhanced `net/http.ServeMux` pattern matching).

## Project Structure

The codebase follows the [golang-standards/project-layout](https://github.com/golang-standards/project-layout) conventions:

- `pkg/gmock/` — Public API (stable; backwards-compatibility matters)
- `internal/` — Implementation details (not importable by external modules)
- `cmd/gmock/` — CLI binary (thin wrapper over the library)
- `config/` — Stub file loading
- `test/integration/` — End-to-end tests
- `testdata/` — Test fixture files (Go's `go test` skips this directory)

## Development Conventions

- **Library first**: All domain logic lives in `internal/` and `pkg/gmock/`. The CLI is just a thin wrapper.
- **Functional options pattern**: Configure things with `gmock.WithFoo(x)`, not by mutating fields.
- **Concurrent-safe**: All shared state uses `sync.RWMutex`. Run `go test -race ./...` before committing.
- **Minimal dependencies**: The core library should remain near-zero dependencies. CLI-only or optional features may add deps.
- **Structured logging**: Use `log/slog`, not `fmt.Println` (CLI output is the exception).
- **Error wrapping**: `fmt.Errorf("context: %w", err)` rather than swallowing.
- **No `init()` functions** in business logic.

## Testing

- Unit tests live next to the code they test (e.g., `internal/matcher/matcher_test.go`).
- Integration tests live in `test/integration/`.
- Use table-driven tests with `t.Run` subtests for matchers.
- All tests must pass with the race detector: `go test -race ./...`.

## Adding a Matcher

The matching system is the most extension-friendly area. To add a new matcher:

1. Create `internal/matcher/<name>.go` implementing the `Matcher` interface:
   ```go
   type Matcher interface {
       Match(req *http.Request) bool
       ScoreMatch(req *http.Request) (matched bool, score int)
       String() string
   }
   ```
2. Add it to `internal/stub/matching.go` `BuildMatcher` function.
3. Extend `internal/spec/spec.go` `RequestPattern` if you need new config fields.
4. Add table-driven tests in `internal/matcher/<name>_test.go`.

## Adding an Admin Endpoint

1. Add a handler method in `internal/admin/<area>.go`.
2. Wire it up in `internal/admin/handler.go` `ServeHTTP`.
3. Add an integration test in `test/integration/server_test.go`.

## Build Slices

Development is organized in incremental "build slices" for fast feedback. PRs should target a single slice or fix.

## Reporting Issues

Please include:

- gmock version
- Go version (`go version`)
- Minimal reproducer
- Expected vs. actual behavior

## License

By contributing, you agree your contributions are licensed under Apache 2.0.

---

## GitHub Conventions

### Issue Labels

| Label | Color | Use For |
|-------|-------|---------|
| `bug` | #d73a4a | Something is broken |
| `enhancement` | #a2eeef | New feature or request |
| `documentation` | #0075ca | Docs improvements |
| `question` | #d876e3 | Support or clarification |
| `good first issue` | #7057ff | Friendly for new contributors |
| `help wanted` | #008672 | Community help needed |
| `dependencies` | #0366d6 | Dependabot PRs |

### Branch Naming

- `feature/<slice>-<short-desc>` — e.g., `feature/6-templating`
- `fix/<issue>-<short-desc>` — e.g., `fix/42-race-condition`
- `docs/<short-desc>` — Documentation updates

### PR Process

1. Open PR from feature branch to `main`
2. Ensure CI passes (tests, lint, build)
3. Request review from `@sunny809`
4. Address feedback
5. Squash-merge with descriptive message

### Release Process

1. Update `CHANGELOG.md` (if maintained)
2. Tag with semantic version: `git tag v1.2.3`
3. Push tag: `git push origin v1.2.3`
4. GoReleaser workflow builds and drafts release
5. Review draft release notes, publish

### Security

- Report security issues privately via GitHub Security Advisories
- Do not open public issues for vulnerabilities
