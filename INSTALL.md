# Codex code reviewer subagent setup

This repository contains a project-scoped Codex reviewer setup:

- `.codex/config.toml` — project Codex defaults, subagent limits, and `/review` model override.
- `.codex/agents/code-reviewer.toml` — the custom read-only `code_reviewer` subagent.
- `AGENTS.md` — repository-level review guidance Codex reads before work.
- `docs/code_review.md` — team review checklist referenced by `AGENTS.md`.
- `prompts/` — copy/paste prompts for branch, PR, commit, and uncommitted-change reviews.
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

## 3. Install into one project

From this repository:

```bash
./scripts/install-project.sh /path/to/your/repo
```

Then from your target repo:

```bash
git add .codex AGENTS.md docs/code_review.md prompts/
git commit -m "Add Codex code reviewer subagent"
```

When Codex opens the project, trust the project if you want `.codex/` project configuration to load.

## 4. Global install option

To use the reviewer in every repo without committing project files:

```bash
./scripts/install-global.sh
```

This copies the reviewer agent to `~/.codex/agents/code-reviewer.toml` and prints the config block to add to `~/.codex/config.toml`.

## 5. Run it

Interactive:

```bash
codex
```

Then paste one of the prompts from `prompts/`.

Non-interactive branch review:

```bash
codex exec "Review this branch against main. Spawn the code_reviewer subagent, wait for it, and report prioritized findings with file references. Do not edit files."
```

Built-in local reviewer:

```text
/review
```

The `review_model = "gpt-5.5"` setting makes `/review` use GPT-5.5 even if your current session model is different.

## 6. Optional GitHub usage

For GitHub PR reviews through Codex cloud, set up Codex cloud for the repository, enable Code review in Codex settings, and add review guidance to `AGENTS.md`. You can request a review in a PR comment with:

```text
@codex review for security regressions, missing tests, and risky behavior changes.
```
