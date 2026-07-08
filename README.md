# Codex Code Reviewer Subagent

A Git-ready Codex configuration template for a read-only code reviewer subagent.

The reviewer focuses on:

- correctness bugs
- security and privacy regressions
- API or behavior regressions
- risky migrations and persistence changes
- concurrency and async hazards
- missing or weak tests
- maintainability issues that create future defects

It avoids style-only comments unless style hides a real bug.

## What is included

```text
.
├── .codex/
│   ├── config.toml
│   └── agents/
│       └── code-reviewer.toml
├── AGENTS.md
├── INSTALL.md
├── README.md
├── docs/
│   ├── code_review.md
│   └── sources.md
├── prompts/
│   ├── review-branch.md
│   ├── review-commit.md
│   ├── review-pr.md
│   └── review-uncommitted.md
└── scripts/
    ├── install-global.sh
    └── install-project.sh
```

## Quick start

Install this setup into a target repository:

```bash
./scripts/install-project.sh /path/to/your/repo
```

Then commit the files in the target repository:

```bash
cd /path/to/your/repo
git add .codex AGENTS.md docs/code_review.md prompts/
git commit -m "Add Codex code reviewer subagent"
```

Open Codex in that repo:

```bash
codex
```

Then paste:

```text
Review this branch against main. Spawn the code_reviewer subagent, have it inspect the diff and relevant surrounding code in read-only mode, wait for it to finish, then summarize prioritized findings with file references and suggested fixes. Focus on correctness, security/privacy, regressions, missing tests, and maintainability. Do not edit files.
```

## Global install

To make the reviewer available across repositories:

```bash
./scripts/install-global.sh
```

Then add the printed config block to `~/.codex/config.toml`.

## Model choice

The reviewer uses:

```toml
model = "gpt-5.5"
model_reasoning_effort = "high"
sandbox_mode = "read-only"
```

Use a smaller/faster model only when you want lightweight review. For high-signal reviews before merge, keep the default.

## Built-in `/review`

This repo also sets:

```toml
review_model = "gpt-5.5"
```

That makes Codex's built-in `/review` command use the same high-capability model.

## Customize severity rules

Edit `docs/code_review.md` and `AGENTS.md` for your team's standards. Keep the reviewer strict about P0/P1 issues and forgiving about nits. That balance keeps reviews useful instead of noisy.
