# Code review checklist for Codex

Use this checklist when reviewing a PR, branch, commit, or uncommitted diff.

## What to check first

1. Does the change do what it claims to do?
2. Could it break an existing caller, API contract, schema, migration, feature flag, or permission boundary?
3. Are edge cases handled: empty input, null/missing data, retries, timeouts, pagination, concurrency, partial failure, and rollback?
4. Are there security or privacy risks: auth bypass, privilege escalation, injection, unsafe deserialization, SSRF, path traversal, secrets, PII logging, dependency risk, or unsafe redirects?
5. Are tests added or updated at the right level? Unit tests for pure logic, integration tests for service contracts, end-to-end tests for critical user flows.
6. Is the code simpler than the problem requires? Flag avoidable complexity, speculative abstraction, and wide blast radius.
7. Are docs, examples, generated types, API schemas, or migrations updated when user or developer behavior changes?

## Comment style

- Lead with the highest-severity findings.
- Explain the failure path and why it matters.
- Suggest a small fix or a targeted test.
- Avoid drive-by style comments. Let formatters and linters handle formatting.
- Say when no blocking issues were found.

## Severity labels

- **P0 Critical**: exploit, data loss, outage, credential/PII leak, or destructive migration risk.
- **P1 Must fix**: credible bug, regression, broken contract, race/deadlock, important missing test, or security weakness.
- **P2 Should fix**: maintainability, test quality, observability, or performance issue that raises material future risk.
- **P3 Nit**: polish only.
