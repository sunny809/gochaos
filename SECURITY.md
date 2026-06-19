# Security Policy

## Supported Versions

| Version | Supported          |
| ------- | ------------------ |
| 0.x     | :white_check_mark: |

gochaos is currently in pre-v1.0 development. All 0.x releases receive security updates.

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

- **Initial response**: Within 48 hours
- **Triage**: Within 7 days
- **Fix**: Critical vulnerabilities will be patched ASAP
- **Disclosure**: After fix is released, we'll publish a security advisory

## Security Best Practices

When using gochaos in production:

- **Don't expose admin API** to the public internet (bind to localhost or use firewall rules)
- **Use `--admin-port`** on a separate internal interface if needed
- **Validate stub configurations** from untrusted sources before loading
- **Review proxy configurations** if using `--proxy-url` mode

## Known Security Considerations

| Feature | Risk | Mitigation |
|---------|------|------------|
| Admin API | Unauthorized stub manipulation | Bind to localhost, use network policies |
| Proxy mode | SSRF via proxy URL | Only proxy to trusted upstreams |
| Callbacks (planned) | SSRF via callback URL | Allowlist configuration (Sprint 8) |

---

Thank you for helping keep gochaos secure!
