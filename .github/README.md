# GitHub Community Health

This directory contains GitHub-specific community and workflow files.

## Documentation

- **Project README**: See the [root README.md](../README.md) for the full project overview, features, installation, and usage.
- **Contributing guide**: See [CONTRIBUTING.md](../CONTRIBUTING.md) for development workflow, commit conventions, and PR process.
- **Security policy**: See [SECURITY.md](../SECURITY.md) for vulnerability reporting and supported versions.

## Issue Templates

Located in [ISSUE_TEMPLATE/](ISSUE_TEMPLATE/):

- **Bug Report** — includes version, environment, persona, and reproducer fields
- **Feature Request** — includes problem statement, proposed solution, alternatives, and persona
- **Question** — for usage and configuration questions

Blank issues are disabled. Use [Discussions](https://github.com/sunny809/gochaos/discussions) for open-ended questions.

## PR Template

See [pull_request_template.md](pull_request_template.md) — includes type of change, breaking changes, backwards compatibility note, and testing checklist.

## CI Workflows

| Workflow | Trigger | Purpose |
|----------|---------|---------|
| [ci.yml](workflows/ci.yml) | Push/PR to main | Build, test, vet |
| [codeql.yml](workflows/codeql.yml) | Push/PR + weekly | Static security analysis |
| [vulncheck.yml](workflows/vulncheck.yml) | Weekly + manual | Go vulnerability check |
| [license.yml](workflows/license.yml) | Push/PR to main | License header validation |
| [release.yml](workflows/release.yml) | Tag push | GoReleaser + Docker image |

## Repository Topics

Apply these topics via `gh repo edit`:

```bash
gh repo edit --add-topic go,mock-server,chaos-engineering,testing,http,resilience,wiremock-alternative
```

## FUNDING.yml

Not adding at this time. There is no sponsorship infrastructure set up. If sponsorship becomes relevant in the future, add `.github/FUNDING.yml` with the appropriate platform.
