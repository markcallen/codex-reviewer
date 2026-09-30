# Review Job placement

- [x] Record requirements and rollout in `PRD.md`.
- [x] Add failing tests for configured/default Job selectors and flag parsing.
- [x] Implement service and chart wiring without changing default scheduling.
- [x] Run full tests, coverage, and Helm lint/render checks (75.3% overall Go coverage).
- [x] Open PR #45, request Copilot review, and monitor CI. All required checks passed. Copilot could not review because the requester reached its quota; there were no code comments.

Rollout: release the API image and chart together, then set `reviewerJob.nodeSelector` in the cluster values. Existing Jobs retain their pod templates. Rollback: remove the chart value or return to the prior release.
