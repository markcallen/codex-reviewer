# Kind E2E Review Test

The kind e2e test is opt-in and gated behind the `e2e` build tag. For normal
individual-developer local runs, prefer the Docker/GHCR workflow in
`docs/docker-ghcr.md`; this kind path exists to exercise the Kubernetes job
integration.

By default, `make e2e` reviews one small private repository and one large
private repository from `e2e/repos.json`. The fixture contains 10 private
`markcallen` repositories: five small and five large, selected by GitHub
`diskUsage`.

For fast smoke tests, `make e2e-small` forces a single small repository. Its
default is the public `octocat/Hello-World` repository so the harness can be
validated without relying on private `markcallen` fixtures.

## Requirements

- `kind`
- `kubectl`
- `gh` authenticated with access to `github.com/markcallen`
- A review runner image loaded into kind
- An OpenAI egress sidecar image loaded into kind
- `OPENAI_API_KEY` set in the local environment
- `GITHUB_TOKEN` set in the local environment for private repository clones

## Environment

The Makefile drives the standard setup path:

```bash
make setup
make setup-e2e
make e2e
```

For faster iteration, run one small repo:

```bash
make e2e-small
```

To force one or more specific repositories:

```bash
E2E_REPOS=markcallen/rpi-k3s-cluster make e2e
E2E_REPOS=octocat/Hello-World make e2e-small
```

`E2E_REPOS` accepts comma-separated `owner/repo` values. Repositories listed in
`e2e/repos.json` keep their fixture metadata; other `owner/repo` values are
treated as GitHub HTTPS repositories.

The e2e test currently exercises a branch-change review, not a full repository
audit. It resolves the default branch, checks out the branch head SHA, and uses
the head commit's first parent SHA as `--base` when available. That produces a
real diff review for the latest branch change. Repositories with only one commit
fall back to `origin/<default-branch>`, which can produce an empty review.

The manual commands below show what the Make targets configure.

Build and load the runner image:

```bash
docker build -f Dockerfile.runner -t codex-reviewer:phase1 .
kind load docker-image codex-reviewer:phase1 --name codex-reviewer-e2e
```

Build and load the egress proxy image:

```bash
docker build -f Dockerfile.egress -t openai-egress:phase1 .
kind load docker-image openai-egress:phase1 --name codex-reviewer-e2e
```

```bash
export RUN_KIND_E2E=1
export OPENAI_API_KEY=...
export GITHUB_TOKEN=...
export CODEX_REVIEWER_REVIEWER_IMAGE=codex-reviewer:phase1
export CODEX_REVIEWER_SIDECAR_IMAGE=openai-egress:phase1
export CODEX_REVIEWER_OPENAI_SECRET=openai-api
export CODEX_REVIEWER_GITHUB_SECRET=github-token
export CODEX_REVIEWER_NAMESPACE=codex-reviewer-e2e
export CODEX_REVIEWER_KIND_CLUSTER=codex-reviewer-e2e
```

Optional:

```bash
export CODEX_REVIEWER_SERVICE_ACCOUNT=codex-reviewer
```

## Run

```bash
go test -tags=e2e ./e2e -run TestKindReviewsSmallAndLargePrivateRepos -count=1
```

Use verbose mode to print the selected repository, URL, size, branch, SHA, Job
name, and resulting review output:

```bash
go test -v -tags=e2e ./e2e -run TestKindReviewsSmallAndLargePrivateRepos -count=1
```

The test creates a kind cluster if one does not exist, creates the configured
namespace if needed, creates the configured Kubernetes Secrets from
`OPENAI_API_KEY` and `GITHUB_TOKEN`, resolves each selected repository's default
branch and head SHA with `gh`, uses the head commit's parent SHA as the review
base when available, creates a one-shot Kubernetes Job manifest, applies it,
waits for the reviewer container to finish, reads the reviewer logs, and asserts
that a review verdict or report write confirmation was produced.

The test skips by default unless `RUN_KIND_E2E=1` is set.
