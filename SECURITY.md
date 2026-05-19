# Security policy

## Reporting a vulnerability

Please **do not** open a public GitHub issue for security-sensitive reports.

Instead, email security reports to `<your-security-email@arafa-dev.com>`
(update before launch) with the following information:

- A description of the issue
- Steps to reproduce
- Impact assessment
- Any suggested mitigation

We aim to acknowledge reports within 48 hours.

## Scope

ccx is a local-only tool. The dashboard HTTP server binds to 127.0.0.1 only.
The most relevant attack surfaces are:

- Local file inclusion via malformed profile paths
- Code execution via crafted JSONL fixtures
- Privilege escalation via misuse of credentials owned by the active profile
- Credential exfiltration through telemetry (we don't have telemetry in v0.1,
  but the constraint is documented for future versions)

## Out of scope

- Bugs in upstream `claude` itself — report those to Anthropic
- Issues that require local root or physical machine access
- Denial-of-service against the local dashboard (it's local)
