# Codex code reviewer subagent setup

This repository contains a project-scoped Codex reviewer setup:

- `.codex/config.toml` — project Codex defaults, subagent limits, and `/review` model override.
- `.codex-reviewer.toml` — the installed `codex-reviewer` version and pre-push review defaults.
- `.codex/agents/code-reviewer.toml` — the custom read-only `code_reviewer` subagent.
- `AGENTS.md` — repository-level review guidance Codex reads before work.
- `docs/code_review.md` — team review checklist referenced by `AGENTS.md`.
- `cmd/codex-reviewer/` — a self-contained Go CLI with embedded artifacts.
- `scripts/` — development helpers.

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
git add .codex .codex-reviewer.toml AGENTS.md docs/code_review.md
git commit -m "Add Codex code reviewer subagent"
```

When Codex opens the project, trust the project if you want `.codex/` project configuration to load.

The installer is non-destructive. It creates missing files, merges missing review settings into an existing `.codex/config.toml`, appends marked review sections to existing `AGENTS.md` and `docs/code_review.md`, and leaves conflicting agent or prompt files unchanged with a warning.

Verify the setup:

```bash
bin/codex-reviewer doctor
```

The installed `.codex-reviewer.toml` records the CLI version used for the
install. `codex-reviewer doctor` and `codex-reviewer review pre-push` fail when
that version does not match the running binary, so hook runners do not silently
use a different reviewer than the one committed with the repository. Rerunning
`codex-reviewer install` with a newer binary refreshes the recorded version
without replacing the pre-push settings.

The same file can also hold repo-local review defaults:

```toml
[review]
base = "origin/main"
ignore = ["dist/**", "*.lock"]
directives = ["Focus on public API compatibility."]
profile = "pr-readiness"
policy_file = "docs/review-policy.md"
```

Explicit CLI flags win over config. `ignore` globs are applied to local and
pre-push branch diff prompts with Git pathspec excludes. For full-repository
reviews they are advisory only, and the CLI prints a warning.

## 5. Local machine setup

To use the reviewer in every repo without committing project files:

```bash
bin/codex-reviewer setup
```

This copies the reviewer agent to `~/.codex/agents/code-reviewer.toml` and
merges missing reviewer settings into `~/.codex/config.toml` after showing the
planned changes and asking for confirmation. Use `bin/codex-reviewer setup --yes`
for unattended setup.

The global reviewer agent records the `codex-reviewer` version that created it.
Non-dry review commands check that marker and stop with setup guidance when the
installed global reviewer was created by a different CLI version. `setup` also
runs a strict Codex config parse check after applying changes and warns if the
installed Codex CLI rejects the generated config.

The setup command also writes:

```toml
[codex_reviewer]
backend = "local"
report = "codex-review/full-review.md"
k8s_api_url = ""
```

Keep `backend = "local"` for local machine reviews. To use a Kubernetes-backed
review API, set `backend = "k8s"` and configure `k8s_api_url`.

## 6. Docker and GHCR local option

For individual developers who want an isolated local runtime without kind, build
the reviewer image:

```bash
make docker-build-runner
```

Publish the image to GHCR:

```bash
export GHCR_IMAGE=ghcr.io/markcallen/codex-reviewer
export GHCR_TAG=v0.1.0
echo "$GITHUB_TOKEN" | docker login ghcr.io -u <github-username> --password-stdin
make docker-push-runner GHCR_IMAGE="$GHCR_IMAGE" GHCR_TAG="$GHCR_TAG"
```

Run a review from the repository being reviewed:

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

See `docs/docker-ghcr.md` for the raw Docker command and runtime options.

## 7. Run it

Non-interactive branch review:

```bash
codex-reviewer review local --base origin/main --report codex-review/branch-review.md
```

Structured branch review for large or high-risk changes:

```bash
codex-reviewer review local \
  --base origin/main \
  --structured \
  --report codex-review/branch-review.md
```

Structured mode requires the report to include a diff summary, areas checked,
areas not checked or not deeply verified, prioritized findings, and tests to
run.

Profile-guided branch reviews:

```bash
codex-reviewer review local \
  --base origin/main \
  --profile repo-policy \
  --policy-file docs/review-policy.md
```

Supported profiles are `standard`, `pr-readiness`, and `repo-policy`.
`repo-policy` includes policy-file context and report-scope metadata in the
prompt.

Advisory mode recommendation:

```bash
codex-reviewer review recommend --base origin/main
```

This command does not use the network or invoke Codex. It reports changed-file
count, approximate diff size, risk signals, a recommended mode, reasons, and an
approximate token range. `codex-reviewer review local --recommend` and
`codex-reviewer review pre-push --recommend` provide the same advisory output
from those workflows.

Non-interactive full repository review:

```bash
codex-reviewer review local
```

Pre-push review command for pre-commit, Husky, or another hook runner:

```bash
codex-reviewer review pre-push
```

The command does not install or manage hooks. It reads `.codex-reviewer.toml`,
checks that the installed config version matches the running binary, requires a
clean working tree by default, runs a prompt-based branch review, and writes the
report to `.git/codex-review/pre-push-review.md` unless configured otherwise.

Built-in local reviewer:

```text
/review
```

The `review_model = "gpt-5.5"` setting makes `/review` use GPT-5.5 even if your current session model is different.

## 9. Optional GitHub usage

For GitHub PR reviews through Codex cloud, set up Codex cloud for the repository, enable Code review in Codex settings, and add review guidance to `AGENTS.md`. You can request a review in a PR comment with:

```text
@codex review for security regressions, missing tests, and risky behavior changes.
```
