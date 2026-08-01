# Usage Estimates and Telemetry

`codex-reviewer` can estimate review token and dollar cost before submitting a
Kubernetes-backed review, and can run a small telemetry aggregator for spend and
effectiveness rollups.

## Dry-run Estimates

Use `service submit --dry-run` to inspect the request that would be sent to the
review API:

```bash
codex-reviewer service submit \
  --dry-run \
  --base origin/main \
  --head HEAD \
  --repo-url git@github.com:org/repo.git \
  --head-sha "$(git rev-parse HEAD)"
```

The JSON output keeps the normal request fields, such as `repo_url`,
`base_ref`, `head_ref`, `head_sha`, `profile`, `profile_config`, and
`return_format`, and adds `usage_estimate`.

`usage_estimate.token_estimate` is approximate. It is based on the resolved
profile prompt, additional instructions, local diff byte size when available,
and changed-file count when available. If git diff data cannot be read, the
estimate falls back to the request/profile size.

`usage_estimate.cost_estimate` applies the built-in default pricing for the
selected model. Pricing is intentionally local and configurable in code through
`internal/usage.DefaultPricing`; it is not fetched from a remote billing API.
Check current model pricing before relying on estimates for budgets.

## Completed Run Metadata

Service runners write `metadata.json` in the review output directory. Completed
runs include:

- `token_usage.status: "available"` plus input, cached input, and output token
  counts when a parseable usage JSON line is observed in command output.
- `cost_usd` when usage is available, calculated with the same default pricing.
- `token_usage.status: "unavailable"` when usage could not be parsed.

Runner debug logs are separate from telemetry and redact obvious credentials.
Telemetry does not include raw source, diffs, prompts, reports, or logs by
default.

## Telemetry Aggregator

Run the in-memory aggregator:

```bash
export CODEX_REVIEWER_TELEMETRY_TOKEN="$(openssl rand -hex 24)"
codex-reviewer service telemetry --listen :8081
```

Kubernetes deployments should provide the token from a Secret and expose the
service only to trusted callers. The process is stateless and stores events only
in memory; restarting it drops retained telemetry.

Endpoints:

- `POST /telemetry/v1/events` ingests a
  `codex-reviewer.review_telemetry.v1` event.
- `GET /telemetry/v1/spend` returns total cost and cost by model/profile.
- `GET /telemetry/v1/effectiveness` returns status, verdict, and profile
  counts.
- `GET /telemetry/v1/export` returns sanitized retained events.

All telemetry endpoints require either:

```text
Authorization: Bearer <token>
```

or:

```text
X-Telemetry-Token: <token>
```

Health and readiness probes are available at `/healthz` and `/readyz`.

## Privacy and Retention Limits

The telemetry service rejects oversized request bodies. The default limit is
256 KiB and can be changed with `--max-body-bytes`.

Telemetry ingestion rejects fields intended to carry raw content, including
`source`, `source_code`, `diff`, `prompt`, `log`, `debug_log`, `report`, and
related raw variants. Accepted event fields are sanitized with the same obvious
secret redaction used by runner debug logs.

The default retention model is process memory only. Do not treat the aggregator
as durable storage unless a future deployment wraps it with explicit persistent
storage and retention controls.

## Non-blocking Submission Telemetry

When the review API is configured with a telemetry recorder, accepted job
submissions emit a sanitized `submitted` event asynchronously. Submission does
not wait for telemetry ingestion. Completed-run telemetry should be produced
from `metadata.json` with `TelemetryEventFromMetadata` so token and cost fields
are included when available.
