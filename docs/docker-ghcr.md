# Docker and GHCR local review

Use this path when an individual developer wants to run the reviewer on their
own machine without kind. The container includes:

- Codex CLI.
- `codex-reviewer`.
- The baked reviewer Codex config.
- The `code_reviewer` subagent definition.
- `git`, `rg`, and basic shell tools.

The container reads credentials from environment variables at runtime:

- `CODEX_API_KEY` for Codex model access.
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

If you only want to build the local CLI and run reviews with the published
container image, build the app normally:

```bash
make build
```

Then use the GHCR run path below. A local Docker image build is not required for
that workflow.

## Publish to GHCR

Choose the package path and tag:

```bash
export GHCR_IMAGE=ghcr.io/markcallen/codex-reviewer
export GHCR_EGRESS_IMAGE=ghcr.io/markcallen/codex-reviewer-egress
export GHCR_TAG=v0.1.0
```

Log in to GHCR with a token that can write packages:

```bash
echo "$GITHUB_TOKEN" | docker login ghcr.io -u <github-username> --password-stdin
```

Tag and push both the reviewer and egress sidecar images:

```bash
make docker-push-images \
  GHCR_IMAGE="$GHCR_IMAGE" \
  GHCR_EGRESS_IMAGE="$GHCR_EGRESS_IMAGE" \
  GHCR_TAG="$GHCR_TAG"
```

The pushed images are:

```text
ghcr.io/markcallen/codex-reviewer:v0.1.0
ghcr.io/markcallen/codex-reviewer-egress:v0.1.0
```

## Run a Local Review from the Image

From the repository you want to review:

```bash
export CODEX_API_KEY=...
export GITHUB_TOKEN=...
codex-reviewer review docker
```

`review docker` checks that `CODEX_API_KEY` and `GITHUB_TOKEN` are set before
starting Docker, then passes both through with Docker `-e` flags. When using
`env-secrets`, store the Codex/OpenAI API key as `CODEX_API_KEY`.

Because Docker is already the isolation boundary, `review docker` runs the
inner Codex process with `--sandbox danger-full-access`. This avoids bubblewrap
namespace failures on hosts that do not allow unprivileged user namespaces
inside containers.

By default, `review docker` uses the release tag from the running
`codex-reviewer` binary version. For example, `codex-reviewer version`
returning `v0.1.0` or `v0.1.0-7-g8fd747e-dirty` runs
`ghcr.io/markcallen/codex-reviewer:v0.1.0`. Development builds with version
`dev` fall back to `latest`. Use `--image` to pin any other image.

If the GHCR package is private, log in first with a GitHub token that has
`read:packages` access:

```bash
echo "$GITHUB_TOKEN" | docker login ghcr.io -u <github-username> --password-stdin
```

To pass branch-review options through the CLI wrapper:

```bash
codex-reviewer review docker --base origin/main --report codex-review/branch-review.md
```

The raw Docker command is:

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
  codex exec --sandbox danger-full-access \
    --output-last-message codex-review/full-review.md \
    "Do a full code review of this repository. Review the entire codebase, not just the current diff. Focus on correctness, security/privacy risks, missing tests, Docker/GHCR workflow problems, installer behavior, CLI behavior, maintainability, and documentation gaps. Do not edit files. Return prioritized findings with file references, severity, why each issue matters, and suggested fixes."
```

This does a full repository review and writes:

```text
codex-review/full-review.md
```

For a branch review, pass `--base`:

```bash
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

## Run App Commands in the Container

Use the same image for CLI commands:

```bash
docker run --rm ghcr.io/markcallen/codex-reviewer:latest codex-reviewer version
docker run --rm ghcr.io/markcallen/codex-reviewer:latest codex-reviewer setup --dry-run
docker run --rm -v "$PWD:/workspace" -w /workspace ghcr.io/markcallen/codex-reviewer:latest codex-reviewer install --dry-run .
docker run --rm -v "$PWD:/workspace" -w /workspace ghcr.io/markcallen/codex-reviewer:latest codex-reviewer doctor .
```

`codex-reviewer review pre-push` is for repositories that have already run
`codex-reviewer install .`, because it validates `.codex-reviewer.toml` before
running the review.
