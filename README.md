# Codex Code Reviewer

A local-first Codex code review CLI with a read-only reviewer subagent, Docker
runtime support, and an optional Kubernetes-backed review service.

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

Build the CLI and install the reviewer for your user account:

```bash
make build
bin/codex-reviewer setup
```

`setup` installs the reviewer agent and updates:

```text
~/.codex/config.toml
```

From the repository you want to review, run a full local review:

```bash
bin/codex-reviewer review local
```

The report is written to:

```text
codex-review/full-review.md
```

For a branch review against `origin/main`:

```bash
bin/codex-reviewer review local --base origin/main --report codex-review/branch-review.md
```

To run the same app in an isolated local Docker container, build the image:

```bash
make docker-build-runner
```

Then export credentials:

```bash
export OPENAI_API_KEY=...
export GITHUB_TOKEN=...
```

And run the review with the local image:

```bash
docker run --rm \
  --user "$(id -u):$(id -g)" \
  -e OPENAI_API_KEY \
  -e GITHUB_TOKEN \
  -v "$PWD:/workspace" \
  -w /workspace \
  codex-reviewer:phase1 \
  codex-reviewer review local
```

The Docker command also writes:

```text
codex-review/full-review.md
```

## Manual Codex Session

After running `codex-reviewer setup`, the `code_reviewer` subagent is available
to normal Codex sessions. From a repository, start Codex:

```bash
codex
```

For a full repository review, paste:

```text
Do a full code review of this repository. Review the entire codebase, not just the current diff. Use the code_reviewer subagent if available. Focus on correctness, security/privacy risks, missing tests, Docker/GHCR workflow problems, installer behavior, CLI behavior, maintainability, and documentation gaps. Do not edit files. Return prioritized findings with file references, severity, why each issue matters, and suggested fixes. If there are no blocking issues, say that clearly and list the main areas checked.
```

For a branch review, paste:

```text
Review this branch against origin/main. Use the code_reviewer subagent if available. Inspect the diff and relevant surrounding code. Focus on correctness, security/privacy, regressions, missing tests, Docker/GHCR workflow problems, installer behavior, CLI behavior, maintainability, and documentation gaps. Do not edit files. Return prioritized findings with file references and suggested fixes.
```

## Global install

To make the reviewer available across repositories:

```bash
codex-reviewer setup
```

The setup command installs the agent and merges missing reviewer settings into
`~/.codex/config.toml` after showing the planned changes and asking for
confirmation. Use `codex-reviewer setup --yes` for unattended setup.

The default backend is local:

```toml
[codex_reviewer]
backend = "local"
report = "codex-review/full-review.md"
k8s_api_url = ""
```

To use a Kubernetes-backed review API, set `backend = "k8s"` and configure
`k8s_api_url`, then run `codex-reviewer service submit`.

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

docker run --rm \
  --user "$(id -u):$(id -g)" \
  -e OPENAI_API_KEY \
  -e GITHUB_TOKEN \
  -v "$PWD:/workspace" \
  -w /workspace \
  "$GHCR_IMAGE:$GHCR_TAG" \
  codex-reviewer review local
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
