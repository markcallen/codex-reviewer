# Documentation

- [Code review checklist](code_review.md)
- [Docker and GHCR local review](docker-ghcr.md)
- [Kind E2E review test](kind-e2e.md)
- [Phase 1 Kubernetes review service plan](phase1-k8s-review-service-plan.md)
- [Sources](sources.md)

## System Flow

```mermaid
flowchart LR
  Dev[Developer] --> CLI[codex-reviewer CLI]
  CLI --> Local[Local Codex review]
  CLI --> API[Kubernetes review API]
  API --> Job[Kubernetes review Job]
  Job --> Runner[Reviewer container]
  Runner --> Report[Markdown review report]
```

## Review Service Request Flow

```mermaid
sequenceDiagram
  participant Client
  participant API as Review API
  participant K8s as Kubernetes
  participant Job as Review Job
  Client->>API: POST /reviews
  API->>K8s: apply Job manifest
  API-->>Client: 202 submitted
  Job->>Job: clone repo and run review
  Client->>API: GET /reviews/{id}/report
  API->>K8s: read reviewer logs
  API-->>Client: report or 202 not ready
```
