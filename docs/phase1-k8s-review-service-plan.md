# Phase 1 Kubernetes Review Service Plan

## Goal

Build an internal, one-shot code review service that runs Codex inside an
ephemeral Kubernetes pod, reviews trusted internal code, returns a Markdown
analysis document, and fits into existing Codex or Claude-driven development
workflows.

Phase 1 assumes the reviewed code is created by trusted internal engineers. More
aggressive isolation for untrusted or third-party code is deferred to Phase 2.

## Non-Goals For Phase 1

- Multi-tenant hostile-code isolation.
- Public SaaS exposure.
- Arbitrary internet access from review workloads.
- Full SAST replacement.
- Automatic merge without human or workflow-level approval.
- Long-lived review workspaces.
- Dynamic skill installation per job.

## Target Workflow

The service should support this happy path from Codex, Claude, CI, or a local
developer script:

1. Implement a change.
2. Commit the change locally.
3. Run unit tests.
4. Submit the committed diff to the review service.
5. Receive a review report.
6. Fix blocking findings.
7. Run unit tests again.
8. Run e2e tests.
9. Push to the remote branch.

The service does not need to perform every step itself in Phase 1. It should
provide a simple CLI/API contract that makes the sequence easy for Codex,
Claude, or an existing workflow runner to orchestrate.

## Architecture

```text
Codex / Claude / CI / local CLI
        |
        v
codex-reviewer submit
        |
        v
Internal review API
        |
        v
Kubernetes Job: one pod per review
        |
        |-- workspace container: clones or receives source
        |-- codex container: runs review command
        |-- network sidecar: controls OpenAI egress
        |
        v
review.md + metadata.json + logs
```

## Phase 1 Components

### 1. Review Runner Image

Create an internal container image with the reviewer implementation baked in:

- Codex CLI.
- `codex-reviewer` binary.
- Reviewer skills.
- Reviewer subagent definitions.
- Default Codex config.
- `git`, `rg`, and common shell tools.
- Optional language runtime bundles for common unit-test workflows.

Do not install skills dynamically during the review job. Runtime install creates
network, version drift, and failure-mode complexity. The image tag should define
the reviewer version.

### 2. Pluggable Review Profiles

Support named review profiles so different subagents and models can be selected
per workflow.

Example profile names:

- `standard`: balanced correctness and regression review.
- `deep`: slower high-reasoning review for risky branches.
- `security`: security/privacy-focused review.
- `fast`: lightweight review for small diffs.
- `docs`: documentation/API-contract review.

Each profile should map to:

```toml
agent = "code_reviewer"
model = "gpt-5.5"
reasoning_effort = "high"
prompt = "review-branch"
timeout = "30m"
```

Phase 1 can store these profiles in the runner image as TOML/YAML. Later phases
can move them into an admin-managed config service.

### 3. Sidecar Network Access

Run review pods with a sidecar responsible for outbound model API access.

Phase 1 intent:

- The Codex container does not receive broad network access.
- OpenAI API traffic is routed through the sidecar or egress proxy.
- The sidecar owns API-key injection and request egress policy.
- The pod has no general-purpose outbound internet path beyond the approved
  model API destination.

Implementation options:

- Local HTTP CONNECT or HTTPS proxy sidecar.
- Mesh sidecar with explicit egress policy.
- Cluster egress gateway restricted by namespace and service account.

For Phase 1, choose the simplest option compatible with the current cluster. The
important contract is that review jobs use a controlled egress path rather than
unrestricted pod networking.

### 4. Review API

Provide a small internal API.

Initial endpoint:

```text
POST /reviews
GET /reviews/{id}
GET /reviews/{id}/report
```

`POST /reviews` request:

```json
{
  "repo_url": "git@github.com:org/repo.git",
  "base_ref": "origin/main",
  "head_ref": "feature/example",
  "head_sha": "abc123",
  "profile": "standard",
  "instructions": "Focus on auth and persistence changes.",
  "return_format": "markdown"
}
```

`GET /reviews/{id}` response:

```json
{
  "id": "review-123",
  "status": "succeeded",
  "verdict": "approve_with_fixes",
  "profile": "standard",
  "report_url": "/reviews/review-123/report"
}
```

### 5. CLI Integration

Extend `codex-reviewer` with service-oriented commands.

Proposed commands:

```bash
codex-reviewer service submit \
  --base origin/main \
  --head HEAD \
  --profile standard \
  --wait \
  --output review.md

codex-reviewer service status REVIEW_ID

codex-reviewer service report REVIEW_ID --output review.md
```

For local or CI use, the CLI should discover:

- Git root.
- Current branch.
- Upstream remote.
- Merge base.
- Head SHA.
- Dirty working tree status.

Phase 1 should require a clean committed tree for the service path. Dirty
workspace upload can come later.

### 6. Codex And Claude Workflow Adapter

Add a documented workflow that both Codex and Claude can run from a repo:

```bash
git status --short
git add .
git commit -m "$MESSAGE"
make test
codex-reviewer service submit --base origin/main --head HEAD --profile standard --wait --output review.md
codex-reviewer review apply-guidance review.md
make test
make e2e
git push
```

Phase 1 does not need a universal `make test` abstraction. Instead, the adapter
should read project instructions from `AGENTS.md`, `CLAUDE.md`, or explicit CLI
flags:

```bash
codex-reviewer workflow run \
  --unit-test "make test" \
  --e2e-test "make e2e" \
  --profile standard
```

The workflow command should be conservative:

- Stop if the working tree is dirty after tests unless instructed otherwise.
- Stop on review verdict `Block`.
- Require an explicit flag before pushing:

```bash
--push
```

### 7. Kubernetes Job Contract

Each review creates one Kubernetes Job.

Job inputs:

- Review ID.
- Repo URL.
- Base ref/SHA.
- Head ref/SHA.
- Review profile.
- Optional extra instructions.
- Output object path.

Job outputs:

- `review.md`
- `metadata.json`
- `codex.log`
- optional `diff.patch`

Suggested pod layout:

```text
/workspace   checked-out source
/out         review artifacts
/config      mounted profile/config
```

Suggested lifecycle:

1. Clone/fetch exact refs.
2. Check out `head_sha`.
3. Verify `base_ref` exists.
4. Run Codex review with selected profile.
5. Write report and metadata.
6. Upload artifacts.
7. Exit non-zero only for infrastructure failure, not for review findings.

Review findings should be represented in `metadata.json`, not only in process
exit status.

### 8. Review Output Contract

`review.md` should start with one of:

- `Block`
- `Approve with fixes`
- `No blocking findings`

`metadata.json` should include:

```json
{
  "verdict": "block",
  "profile": "standard",
  "model": "gpt-5.5",
  "base_ref": "origin/main",
  "head_sha": "abc123",
  "started_at": "2026-07-25T00:00:00Z",
  "finished_at": "2026-07-25T00:12:00Z",
  "report_path": "review.md"
}
```

## Phase 1 Milestones

### Milestone 1: Runner Contract

- Define review profile config format.
- Define Markdown and metadata output contract.
- Add local `codex-reviewer service submit --dry-run`.
- Add local profile resolution.

### Milestone 2: Container Runner

- Build runner image.
- Bake in Codex CLI, reviewer skill, subagents, and profiles.
- Add an entrypoint that runs a single review from environment variables or a
  mounted request file.
- Validate with a local repository mounted into the container.

### Milestone 3: Kubernetes Job

- Add Job template.
- Add Secret reference for model API credential or sidecar credential.
- Add ConfigMap for profiles.
- Add `emptyDir` volumes for workspace and output.
- Add TTL cleanup and resource limits.
- Add sidecar egress path.

### Milestone 4: Internal API

- Implement `POST /reviews`.
- Create a Kubernetes Job per request.
- Persist status and artifact location.
- Implement report retrieval.

### Milestone 5: Workflow Integration

- Add CLI command that submits current committed branch.
- Add `--wait` and `--output`.
- Add example Codex prompt.
- Add example Claude prompt.
- Add example CI step.

### Milestone 6: End-To-End Pilot

- Run against this repository.
- Run against one service repository.
- Validate profile switching.
- Validate sidecar egress.
- Validate report artifact retrieval.
- Document operational runbook.

## Phase 1 Acceptance Criteria

- A developer can run one command from an existing repo and receive `review.md`.
- A CI job can submit a committed branch review and retrieve the report.
- A workflow agent can commit, test, review, fix, e2e test, and push using
  documented commands.
- Review pods are one-shot Kubernetes Jobs.
- Reviewer implementation is baked into the image.
- Profiles can select different subagents and models.
- OpenAI egress goes through the sidecar or approved egress proxy.
- Review findings are available as Markdown and structured metadata.

## Deferred To Phase 2

Security hardening for code outside trusted internal development moves to Phase
2, including:

- Hostile-code sandboxing.
- Strong per-command network isolation.
- Untrusted dependency install controls.
- Secret exfiltration resistance beyond the Phase 1 sidecar boundary.
- Tenant isolation.
- Admission controls.
- SBOM and provenance enforcement.
- Malware scanning.
- Policy-as-code for allowed commands.
- Running tests from untrusted repos.
- Dirty workspace rsync/upload review.
- Public webhook exposure.
- Organization-wide quota enforcement and abuse controls.

