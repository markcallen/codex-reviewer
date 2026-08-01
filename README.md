# Codex Reviewer

[![CI](https://github.com/markcallen/codex-reviewer/actions/workflows/ci.yml/badge.svg)](https://github.com/markcallen/codex-reviewer/actions/workflows/ci.yml)
[![Smoke](https://github.com/markcallen/codex-reviewer/actions/workflows/smoke.yml/badge.svg)](https://github.com/markcallen/codex-reviewer/actions/workflows/smoke.yml)
[![Release](https://github.com/markcallen/codex-reviewer/actions/workflows/publish-cli.yml/badge.svg)](https://github.com/markcallen/codex-reviewer/actions/workflows/publish-cli.yml)
[![Docker](https://github.com/markcallen/codex-reviewer/actions/workflows/publish-docker.yml/badge.svg)](https://github.com/markcallen/codex-reviewer/actions/workflows/publish-docker.yml)
[![License](https://img.shields.io/github/license/markcallen/codex-reviewer)](LICENSE)
[![GitHub Release](https://img.shields.io/github/v/release/markcallen/codex-reviewer)](https://github.com/markcallen/codex-reviewer/releases)

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
bin/codex-reviewer review local --base origin/main
```

Branch reviews default to `codex-review/branch-review.md` and require a concise
auditable report with a verdict, diff summary, areas checked, limits, findings,
and tests to run.

For a large or risky branch, require a structured report with subsystem
coverage and explicit review limits:

```bash
bin/codex-reviewer review local \
  --base origin/main \
  --structured
```

Structured review mode asks Codex to summarize the diff, list areas checked,
list areas not checked or not deeply verified, report all concrete P0-P2
findings, and include the smallest useful validation commands.

To get an advisory mode recommendation before spending tokens:

```bash
bin/codex-reviewer review recommend --base origin/main
```

The recommendation is local-only and deterministic. It summarizes changed files,
approximate diff size, simple risk signals, a recommended mode
(`native`, `structured`, `full`, or `pre-push`), reasons, and an approximate
token range. You can also use `review local --recommend` or
`review pre-push --recommend` to print the same advisory output from those
workflows without invoking Codex.

To run the same app in an isolated local Docker container, build the image:

```bash
make docker-build-runner
```

Then export credentials:

```bash
export CODEX_API_KEY=...
export GITHUB_TOKEN=...
```

And run the review with the local image:

```bash
docker run --rm \
  --user "$(id -u):$(id -g)" \
  -e CODEX_API_KEY \
  -e GITHUB_TOKEN \
  -v "$PWD:/workspace" \
  -w /workspace \
  codex-reviewer:phase1 \
  codex exec --sandbox danger-full-access \
    --output-last-message codex-review/full-review.md \
    "Do a full code review of this repository. Review the entire codebase, not just the current diff. Focus on correctness, security/privacy risks, missing tests, Docker/GHCR workflow problems, installer behavior, CLI behavior, maintainability, and documentation gaps. Do not edit files. Return prioritized findings with file references, severity, why each issue matters, and suggested fixes."
```

The Docker command also writes:

```text
codex-review/full-review.md
```

If you want to build the local CLI but run reviews with the published GHCR
image instead of building a local Docker image:

```bash
make build
export CODEX_API_KEY=...
export GITHUB_TOKEN=...
bin/codex-reviewer review docker
```

For a private GHCR package, run `docker login ghcr.io` first with a GitHub token
that has `read:packages` access.

`review docker` checks that `CODEX_API_KEY` and `GITHUB_TOKEN` are set before
starting Docker, then passes both through with Docker `-e` flags. When using
`env-secrets`, store the Codex/OpenAI API key as `CODEX_API_KEY`.

Because Docker is already the isolation boundary, `review docker` runs the
inner Codex process with `--sandbox danger-full-access`. This avoids bubblewrap
namespace failures on hosts that do not allow unprivileged user namespaces
inside containers.

`review docker` uses the release tag from the running `codex-reviewer` binary
version. For example, `v0.1.0` and `v0.1.0-7-g8fd747e-dirty` both run
`ghcr.io/markcallen/codex-reviewer:v0.1.0`. Development builds with version
`dev` fall back to `latest`.

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

`setup` records the `codex-reviewer` version that created
`~/.codex/agents/code-reviewer.toml` and validates the resulting Codex config
with `codex --strict-config --help`. Non-dry review commands check that marker
before running and ask you to rerun `codex-reviewer setup` when the installed
global reviewer was created by a different CLI version.

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
export GHCR_IMAGE=ghcr.io/markcallen/codex-reviewer
export GHCR_TAG=v0.1.0
echo "$GITHUB_TOKEN" | docker login ghcr.io -u <github-username> --password-stdin
make docker-push-runner GHCR_IMAGE="$GHCR_IMAGE" GHCR_TAG="$GHCR_TAG"
```

Run a review from any checked-out repository:

```bash
export CODEX_API_KEY=...
export GITHUB_TOKEN=...
codex-reviewer review docker
```

For a private GHCR package, log in first:

```bash
echo "$GITHUB_TOKEN" | docker login ghcr.io -u <github-username> --password-stdin
```

Or run Docker directly:

```bash
export CODEX_API_KEY=...
export GITHUB_TOKEN=...

docker run --rm \
  --user "$(id -u):$(id -g)" \
  -e CODEX_API_KEY \
  -e GITHUB_TOKEN \
  -v "$PWD:/workspace" \
  -w /workspace \
  ghcr.io/markcallen/codex-reviewer:latest \
  codex exec --sandbox danger-full-access review \
    --base origin/main \
    --output-last-message codex-review/branch-review.md
```

See `docs/docker-ghcr.md` for the raw Docker commands and options.

## Installer behavior

The Go CLI embeds all artifacts required for project installation, so the built binary can be copied and run without this source checkout.

It is intentionally non-destructive:

- Missing files are created.
- `.codex-reviewer.toml` is created on first install with the installed CLI version, repo review defaults, and pre-push review defaults; later installs refresh only the recorded version.
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
runs a prompt-based branch review, and writes the report under
`.git/codex-review/`.

## Repo review config

`.codex-reviewer.toml` can set shared review defaults:

```toml
version = "v1.2.3"

[review]
base = "origin/main"
ignore = ["dist/**", "*.lock"]
directives = [
  "Check public API compatibility.",
  "Treat missing behavior tests as a blocking finding.",
]
profile = "pr-readiness"
policy_file = "docs/review-policy.md"

[review.pre_push]
base = ""
block_on = "block"
report = ".git/codex-review/pre-push-review.md"
require_clean_tree = true
```

Explicit CLI flags win over config. For example, `--base release/1.2` overrides
`[review].base`, and `--profile standard` overrides `[review].profile`.
When neither is set, branch review base falls back to upstream, `origin/main`,
`origin/master`, then `main`.

`ignore` globs are applied to local and pre-push branch diff prompts with Git
pathspec excludes before Codex is invoked, for example
`git diff origin/main...HEAD -- . ':(exclude)dist/**'`. For full-repository
reviews the ignore list is advisory because Codex can still inspect the
workspace; the CLI prints a warning in that case. The CLI does not accept
explicit path arguments today, so it does not hide user-requested paths.

Profiles change branch-review emphasis:

- `standard` prioritizes concrete defects and material review gaps.
- `pr-readiness` asks whether the branch is ready for review or merge, including tests, docs, CI/build impact, and rollout risk.
- `repo-policy` includes policy-file context and asks Codex to report concrete repository policy conflicts.

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

## License

MIT License - see [LICENSE](LICENSE) for details.
