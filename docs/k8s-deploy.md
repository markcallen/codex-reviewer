# Kubernetes Review Service Deployment

This deploys the `codex-reviewer` API into a Kubernetes namespace and creates
one Job per submitted review.

## Build And Load Images

For kind:

```bash
make kind-load-images
```

For a real cluster, publish the runner image and update
`deploy/k8s/codex-reviewer-api.yaml` to use the published runner and sidecar
image tags.

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
`$CODEX_HOME/auth.json` with mode `0600` and unsets `CODEX_API_KEY` and
`OPENAI_API_KEY` before invoking Codex.

To use device auth in the API Deployment, change the API args from:

```yaml
- --openai-secret=openai-api
```

to:

```yaml
- --codex-auth-secret=codex-auth
```

Keep `--github-secret=github-token` for private repositories.

## Deploy

For kind:

```bash
make deploy-k8s
kubectl --context "$KUBE_CONTEXT" -n "$NAMESPACE" rollout status deploy/codex-reviewer-api
```

For manual apply:

```bash
kubectl -n codex-reviewer apply -f deploy/k8s/codex-reviewer-api.yaml
```

## Local Access

Port-forward the API:

```bash
kubectl --context "$KUBE_CONTEXT" -n "$NAMESPACE" port-forward svc/codex-reviewer-api 8080:8080
export CODEX_REVIEWER_API_URL=http://127.0.0.1:8080
```

Submit the current committed branch and track the result in git-visible files:

```bash
codex-reviewer service submit \
  --base origin/main \
  --head HEAD \
  --profile standard \
  --wait \
  --output codex-review/k8s-review.md
```

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
