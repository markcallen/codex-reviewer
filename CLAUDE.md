# CLAUDE.md

This file provides guidance to Claude Code for working in this repository.

## Repository Facts

Use this section for durable repo-specific facts that agents repeatedly need. Prefer facts stored here over re-deriving them with shell commands on every task.

Keep only stable, reviewable metadata here. Do not store secrets, credentials, or ephemeral runtime state.

Suggested facts to record:

- Canonical GitHub repo: `markcallen/codex-reviewer`
- Default branch: `<main>`
- Primary package manager: `go`
- Version-file locations agents should check first: `go.mod`
- Canonical config files: `go.mod`
- Primary CI workflows: `<workflow filenames>`
- Primary release/publish workflows: `<workflow filenames>`
- Preferred build/test/lint/format/coverage commands: `make test, make lint, make build`
- Coverage threshold: `<value>`
- Generated or protected paths agents should avoid editing directly: `coverage/, .ballast/`

Update this section when those facts change. If live runtime state is required, discover it separately instead of treating it as a durable repo fact.

## Installed agent rules

Created by [Ballast](https://github.com/everydaydevopsio/ballast) v5.12.0. Do not edit this section.

Read and follow these rule files in `.claude/rules/` when they apply:

- `.claude/rules/local-dev-badges.md` — Add standard badges (CI, Release, License, GitHub Release, npm) to the top of README.md
- `.claude/rules/local-dev-env.md` — Local development environment specialist - reproducible dev setup, DX, and documentation
- `.claude/rules/local-dev-license.md` — License setup - ensure LICENSE file, package.json license field, and README reference (default MIT; overridable in AGENTS.md/CLAUDE.md)
- `.claude/rules/local-dev-mcp.md` — Optional: use GitHub MCP and issues MCP (Jira/Linear/GitHub) for local-dev context
- `.claude/rules/docs.md` — Documentation specialist - GitHub Markdown docs by default, or maintain existing Docusaurus sites with publish-docs automation
- `.claude/rules/cicd.md` — CI/CD specialist - pipeline design, quality gates, and deployment
- `.claude/rules/observability.md` — Observability specialist - logging, tracing, metrics, and SLOs
- `.claude/rules/publishing-api.md` — REST API publishing specialist - Docker or platform service CD with runtime health checks
- `.claude/rules/publishing-apps.md` — App publishing specialist - npmjs for Node apps, PyPI for Python apps, GitHub Releases for Go apps
- `.claude/rules/publishing-apt.md` — APT/deb package publishing specialist - GoReleaser nfpms and GitHub Releases
- `.claude/rules/publishing-brew.md` — Homebrew tap publishing specialist - GoReleaser brews block and tap repo setup
- `.claude/rules/publishing-cli.md` — CLI publishing specialist - GoReleaser for Go, npmjs for Node, PyPI for Python
- `.claude/rules/publishing-libraries.md` — Library publishing specialist - npmjs for TypeScript, PyPI for Python, GitHub tags/releases for Go
- `.claude/rules/publishing-sdks.md` — SDK publishing specialist - npmjs for TypeScript SDKs, PyPI for Python SDKs, GitHub tags/releases for Go SDKs
- `.claude/rules/publishing-web.md` — Web app publishing specialist - Docker or platform app CD on push to main
- `.claude/rules/git-hooks.md` — Git hook specialist - configure commit and push hooks that match the repository layout
- `.claude/rules/tasks-task-system.md` — Task system integration - use GitHub Issues for work items and configure the MCP server
- `.claude/rules/tasks-todo.md` — Branch-local TODO tracking - manage tasks/TODO.md and triage before PR
- `.claude/rules/go-linting.md` — Go linting specialist for gofmt and golangci-lint setup
- `.claude/rules/go-logging.md` — Go logging specialist for structured logging patterns
- `.claude/rules/go-testing.md` — Go testing specialist for go test and coverage setup
