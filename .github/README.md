# GitHub Conventions

This document describes conventions specific to the GitHub workflow for `gmock`.

## Issue Labels

| Label | Color | Use For |
|-------|-------|---------|
| `bug` | #d73a4a | Something is broken |
| `enhancement` | #a2eeef | New feature or request |
| `documentation` | #0075ca | Docs improvements |
| `question` | #d876e3 | Support or clarification |
| `good first issue` | #7057ff | Friendly for new contributors |
| `help wanted` | #008672 | Community help needed |
| `dependencies` | #0366d6 | Dependabot PRs |

## Branch Naming

- `feature/<slice>-<short-desc>` — e.g., `feature/6-templating`
- `fix/<issue>-<short-desc>` — e.g., `fix/42-race-condition`
- `docs/<short-desc>` — Documentation updates

## PR Process

1. Open PR from feature branch to `main`
2. Ensure CI passes (tests, lint, build)
3. Request review from `@sunny809`
4. Address feedback
5. Squash-merge with descriptive message

## Release Process

1. Update `CHANGELOG.md` (if maintained)
2. Tag with semantic version: `git tag v1.2.3`
3. Push tag: `git push origin v1.2.3`
4. GoReleaser workflow builds and drafts release
5. Review draft release notes, publish

## Security

- Report security issues privately via GitHub Security Advisories
- Do not open public issues for vulnerabilities
