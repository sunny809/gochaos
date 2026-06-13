# ADR-006: text/template for Response Templating

## Status

Accepted

## Context

Slice 6 requires response body templating (e.g., `{{requestPath}}`, `{{randomUUID}}`). Go provides two template packages: `html/template` and `text/template`.

## Decision

Use `text/template` for response body templating. Explicitly avoid `html/template`.

## Consequences

**Positive:**
- `text/template` does not escape JSON content (correct for API mock responses)
- Standard library, no additional dependencies

**Negative:**
- Must be careful not to accidentally use `html/template` in future edits
- No built-in XSS protection (irrelevant for mock server use case)

## Alternatives Considered

- `html/template`: Rejected because it HTML-escapes JSON, producing invalid responses
- Custom template engine: Rejected — unnecessary complexity when `text/template` suffices
