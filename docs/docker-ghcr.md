# Docker and GHCR local review

Use this path when an individual developer wants to run the reviewer on their
own machine without kind. The container includes:

- Codex CLI.
- `codex-reviewer`.
- The baked reviewer Codex config.
- The `code_reviewer` subagent definition.
- `git`, `rg`, and basic shell tools.

The container reads credentials from environment variables at runtime:

- `OPENAI_API_KEY` for Codex model access.
- `GITHUB_TOKEN` for GitHub access when the review needs private repository
  metadata or remote fetches.

## Build

```bash
make docker-build-runner
```

The default local image is:

```text
codex-reviewer:phase1
```

## Publish to GHCR

Choose the package path and tag:

```bash
export GHCR_IMAGE=ghcr.io/<owner>/codex-code-reviewer
export GHCR_TAG=v0.1.0
```

Log in to GHCR with a token that can write packages:

```bash
echo "$GITHUB_TOKEN" | docker login ghcr.io -u <github-username> --password-stdin
```

Tag and push:

```bash
make docker-push-runner GHCR_IMAGE="$GHCR_IMAGE" GHCR_TAG="$GHCR_TAG"
```

The pushed image is:

```text
ghcr.io/<owner>/codex-code-reviewer:v0.1.0
```

## Run a Local Review from the Image

From the repository you want to review:

```bash
export OPENAI_API_KEY=...
export GITHUB_TOKEN=...

make docker-run-review
```

By default this reviews against `origin/main` and writes:

```text
.git/codex-review/docker-review.md
```

Override the base or report path:

```bash
make docker-run-review \
  REVIEW_BASE=origin/main \
  REVIEW_REPORT=.git/codex-review/docker-review.md
```

Run the review using a GHCR image instead of the local image:

```bash
make docker-run-review \
  DOCKER_RUN_IMAGE=ghcr.io/<owner>/codex-code-reviewer:v0.1.0
```

The equivalent raw Docker command is:

```bash
docker run --rm \
  --user "$(id -u):$(id -g)" \
  -e OPENAI_API_KEY \
  -e GITHUB_TOKEN \
  -e REVIEW_BASE=origin/main \
  -e REVIEW_REPORT=.git/codex-review/docker-review.md \
  -v "$PWD:/workspace" \
  -w /workspace \
  ghcr.io/<owner>/codex-code-reviewer:v0.1.0 \
  sh -lc 'mkdir -p "$(dirname "$REVIEW_REPORT")" && codex exec review --base "$REVIEW_BASE" --output-last-message "$REVIEW_REPORT" "Focus on correctness, security/privacy, regressions, missing tests, and maintainability. Do not edit files."'
```

## Run App Commands in the Container

Use the same image for CLI commands:

```bash
docker run --rm ghcr.io/<owner>/codex-code-reviewer:v0.1.0 codex-reviewer version
docker run --rm -v "$PWD:/workspace" -w /workspace ghcr.io/<owner>/codex-code-reviewer:v0.1.0 codex-reviewer install --dry-run .
docker run --rm -v "$PWD:/workspace" -w /workspace ghcr.io/<owner>/codex-code-reviewer:v0.1.0 codex-reviewer doctor .
```

`codex-reviewer review pre-push` is for repositories that have already run
`codex-reviewer install .`, because it validates `.codex-reviewer.toml` before
running the review.
