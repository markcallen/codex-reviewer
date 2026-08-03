# Kubernetes Code Review

Use this skill when an agent should submit the current committed branch to the
shared Kubernetes-backed `codex-reviewer` service and preserve the review
outcome in the repository.

## Requirements

- The repository has a clean working tree. Commit or stash local changes first.
- `codex-reviewer` is installed on PATH.
- `CODEX_AUTH` contains the literal contents of `~/.codex/auth.json`, or
  `~/.codex/auth.json` is readable by the invoking user. `OPENAI_API_KEY` may
  be used as a fallback.
- `GITHUB_TOKEN` is set when the Kubernetes Job must clone private
  repositories.
- Either `CODEX_REVIEWER_API_URL` is set, Codex global config contains a K8s
  API URL, or the Helm chart is available at `deploy/helm/codex-reviewer` or
  through `HELM_CHART`.

```toml
[codex_reviewer]
backend = "k8s"
k8s_api_url = "http://127.0.0.1:8080"
```

## Workflow

1. Check repository state:

```bash
git status --short
git rev-parse --verify HEAD
```

2. Run the smallest relevant tests requested by project instructions.

3. Submit a Kubernetes review and wait for the report:

```bash
codex-reviewer review submit \
  --base origin/main \
  --head HEAD \
  --profile standard \
  --output codex-review/k8s-review.md
```

The command creates or updates Kubernetes Secrets from `CODEX_AUTH`,
`OPENAI_API_KEY`, and `GITHUB_TOKEN`, deploys the Helm chart when needed, starts
a temporary port-forward when no API URL is configured, writes the report, and
updates the tracking record.

4. If the review was submitted with `--wait=false`, refresh it later:

```bash
codex-reviewer service status REVIEW_ID
codex-reviewer service report REVIEW_ID --output codex-review/k8s-review.md
```

5. Read the review report and the generated tracking record:

```bash
find codex-review/k8s-reviews -maxdepth 2 -name record.json -print
```

6. Treat `Block` as merge-blocking. Fix concrete P0/P1 findings, rerun tests,
and submit another Kubernetes review.

7. Commit the non-secret review artifacts when they are relevant to future
agents:

```bash
git add codex-review/k8s-review.md codex-review/k8s-reviews
```

## Output Contract

The report starts with one of:

- `Block`
- `Approve with fixes`
- `No blocking findings`

The tracking record is written under:

```text
codex-review/k8s-reviews/<review-id>/record.json
```

It includes the submitted repo URL, base ref, head ref, head SHA, profile,
review API response, report path, verdict when available, and a timestamp. It
must not contain API keys, Codex auth JSON, GitHub tokens, or other secrets.

## Notes

- Use `--base` to match the branch policy for the repository.
- Use `--profile deep` for risky auth, persistence, migration, or concurrency
changes.
- Use `--track=false` only when the review is intentionally ephemeral.
