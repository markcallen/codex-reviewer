# Kind E2E Review Test

The kind e2e test is opt-in and gated behind the `e2e` build tag.

It reviews one small private repository and one large private repository from
`e2e/repos.json`. The fixture contains 10 private `markcallen` repositories:
five small and five large, selected by GitHub `diskUsage`.

## Requirements

- `kind`
- `kubectl`
- `gh` authenticated with access to `github.com/markcallen`
- A review runner image loaded into kind
- An OpenAI egress sidecar image loaded into kind
- A Kubernetes Secret containing the model API credential
- A Kubernetes Secret containing a GitHub token for private repository clones

## Environment

The Makefile drives the standard setup path:

```bash
make setup
make setup-e2e
make e2e
```

The manual commands below show what those targets configure.

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

The test creates a kind cluster if one does not exist, creates the configured
namespace if needed, resolves each selected repository's default branch SHA with
`gh`, creates a one-shot Kubernetes Job manifest, applies it, and waits for the
Job to complete.

The test skips by default unless `RUN_KIND_E2E=1` is set.
