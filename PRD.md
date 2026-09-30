# Codex Reviewer

## Review Job placement

Operators can optionally select Kubernetes nodes for dynamically created review Jobs without changing where the API Deployment runs. The Helm chart exposes `reviewerJob.nodeSelector` as a map of node label keys and values. The API accepts repeatable `--node-selector=key=value` flags and copies those selectors into the Job pod template. An empty selector preserves default Kubernetes scheduling. The standalone `service job-manifest` command accepts the same flags.

Acceptance criteria:

- Multiple node selectors appear in each newly created Job pod template.
- With no selectors configured, the Job pod template omits `nodeSelector`.
- Invalid flag entries fail before the API starts or a manifest is generated.
- The chart keeps the API Deployment's existing `nodeSelector` separate from the review Job selector.
