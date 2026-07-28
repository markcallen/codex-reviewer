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

	reportPath := opts.Report
	if !filepath.IsAbs(reportPath) {
		reportPath = filepath.Join(repoRoot, filepath.FromSlash(reportPath))
	}

	args := localReviewArgs(opts, reportPath)
	_, _ = fmt.Fprintf(opts.Stdout, "codex-reviewer: running local review\n")
	_, _ = fmt.Fprintf(opts.Stdout, "codex-reviewer: report path %s\n", reportPath)
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
		return append(baseArgs, "--output-last-message", reportPath, branchReviewPrompt(opts.Base, instructions))
	}
	prompt := instructions
	if prompt == "" {
		prompt = "Do a full code review of this repository. Review the entire codebase, not just the current diff. Use the code_reviewer subagent if available. Focus on correctness, security/privacy risks, missing tests, Docker/GHCR workflow problems, installer behavior, CLI behavior, maintainability, and documentation gaps. Do not edit files. Return prioritized findings with file references, severity, why each issue matters, and suggested fixes. If there are no blocking issues, say that clearly and list the main areas checked."
	}
	return append(baseArgs, "--output-last-message", reportPath, prompt)
}

func branchReviewPrompt(base, instructions string) string {
	if strings.TrimSpace(instructions) == "" {
		return defaultBranchReviewPrompt(base)
	}
	return fmt.Sprintf("Review this branch against %s. Inspect the diff with `git diff %s...HEAD` and read relevant surrounding code before writing the report. Do not edit files.\n\n%s", base, base, instructions)
}

func defaultBranchReviewPrompt(base string) string {
	return fmt.Sprintf(`Review this branch against %s.

Review scope:
- Inspect the diff with git diff %s...HEAD and read relevant surrounding code before writing findings.
- Focus on correctness, security/privacy, behavior regressions, API or CLI contract changes, concurrency, error handling, tests, CI/build behavior, and documentation required by user-visible changes.
- Avoid style-only comments unless they hide a defect or violate an explicit formatter/linter contract.
- Do not edit files; write only the review report.

Required report format:
1. Start with exactly one verdict line: "Block", "Approve with fixes", or "No blocking findings".
2. Add "Diff summary" with the base/ref reviewed and the major subsystems touched.
3. Add "Areas checked" with concise bullets for the touched subsystems and risk classes checked.
4. Add "Areas not checked / limits" for any subsystem, dependency surface, test path, or runtime behavior not deeply verified.
5. Add "Findings" in priority order. If there are no findings, say "None".
6. Add "Tests to run" with the smallest useful validation commands.

Report concrete P0, P1, and P2 issues. Do not invent speculative issues without an execution path.`, base, base)
}

func reviewInstructions(opts LocalOptions) string {
	custom := strings.TrimSpace(opts.Instructions)
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
- Avoid style-only comments unless they hide a defect or violate an explicit formatter/linter contract.
- Do not edit files; write only the review report.

Required report format:
1. Start with exactly one verdict line: "Block", "Approve with fixes", or "No blocking findings".
2. Add "Diff summary" with the base/ref reviewed, approximate changed-file count if available, and the major subsystems touched.
3. Add "Areas checked" with a concise bullet for every touched subsystem you inspected and the risk classes checked there.
4. Add "Areas not checked / limits" for any subsystem, generated artifact, dependency surface, test path, or runtime behavior not deeply verified. Say "None" only if you actually checked all relevant areas.
5. Add "Findings" in priority order. For each finding include severity, title, file/line evidence, failure path, why it matters, and a small suggested fix or targeted test.
6. Add "Tests to run" with the smallest useful validation commands.

Do not stop after the first few findings. Report every concrete P0, P1, and P2 issue found, but do not invent speculative issues without an execution path.`
