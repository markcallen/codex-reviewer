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
	fmt.Fprintf(opts.Stdout, "codex-reviewer: running local review\n")
	fmt.Fprintf(opts.Stdout, "codex-reviewer: report path %s\n", reportPath)
	if opts.DryRun {
		fmt.Fprintf(opts.Stdout, "codex-reviewer: dry run: codex %s\n", strings.Join(args, " "))
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(reportPath), 0o755); err != nil {
		return fmt.Errorf("create review report directory: %w", err)
	}
	if _, err := os.Stat(reportPath); err == nil {
		fmt.Fprintf(opts.Stdout, "codex-reviewer: existing report will be overwritten: %s\n", reportPath)
	} else if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("check review report path: %w", err)
	}
	cmd := exec.CommandContext(ctx, "codex", args...)
	cmd.Dir = repoRoot
	cmd.Stdout = opts.Stdout
	cmd.Stderr = opts.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run codex review: %w", err)
	}
	fmt.Fprintf(opts.Stdout, "codex-reviewer: review complete\n")
	fmt.Fprintf(opts.Stdout, "codex-reviewer: wrote review report to %s\n", reportPath)
	return nil
}

func localReviewArgs(opts LocalOptions, reportPath string) []string {
	instructions := strings.TrimSpace(opts.Instructions)
	if opts.Base != "" && !opts.Full {
		if instructions == "" {
			instructions = "Focus on correctness, security/privacy, regressions, missing tests, Docker/GHCR workflow problems, installer behavior, CLI behavior, maintainability, and documentation gaps. Do not edit files."
		}
		return []string{"exec", "review", "--base", opts.Base, "--output-last-message", reportPath, instructions}
	}
	prompt := instructions
	if prompt == "" {
		prompt = "Do a full code review of this repository. Review the entire codebase, not just the current diff. Use the code_reviewer subagent if available. Focus on correctness, security/privacy risks, missing tests, Docker/GHCR workflow problems, installer behavior, CLI behavior, maintainability, and documentation gaps. Do not edit files. Return prioritized findings with file references, severity, why each issue matters, and suggested fixes. If there are no blocking issues, say that clearly and list the main areas checked."
	}
	return []string{"exec", "--output-last-message", reportPath, prompt}
}
