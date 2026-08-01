# Review config and mode selection

`codex-reviewer` reads repo-local review defaults from `.codex-reviewer.toml`.
Explicit CLI flags always win over config.

```toml
version = "v1.2.3"

[review]
base = "origin/main"
ignore = ["dist/**", "*.lock"]
directives = [
  "Check public API compatibility.",
  "Treat missing behavior tests as a blocking finding.",
]
profile = "standard"
policy_file = ""

[review.pre_push]
base = ""
block_on = "block"
report = ".git/codex-review/pre-push-review.md"
require_clean_tree = true
```

## Defaults and precedence

`--base` overrides `[review].base` and `[review.pre_push].base`. If no base is
configured, branch reviews fall back to upstream, `origin/main`,
`origin/master`, then `main`.

`--profile` overrides `[review].profile`. Supported profiles are:

- `standard`: concrete correctness, security, regression, test, and maintenance findings.
- `pr-readiness`: merge readiness, including tests, docs, CI/build impact, hook/workflow setup, config/default/flag consistency, naming consistency, and rollout risk.
- `strict`: standard defect review plus PR-readiness and repo-policy checks, with concrete actionable P3 workflow, policy, and release-readiness findings.
- `repo-policy`: compatibility profile for repository policy conflicts, with optional policy-file context.

`--policy-file` overrides `[review].policy_file`.

Branch review reports are prompted to include a `Review Scope` section with the
base, head, review type, selected profile, policy files considered, changed
files reviewed, and checks performed. Branch prompts always tell Codex to read
`AGENTS.md` and `docs/code_review.md` when present and to inspect changed rule
files under `.codex/rules/**` and `.claude/rules/**`.

## Ignore globs

`[review].ignore` is applied to local and pre-push branch diff prompts with Git
pathspec excludes before Codex is invoked:

```bash
git diff origin/main...HEAD -- . ':(exclude)dist/**' ':(exclude)*.lock'
```

For full-repository reviews, ignore globs are advisory because Codex can still
inspect the workspace. The CLI prints a warning for that mode. The CLI does not
accept explicit path arguments today, so it does not hide user-requested paths.

## Recommendations

Use:

```bash
codex-reviewer review recommend --base origin/main
```

The command is local-only and deterministic. It summarizes changed files,
approximate diff size, simple risk signals, a recommended review mode, reasons,
and an approximate token range. It does not invoke Codex or use the network.
The same advisory output is available from `review local --recommend` and
`review pre-push --recommend`.
