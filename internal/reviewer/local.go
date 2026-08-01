package reviewer

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type LocalOptions struct {
	Dir          string
	Base         string
	Report       string
	Instructions string
	Directives   []string
	Ignore       []string
	Profile      string
	PolicyFile   string
	Structured   bool
	Full         bool
	DryRun       bool
	Stdout       io.Writer
	Stderr       io.Writer
}

func RunLocal(ctx context.Context, opts LocalOptions) error {
	if opts.Dir == "" {
		opts.Dir = "."
	}
	if opts.Report == "" {
		opts.Report = "codex-review/full-review.md"
	}
	if opts.Stdout == nil {
		opts.Stdout = os.Stdout
	}
	if opts.Stderr == nil {
		opts.Stderr = os.Stderr
	}
	repoRoot, err := gitOutput(ctx, opts.Dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return fmt.Errorf("find git repository root: %w", err)
	}
	repoRoot = strings.TrimSpace(repoRoot)
	cfg, _, err := LoadRepoConfig(repoRoot, false)
	if err != nil {
		return err
	}
	if opts.Base == "" {
		opts.Base = cfg.Base
	}
	if len(opts.Ignore) == 0 {
		opts.Ignore = cfg.Ignore
	}
	if len(opts.Directives) == 0 {
		opts.Directives = cfg.Directives
	}
	if opts.Profile == "" {
		opts.Profile = cfg.Profile
	}
	if opts.PolicyFile == "" {
		opts.PolicyFile = cfg.PolicyFile
	}

	reportPath := opts.Report
	if !filepath.IsAbs(reportPath) {
		reportPath = filepath.Join(repoRoot, filepath.FromSlash(reportPath))
	}

	args := localReviewArgs(opts, reportPath)
	_, _ = fmt.Fprintf(opts.Stdout, "codex-reviewer: running local review\n")
	_, _ = fmt.Fprintf(opts.Stdout, "codex-reviewer: report path %s\n", reportPath)
	warnIgnoredScope(opts.Stdout, opts)
	if opts.DryRun {
		_, _ = fmt.Fprintf(opts.Stdout, "codex-reviewer: dry run: codex %s\n", strings.Join(args, " "))
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(reportPath), 0o755); err != nil {
		return fmt.Errorf("create review report directory: %w", err)
	}
	if _, err := os.Stat(reportPath); err == nil {
		_, _ = fmt.Fprintf(opts.Stdout, "codex-reviewer: existing report will be overwritten: %s\n", reportPath)
	} else if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("check review report path: %w", err)
	}
	cmd := exec.CommandContext(ctx, "codex", args...)
	configureReviewCommand(cmd)
	cmd.Dir = repoRoot
	cmd.Stdout = opts.Stdout
	cmd.Stderr = opts.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run codex review: %w", err)
	}
	_, _ = fmt.Fprintf(opts.Stdout, "codex-reviewer: review complete\n")
	_, _ = fmt.Fprintf(opts.Stdout, "codex-reviewer: wrote review report to %s\n", reportPath)
	return nil
}

func localReviewArgs(opts LocalOptions, reportPath string) []string {
	return codexReviewArgs(opts, reportPath)
}

func codexReviewArgs(opts LocalOptions, reportPath string) []string {
	instructions := reviewInstructions(opts)
	baseArgs := []string{"exec"}
	if opts.Base != "" && !opts.Full {
		return append(baseArgs, "--output-last-message", reportPath, branchReviewPrompt(opts, instructions))
	}
	prompt := instructions
	if prompt == "" {
		prompt = "Do a full code review of this repository. Review the entire codebase, not just the current diff. Use the code_reviewer subagent if available. Focus on correctness, security/privacy risks, missing tests, Docker/GHCR workflow problems, installer behavior, CLI behavior, maintainability, and documentation gaps. Do not edit files. Return prioritized findings with file references, severity, why each issue matters, and suggested fixes. If there are no blocking issues, say that clearly and list the main areas checked."
	}
	return append(baseArgs, "--output-last-message", reportPath, prompt)
}

func branchReviewPrompt(opts LocalOptions, instructions string) string {
	base := opts.Base
	if strings.TrimSpace(instructions) == "" {
		return defaultBranchReviewPrompt(opts)
	}
	return fmt.Sprintf("Review this branch against %s. Inspect the diff with `%s` and read relevant surrounding code before writing the report. Do not edit files.\n%s\n\n%s\n%s\n\nThe report must include a \"Review Scope\" section with base, head, review type, profile, policy files considered, changed files reviewed, and checks performed.", base, diffCommand(base, opts.Ignore), profilePrompt(opts), instructions, reviewMetadata(opts))
}

func defaultBranchReviewPrompt(opts LocalOptions) string {
	base := opts.Base
	return fmt.Sprintf(`Review this branch against %s.

Review scope:
- Inspect the diff with %s and read relevant surrounding code before writing findings.
- Focus on correctness, security/privacy, behavior regressions, API or CLI contract changes, concurrency, error handling, tests, CI/build behavior, and documentation required by user-visible changes.
- Read repository policy context before writing findings. Always check AGENTS.md and docs/code_review.md when present. If this branch changes .codex/rules/**, .claude/rules/**, AGENTS.md, docs/code_review.md, or other review/setup policy docs, verify the rest of the branch complies with those changed rules.
- Check PR-readiness and developer workflow consistency: introduced TODOs and task-tracking rules, docs versus CLI behavior, hook/workflow setup completeness, config/default/flag consistency, tool and command naming consistency, and release-readiness gaps.
- Avoid style-only comments unless they hide a defect or violate an explicit formatter/linter contract.
- Include concrete P3 findings when they affect developer workflow, repository policy compliance, or release readiness.
- Do not edit files; write only the review report.
%s

Required report format:
1. Start with exactly one verdict line: "Block", "Approve with fixes", or "No blocking findings".
2. Add "Review Scope" with base, head, review type, profile, policy files considered, changed files reviewed, and checks performed.
3. Add "Diff summary" with the base/ref reviewed and the major subsystems touched.
4. Add "Areas checked" with concise bullets for the touched subsystems and risk classes checked.
5. Add "Areas not checked / limits" for any subsystem, dependency surface, test path, or runtime behavior not deeply verified.
6. Add "Findings" in priority order. If there are no findings, say "None".
7. Add "Tests to run" with the smallest useful validation commands.

Report concrete P0, P1, and P2 issues. For pr-readiness, repo-policy, and strict profiles, also report concrete actionable P3 developer-workflow or release-readiness issues. Do not invent speculative issues without an execution path.%s`, base, diffCommand(base, opts.Ignore), profilePrompt(opts), reviewMetadata(opts))
}

func reviewInstructions(opts LocalOptions) string {
	custom := strings.TrimSpace(opts.Instructions)
	directives := strings.TrimSpace(strings.Join(opts.Directives, "\n"))
	if directives != "" {
		if custom == "" {
			custom = directives
		} else {
			custom += "\n" + directives
		}
	}
	if !opts.Structured {
		return custom
	}
	if custom == "" {
		return structuredReviewPrompt
	}
	return structuredReviewPrompt + "\n\nAdditional caller instructions:\n" + custom
}

const structuredReviewPrompt = `Do a structured code review.

Review scope:
- Inspect the requested branch diff or full repository scope, then identify the major subsystems touched before writing findings.
- Read relevant surrounding code for each high-risk subsystem instead of only isolated diff hunks.
- Focus on correctness, security/privacy, behavior regressions, API or CLI contract changes, persistence/migration risk, concurrency, error handling, tests, CI/build behavior, deployment/runtime behavior, and documentation required by user-visible changes.
- Read repository policy context before writing findings. Always check AGENTS.md and docs/code_review.md when present. If this branch changes .codex/rules/**, .claude/rules/**, AGENTS.md, docs/code_review.md, or other review/setup policy docs, verify the rest of the branch complies with those changed rules.
- Check PR-readiness and developer workflow consistency: introduced TODOs and task-tracking rules, docs versus CLI behavior, hook/workflow setup completeness, config/default/flag consistency, tool and command naming consistency, and release-readiness gaps.
- Avoid style-only comments unless they hide a defect or violate an explicit formatter/linter contract.
- Include concrete P3 findings when they affect developer workflow, repository policy compliance, or release readiness.
- Do not edit files; write only the review report.

Required report format:
1. Start with exactly one verdict line: "Block", "Approve with fixes", or "No blocking findings".
2. Add "Review Scope" with base, head, review type, profile, policy files considered, changed files reviewed, and checks performed.
3. Add "Diff summary" with the base/ref reviewed, approximate changed-file count if available, and the major subsystems touched.
4. Add "Areas checked" with a concise bullet for every touched subsystem you inspected and the risk classes checked there.
5. Add "Areas not checked / limits" for any subsystem, generated artifact, dependency surface, test path, or runtime behavior not deeply verified. Say "None" only if you actually checked all relevant areas.
6. Add "Findings" in priority order. For each finding include severity, title, file/line evidence, failure path, why it matters, and a small suggested fix or targeted test.
7. Add "Tests to run" with the smallest useful validation commands.

Do not stop after the first few findings. Report every concrete P0, P1, and P2 issue found. Also report concrete actionable P3 developer-workflow or release-readiness issues when the selected profile asks for them, but do not invent speculative issues without an execution path.`

func diffCommand(base string, ignore []string) string {
	parts := []string{"git diff", base + "...HEAD"}
	excludes := pathspecExcludes(ignore)
	if len(excludes) > 0 {
		parts = append(parts, "--", ".")
		parts = append(parts, excludes...)
	}
	return strings.Join(parts, " ")
}

func profilePrompt(opts LocalOptions) string {
	switch opts.Profile {
	case "pr-readiness":
		return "- PR-readiness profile: assess whether this branch is ready for review or merge, including user-visible behavior, tests, docs, CI/build impact, hook/workflow setup, config/default/flag consistency, naming consistency, and rollout risk. Report concrete P3 findings when they affect developer workflow or release readiness."
	case "repo-policy":
		return "- Repo-policy profile: compare the branch against repository policy and call out concrete policy conflicts. Read AGENTS.md and docs/code_review.md when present, read the policy file when one is provided, and inspect changed .codex/rules/** and .claude/rules/** files before writing findings."
	case "strict":
		return "- Strict profile: combine standard defect review with the PR-readiness and repo-policy passes. Be more willing to report concrete actionable P3 findings for developer workflow, repository policy, config/default/flag consistency, naming consistency, hook/workflow setup, and release readiness."
	default:
		return "- Standard profile: prioritize concrete defects and material review gaps."
	}
}

func reviewMetadata(opts LocalOptions) string {
	var lines []string
	profile := opts.Profile
	if profile == "" {
		profile = "standard"
	}
	lines = append(lines, "", "Review Scope metadata to include in the report:")
	lines = append(lines, "- base: "+opts.Base)
	lines = append(lines, "- head: HEAD")
	lines = append(lines, "- review type: branch")
	lines = append(lines, "- profile: "+profile)
	lines = append(lines, "- default policy files to check when present: AGENTS.md, docs/code_review.md")
	lines = append(lines, "- changed policy globs to check when present: .codex/rules/**, .claude/rules/**, AGENTS.md, docs/code_review.md")
	lines = append(lines, "- checks: correctness/security, tests/coverage, CI/release workflows, repo rule compliance, developer workflow consistency")
	if opts.PolicyFile != "" {
		lines = append(lines, "- policy file: "+opts.PolicyFile)
		lines = append(lines, "Read the policy file and include any policy-specific limits in the report.")
	}
	if len(opts.Ignore) > 0 {
		lines = append(lines, "- ignored diff globs: "+strings.Join(opts.Ignore, ", "))
		lines = append(lines, "Treat ignored globs as outside diff-review scope unless surrounding context is needed for a finding.")
	}
	return strings.Join(lines, "\n")
}

func warnIgnoredScope(out io.Writer, opts LocalOptions) {
	if len(opts.Ignore) == 0 {
		return
	}
	if opts.Base != "" && !opts.Full {
		_, _ = fmt.Fprintf(out, "codex-reviewer: applying review ignore globs to diff command: %s\n", strings.Join(opts.Ignore, ", "))
		return
	}
	_, _ = fmt.Fprintf(out, "codex-reviewer: warning: review ignore globs are advisory for full-repository prompts; use --base for pathspec-filtered diff review\n")
}
