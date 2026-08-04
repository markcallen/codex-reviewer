# Kubernetes Review Service Deployment

This deploys the `codex-reviewer` API into a Kubernetes namespace and creates
one Job per submitted review.

## One Command Review

From a committed repository you want reviewed, run:

```bash
export CODEX_AUTH="$(cat ~/.codex/auth.json)"
export GITHUB_TOKEN=...
codex-reviewer review submit
```

`review submit` builds the review request, creates or updates the Kubernetes
namespace, creates the required Secrets, deploys the Helm chart when an API URL
is not already configured, starts a temporary port-forward, submits the review,
waits for the report, and writes the tracking artifacts.

Use `--dry-run` to inspect the request and planned Kubernetes actions without
submitting anything:

```bash
codex-reviewer review submit --dry-run
```

If you run the command outside the `codex-reviewer` checkout, either set
`CODEX_REVIEWER_API_URL` for an already-running service or point at the chart:

```bash
HELM_CHART=/path/to/codex-code-reviewer/deploy/helm/codex-reviewer \
codex-reviewer review submit
```

The command uses these defaults, all overrideable with flags or matching
environment variables:

```text
KUBE_CONTEXT=kind-codex-reviewer-e2e
NAMESPACE=codex-reviewer-e2e
HELM_RELEASE=codex-reviewer
RUNNER_IMAGE=codex-reviewer:phase1
SIDECAR_IMAGE=openai-egress:phase1
AUTH_MODE=auto
```

`AUTH_MODE=auto` prefers `CODEX_AUTH` or `~/.codex/auth.json`, then falls back
to `OPENAI_API_KEY`.

Review Jobs run the OpenAI egress proxy as a native Kubernetes sidecar init
container (`initContainers[*].restartPolicy=Always`) so the reviewer container
can finish without keeping the Job alive. Use Kubernetes 1.29 or newer, where
sidecar containers are stable. Older clusters may reject the manifest or run the
proxy with normal init-container semantics, which breaks `HTTP_PROXY` and
`HTTPS_PROXY` for the reviewer.

## Build And Load Images

For kind:

```bash
make kind-load-images
```

For a real cluster, publish the runner image and pass the published runner and
sidecar image tags through `RUNNER_IMAGE` and `SIDECAR_IMAGE`.

## Secrets

Device-code auth is preferred when you want the pod to use the same Codex auth
shape as a local Codex login. Export `CODEX_AUTH` as the literal JSON content
that Codex normally stores in `auth.json`:

```bash
export CODEX_AUTH="$(cat ~/.codex/auth.json)"
export GITHUB_TOKEN=...
make kind-secrets
```

`kind-secrets` creates:

- `codex-auth`, key `auth.json`, when `CODEX_AUTH` is set.
- `openai-api`, key `api-key`, when `CODEX_AUTH` is not set and
  `OPENAI_API_KEY` is set.
- `github-token`, key `token`, when `GITHUB_TOKEN` is set.

When a review Job receives `CODEX_AUTH`, the runner writes it to
`$CODEX_HOME/auth.json` with mode `0600` and unsets `CODEX_AUTH`,
`CODEX_API_KEY`, and `OPENAI_API_KEY` before invoking Codex.

When `CODEX_AUTH` is set, `make deploy-k8s` defaults `AUTH_MODE=codex` and the
chart configures Jobs with `--codex-auth-secret=codex-auth`. Without
`CODEX_AUTH`, it defaults to `AUTH_MODE=openai` and configures
`--openai-secret=openai-api`.

You can override the auth mode and secret names explicitly:

```bash
AUTH_MODE=codex \
CODEX_AUTH_SECRET=codex-auth \
CODEX_AUTH_SECRET_KEY=auth.json \
make deploy-k8s
```

Keep `GITHUB_SECRET=github-token` for private repositories.

## Deploy

For kind:

```bash
make helm-lint
make deploy-k8s
kubectl --context "$KUBE_CONTEXT" -n "$NAMESPACE" rollout status deploy/codex-reviewer
```

For manual Helm deployment:

```bash
helm upgrade --install codex-reviewer deploy/helm/codex-reviewer \
  --namespace codex-reviewer \
  --create-namespace \
  --set-string image.fullOverride=ghcr.io/markcallen/codex-reviewer:latest \
  --set-string reviewerJob.image.fullOverride=ghcr.io/markcallen/codex-reviewer:latest \
  --set-string reviewerJob.sidecarImage.fullOverride=openai-egress:phase1 \
  --set-string auth.mode=codex \
  --set-string auth.codexAuthSecret.name=codex-auth \
  --set-string github.secret.name=github-token
```

## Local Access

Port-forward the API:

```bash
kubectl --context "$KUBE_CONTEXT" -n "$NAMESPACE" port-forward svc/codex-reviewer 8080:8080
export CODEX_REVIEWER_API_URL=http://127.0.0.1:8080
```

Submit the current committed branch and track the result in git-visible files:

```bash
codex-reviewer review submit \
  --base origin/main \
  --head HEAD \
  --profile standard \
  --output codex-review/k8s-review.md
```

For asynchronous submissions, keep the review ID from `service submit`, then
refresh status or fetch the report later:

```bash
codex-reviewer service status REVIEW_ID
codex-reviewer service report REVIEW_ID --output codex-review/k8s-review.md
```

Both commands update the same git-visible tracking record by default.

The CLI writes:

```text
codex-review/k8s-review.md
codex-review/k8s-reviews/<review-id>/record.json
```

Commit those files when future agents should know the review request and
outcome.

## Skill Install

The shared skill lives at:

```text
skills/k8s-code-review/SKILL.md
```

Install it for Codex, Claude, or both:

```bash
scripts/install-k8s-code-review-skill.sh codex
scripts/install-k8s-code-review-skill.sh claude
scripts/install-k8s-code-review-skill.sh both
```

The installer uses `$CODEX_HOME/skills` for Codex and
`$CLAUDE_SKILLS_HOME` or `~/.claude/skills` for Claude. The skill tells agents
how to submit a Kubernetes review, read the report, and preserve the tracking
record.
