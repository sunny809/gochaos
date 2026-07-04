# Contributing to gochaos

Thanks for your interest in contributing to gochaos!

> **Note**: The project is named `gochaos` (repository/module name), while `gmock` is the CLI binary name. The Go package is imported as `github.com/sunny809/gochaos/pkg/gmock`.

## Quick Start

```bash
# Clone and build
git clone https://github.com/sunny809/gochaos
cd gochaos
go build ./...

# Run all tests with race detector (required before every commit)
go test -race ./...

# Static analysis
go vet ./...

# Build the CLI binary
go build -o /tmp/gmock ./cmd/gmock
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

For the full architecture and coding conventions, see [`CLAUDE.md`](CLAUDE.md) — it is the source of truth for architecture decisions, import rules, and gotchas.

## Development Conventions

- **Library first**: All domain logic lives in `internal/` and `pkg/gmock/`. The CLI is just a thin wrapper.
- **Functional options pattern**: Configure things with `gmock.WithFoo(x)`, not by mutating fields.
- **Concurrent-safe**: All shared state uses `sync.RWMutex`. Run `go test -race ./...` before committing.
- **Minimal dependencies**: The core library should remain near-zero dependencies. CLI-only or optional features may add deps.
- **Structured logging**: Use `log/slog`, not `fmt.Println` (CLI output is the exception).
- **Error wrapping**: `fmt.Errorf("context: %w", err)` rather than swallowing.
- **No `init()` functions** in business logic.
- **`text/template` not `html/template`** for response templating (avoids HTML escaping JSON).
- **Restore `req.Body`**: After reading request body in matchers, restore with `io.NopCloser(strings.NewReader(...))`.

## Sprint Workflow

Development is organized in sprints with defined roles. Each sprint follows:

```
Kickoff (PO) → Design (Tech Lead) → Develop (Developer) → Test (QA) → Review (Tech Lead) → Release (PO+SM)
```

See [`docs/sprints/INDEX.md`](docs/sprints/INDEX.md) for the full timeline and sprint status.

PRs should target a single slice, story, or fix. Large cross-cutting changes should be discussed in an issue first.

## Commit Messages

Use [Conventional Commits](https://www.conventionalcommits.org/) format:

```
<type>: <short summary>

<body with context>
```

| Type | Use For |
|------|---------|
| `feat:` | New feature |
| `fix:` | Bug fix |
| `test:` | Adding or updating tests |
| `docs:` | Documentation changes |
| `chore:` | Build, CI, tooling, dependencies |
| `refactor:` | Code restructuring without behavior change |

Examples:

```
feat: add JSONPath body matcher with pre-compilation
fix: restore request body after reading in header matcher
test: add table-driven tests for lognormal delay distribution
docs: update admin API reference for callback endpoints
chore: bump GoReleaser to v2
refactor: extract SSRF check into internal/callback package
```

## Testing

- Unit tests live next to the code they test (e.g., `internal/matcher/matcher_test.go`).
- Integration tests live in `test/integration/`.
- Use table-driven tests with `t.Run` subtests for matchers.
- All tests must pass with the race detector: `go test -race ./...`.
- Matchers return `(bool, int)` — match result and score. Cover both dimensions in tests.

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
5. Pre-compile regex patterns and JSONPath expressions at construction time, not per-request.

## Adding an Admin Endpoint

1. Add a handler method in `internal/admin/<area>.go`.
2. Wire it up in `internal/admin/handler.go` `ServeHTTP`.
3. Add an integration test in `test/integration/server_test.go`.

## Reporting Issues

Please include:

- gmock version (`gmock version` or commit hash)
- Go version (`go version`)
- OS and architecture
- Minimal reproducer (code snippet or YAML config)
- Expected vs. actual behavior

See our [issue templates](.github/ISSUE_TEMPLATE/) for structured forms.

## License

By contributing, you agree your contributions are licensed under Apache 2.0.

---

## Label Taxonomy

### Area Labels

| Label | Use For |
|-------|---------|
| `area/core` | Stub registry, matching engine, server lifecycle |
| `area/admin` | Admin REST API endpoints |
| `area/chaos` | Fault injection, delays, activation modes, callbacks |
| `area/cli` | CLI binary, cobra commands |
| `area/docs` | Documentation, README, feature guides |
| `area/infra` | CI/CD, Docker, GoReleaser, GitHub workflows |

### Type Labels

| Label | Use For |
|-------|---------|
| `bug` | Something is broken |
| `enhancement` | New feature or request |
| `documentation` | Docs improvements |
| `question` | Support or clarification |
| `good first issue` | Friendly for new contributors |
| `help wanted` | Community help needed |
| `dependencies` | Dependabot PRs |
| `wontfix` | Will not be fixed |

### Branch Naming

- `feature/<slice>-<short-desc>` — e.g., `feature/6-templating`
- `fix/<issue>-<short-desc>` — e.g., `fix/42-race-condition`
- `docs/<short-desc>` — Documentation updates

## PR Process

1. Open PR from feature branch to `main`
2. Ensure CI passes (tests, lint, build)
3. Request review from `@sunny809`
4. Address feedback
5. Squash-merge with a conventional commit message

## Release Process

1. Update `CHANGELOG.md` (if maintained)
2. Tag with semantic version: `git tag v1.2.3`
3. Push tag: `git push origin v1.2.3`
4. GoReleaser workflow builds and drafts release
5. Review draft release notes, publish

## Security

- Report security issues privately via [GitHub Security Advisories](https://github.com/sunny809/gochaos/security/advisories)
- Do not open public issues for vulnerabilities
- See [SECURITY.md](SECURITY.md) for the full security policy
