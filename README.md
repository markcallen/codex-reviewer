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

## Quick start

Build the self-contained CLI:

```bash
make build
```

Preview the install into a target repository:

```bash
bin/codex-reviewer install --dry-run /path/to/your/repo
```

Install this setup into the target repository:

```bash
bin/codex-reviewer install /path/to/your/repo
```

Check whether a repository is set up correctly:

```bash
bin/codex-reviewer doctor /path/to/your/repo
```

Then commit the files in the target repository:

```bash
cd /path/to/your/repo
git add .codex .codex-reviewer.toml AGENTS.md docs/code_review.md prompts/
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

## Docker and GHCR

For a local container workflow without kind, build the reviewer image:

```bash
make docker-build-runner
```

Publish it to GHCR:

```bash
export GHCR_IMAGE=ghcr.io/<owner>/codex-code-reviewer
export GHCR_TAG=v0.1.0
echo "$GITHUB_TOKEN" | docker login ghcr.io -u <github-username> --password-stdin
make docker-push-runner GHCR_IMAGE="$GHCR_IMAGE" GHCR_TAG="$GHCR_TAG"
```

Run a review from any checked-out repository:

```bash
export OPENAI_API_KEY=...
export GITHUB_TOKEN=...
make docker-run-review DOCKER_RUN_IMAGE="$GHCR_IMAGE:$GHCR_TAG"
```

See `docs/docker-ghcr.md` for the raw Docker commands and options.

## Installer behavior

The Go CLI embeds all artifacts required for project installation, so the built binary can be copied and run without this source checkout.

It is intentionally non-destructive:

- Missing files are created.
- `.codex-reviewer.toml` is created on first install with the installed CLI version and pre-push review defaults; later installs refresh only the recorded version.
- Existing `.codex/config.toml` files are merged with only the missing review-related settings.
- Existing `AGENTS.md` and `docs/code_review.md` files are extended with marked managed sections.
- Existing agent and prompt files that differ from the bundled artifacts are kept unchanged and reported as warnings.
- Running the installer repeatedly is idempotent.

Build versions are injected with Go linker flags. By default, `make build` uses `git describe --tags --always --dirty`; release builds can pass an explicit tag:

```bash
make build VERSION=v1.0.0
bin/codex-reviewer version
```

## Pre-push reviews

Use your existing hook runner, such as pre-commit or Husky, to call:

```bash
codex-reviewer review pre-push
```

The command reads `.codex-reviewer.toml`, verifies the installed version matches
the running `codex-reviewer` binary, requires a clean working tree by default,
runs `codex exec review`, and writes the report under `.git/codex-review/`.

For pre-commit:

```yaml
repos:
  - repo: local
    hooks:
      - id: codex-reviewer-pre-push
        name: Codex AI code review
        entry: codex-reviewer review pre-push
        language: system
        stages: [pre-push]
        pass_filenames: false
```

For Husky:

```sh
#!/usr/bin/env sh
. "$(dirname "$0")/_/husky.sh"

codex-reviewer review pre-push
```

## Model choice

The reviewer uses:

```toml
model = "gpt-5.5"
model_reasoning_effort = "high"
sandbox_mode = "read-only"
```

Use a smaller/faster model only when you want lightweight review. For high-signal reviews before merge, keep the default.

## Running tests during review without editing code

Some test runners need writable scratch space even when they do not modify source
files. Jest, for example, may write a haste map under `/tmp`. A pure
`sandbox_mode = "read-only"` reviewer cannot do that.

On Codex clients that support permission profiles, configure a reviewer profile
that can read the repository but write only to temporary directories:

```toml
default_permissions = "review-test"

[permissions.review-test.filesystem]
":minimal" = "read"
":workspace_roots" = "read"
":tmpdir" = "write"
":slash_tmp" = "write"

[permissions.review-test.network]
enabled = false
```

Permission profiles do not compose with the older sandbox settings. If you use
this profile, remove `sandbox_mode` and `[sandbox_workspace_write]` from the same
loaded config layer or start Codex without a `--sandbox` override.

For older Codex clients, the practical fallback is:

```toml
sandbox_mode = "workspace-write"
approval_policy = "on-request"

[sandbox_workspace_write]
exclude_tmpdir_env_var = false
exclude_slash_tmp = false
network_access = false
```

Keep the reviewer prompt instruction `Do not edit files` in place. This fallback
allows local commands such as Jest to create caches, but it relies on reviewer
instructions rather than filesystem policy to avoid source edits.

## Built-in `/review`

This repo also sets:

```toml
review_model = "gpt-5.5"
```

That makes Codex's built-in `/review` command use the same high-capability model.

## Customize severity rules

Edit `docs/code_review.md` and `AGENTS.md` for your team's standards. Keep the reviewer strict about P0/P1 issues and forgiving about nits. That balance keeps reviews useful instead of noisy.
