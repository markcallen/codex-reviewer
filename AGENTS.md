# AGENTS.md

## Project review expectations

- Follow `docs/code_review.md` for code reviews.
- Reviews should prioritize correctness, security, regressions, missing tests, and maintainability.
- Avoid style-only feedback unless it hides a defect or contradicts this repository's formatter/linter rules.
- For implementation tasks, run the smallest relevant test, lint, or type-check command before reporting completion.
- Never include secrets in prompts, commits, logs, or generated documentation.

## Review guidelines

- Flag P0/P1 issues only when there is a concrete failure mode or credible risk.
- Treat missing tests as P1 when the change affects behavior, auth, billing, persistence, migrations, concurrency, permissions, or user-visible output.
- Treat documentation gaps as P1 only when the change alters setup, public APIs, release/deploy steps, or user-visible behavior.
- Use P3/Nit only for polish, and never block merge on personal preference.
