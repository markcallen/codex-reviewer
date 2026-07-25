# Codex code reviewer subagent setup

This repository contains a project-scoped Codex reviewer setup:

- `.codex/config.toml` — project Codex defaults, subagent limits, and `/review` model override.
- `.codex-reviewer.toml` — the installed `codex-reviewer` version and pre-push review defaults.
- `.codex/agents/code-reviewer.toml` — the custom read-only `code_reviewer` subagent.
- `AGENTS.md` — repository-level review guidance Codex reads before work.
- `docs/code_review.md` — team review checklist referenced by `AGENTS.md`.
- `prompts/` — copy/paste prompts for branch, PR, commit, and uncommitted-change reviews.
- `cmd/codex-reviewer/` — a self-contained Go CLI with embedded artifacts.
- `scripts/` — install helpers for project-scoped or global setup.

## 1. Install Codex CLI

macOS or Linux:

```bash
curl -fsSL https://chatgpt.com/codex/install.sh | sh
codex --version
```

Windows PowerShell:

```powershell
powershell -ExecutionPolicy ByPass -c "irm https://chatgpt.com/codex/install.ps1 | iex"
codex --version
```

Alternative package managers:

```bash
npm install -g @openai/codex
# or
brew install --cask codex
```

## 2. Sign in

Run:

```bash
codex
```

Choose ChatGPT sign-in for subscription access, or API-key sign-in for usage-based access.

## 3. Build the project CLI

From this repository:

```bash
make build
```

The resulting binary contains the reviewer agent, Codex config, review checklist, prompts, and repository guidance.

`make build` injects the version from `git describe --tags --always --dirty`. To build a release with an explicit tag:

```bash
make build VERSION=v1.0.0
bin/codex-reviewer version
```

## 4. Install into one project

Preview the changes first:

```bash
bin/codex-reviewer install --dry-run /path/to/your/repo
```

From this repository:

```bash
bin/codex-reviewer install /path/to/your/repo
```

Then from your target repo:

```bash
git add .codex .codex-reviewer.toml AGENTS.md docs/code_review.md prompts/
git commit -m "Add Codex code reviewer subagent"
```

When Codex opens the project, trust the project if you want `.codex/` project configuration to load.

The installer is non-destructive. It creates missing files, merges missing review settings into an existing `.codex/config.toml`, appends marked review sections to existing `AGENTS.md` and `docs/code_review.md`, and leaves conflicting agent or prompt files unchanged with a warning.

Verify the setup:

```bash
bin/codex-reviewer doctor /path/to/your/repo
```

The installed `.codex-reviewer.toml` records the CLI version used for the
install. `codex-reviewer doctor` and `codex-reviewer review pre-push` fail when
that version does not match the running binary, so hook runners do not silently
use a different reviewer than the one committed with the repository. Rerunning
`codex-reviewer install` with a newer binary refreshes the recorded version
without replacing the pre-push settings.

## 5. Legacy shell installer

The older shell installer still exists for simple copy-only installs:

```bash
./scripts/install-project.sh /path/to/your/repo
```

Prefer the Go installer for repositories that already have Codex or agent guidance configured.

## 6. Global install option

To use the reviewer in every repo without committing project files:

```bash
./scripts/install-global.sh
```

This copies the reviewer agent to `~/.codex/agents/code-reviewer.toml` and prints the config block to add to `~/.codex/config.toml`.

## 7. Run it

Interactive:

```bash
codex
```

Then paste one of the prompts from `prompts/`.

Non-interactive branch review:

```bash
codex exec "Review this branch against main. Spawn the code_reviewer subagent, wait for it, and report prioritized findings with file references. Do not edit files."
```

Pre-push review command for pre-commit, Husky, or another hook runner:

```bash
codex-reviewer review pre-push
```

The command does not install or manage hooks. It reads `.codex-reviewer.toml`,
checks that the installed config version matches the running binary, requires a
clean working tree by default, runs `codex exec review`, and writes the report to
`.git/codex-review/pre-push-review.md` unless configured otherwise.

Built-in local reviewer:

```text
/review
```

The `review_model = "gpt-5.5"` setting makes `/review` use GPT-5.5 even if your current session model is different.

## 8. Optional GitHub usage

For GitHub PR reviews through Codex cloud, set up Codex cloud for the repository, enable Code review in Codex settings, and add review guidance to `AGENTS.md`. You can request a review in a PR comment with:

```text
@codex review for security regressions, missing tests, and risky behavior changes.
```
