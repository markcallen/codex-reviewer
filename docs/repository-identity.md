# Repository Identity

The canonical repository identity is:

```text
markcallen/codex-reviewer
```

This identity is used for the GitHub repository, Go module path, release
configuration, README badges, release URLs, and published GHCR image path.

| Surface | Canonical value |
| --- | --- |
| GitHub repository | `github.com/markcallen/codex-reviewer` |
| Go module | `github.com/markcallen/codex-reviewer` |
| GHCR reviewer image | `ghcr.io/markcallen/codex-reviewer` |
| GHCR egress sidecar image | `ghcr.io/markcallen/codex-reviewer-egress` |
| Release URL | `https://github.com/markcallen/codex-reviewer/releases` |
| CLI binary | `codex-reviewer` |
| GoReleaser project | `codex-reviewer` |

The product-facing name is `Codex Reviewer`. The CLI and package names remain
lowercase `codex-reviewer`.

Intentional exceptions:

- A local checkout directory may have any name, including historical or
  user-specific names such as `codex-code-reviewer`; it is not part of the
  published repository identity.
- The installed Codex subagent is named `code_reviewer` because Codex subagent
  identifiers use snake_case and this name describes the subagent role.
