# Security Policy

## Supported Versions

| Version | Supported          |
| ------- | ------------------ |
| v1.0.x  | :white_check_mark: |
| main    | :white_check_mark: |

Security updates are applied to the latest release on the `main` branch. Pre-release versions (0.x) are no longer actively maintained.

## Reporting a Vulnerability

**Do NOT open a public issue for security vulnerabilities.**

Instead, please report security issues privately using one of these methods:

1. **GitHub Security Advisories** (preferred):
   - Go to https://github.com/sunny809/gochaos/security/advisories
   - Click "Report a vulnerability"
   - Fill in the details

2. **Email**:
   - Send details to the maintainer via GitHub profile

### What to include

- Description of the vulnerability
- Steps to reproduce
- Affected versions
- Potential impact
- Suggested fix (if any)

### Response timeline

- **Acknowledgment**: Within 48 hours
- **Initial assessment**: Within 5 business days
- **Fix**: Critical vulnerabilities will be patched as soon as possible; lower-severity issues are addressed in the next release
- **Disclosure**: After the fix is released, we will publish a GitHub Security Advisory with credit to the reporter

## Scope

### In Scope

- **gmock core library** (`pkg/gmock/`, `internal/`) — stub registry, matching engine, response pipeline
- **Admin API** (`internal/admin/`) — all `/__admin/*` endpoints
- **CLI binary** (`cmd/gmock/`) — command-line interface

### Out of Scope

- **Third-party dependencies** — report vulnerabilities upstream to the dependency maintainers
- **Companion repositories** — scenarios repo, Prometheus exporter repo, and other non-core repositories
- **Denial of service** via the admin API when exposed to the public internet (the admin API is designed for localhost use)

## Automated Security Scanning

The following CI workflows run automatically:

| Workflow | Schedule | Purpose |
|----------|----------|---------|
| [CodeQL](https://github.com/sunny809/gochaos/blob/main/.github/workflows/codeql.yml) | Every push/PR + weekly | Static analysis for Go security vulnerabilities |
| [govulncheck](https://github.com/sunny809/gochaos/blob/main/.github/workflows/vulncheck.yml) | Weekly + manual trigger | Go dependency vulnerability scanning |

## Security Best Practices

When using gochaos in production:

- **Don't expose admin API** to the public internet (bind to localhost or use firewall rules)
- **Use `--admin-port`** on a separate internal interface if needed
- **Validate stub configurations** from untrusted sources before loading
- **Review proxy configurations** if using `--proxy-url` mode
- **Review callback URLs** — callbacks are SSRF-protected (private IPs are blocked), but validate that callback targets are intentional

## Known Security Considerations

| Feature | Risk | Mitigation |
|---------|------|------------|
| Admin API | Unauthorized stub manipulation | Bind to localhost, use network policies |
| Proxy mode | SSRF via proxy URL | Only proxy to trusted upstreams |
| Callbacks | SSRF via callback URL | DNS-time SSRF guard blocks private/reserved IPs (loopback, link-local, RFC 1918, RFC 6598) |

---

Thank you for helping keep gochaos secure!
