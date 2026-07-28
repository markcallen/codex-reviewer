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

const DefaultDockerRepository = "ghcr.io/markcallen/codex-reviewer"

var requiredDockerEnv = []string{"CODEX_API_KEY", "GITHUB_TOKEN"}

type DockerOptions struct {
	Dir          string
	Image        string
	Pull         string
	Base         string
	Report       string
	Instructions string
	Structured   bool
	Full         bool
	DryRun       bool
	Stdout       io.Writer
	Stderr       io.Writer
}

func RunDocker(ctx context.Context, opts DockerOptions) error {
	if opts.Dir == "" {
		opts.Dir = "."
	}
	if opts.Image == "" {
		opts.Image = DefaultDockerRepository + ":latest"
	}
	if opts.Pull == "" {
		opts.Pull = "missing"
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
	if !opts.DryRun {
		if err := requireDockerEnv(requiredDockerEnv); err != nil {
			return err
		}
	}

	repoRoot, err := gitOutput(ctx, opts.Dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return fmt.Errorf("find git repository root: %w", err)
	}
	repoRoot = filepath.Clean(strings.TrimSpace(repoRoot))

	reportPath, err := dockerContainerReportPath(repoRoot, opts.Report)
	if err != nil {
		return err
	}
	args := dockerReviewArgs(repoRoot, reportPath, opts)
	_, _ = fmt.Fprintf(opts.Stdout, "codex-reviewer: running Docker review with %s\n", opts.Image)
	_, _ = fmt.Fprintf(opts.Stdout, "codex-reviewer: mounted repository %s at /workspace\n", repoRoot)
	if opts.DryRun {
		_, _ = fmt.Fprintf(opts.Stdout, "codex-reviewer: dry run: docker %s\n", strings.Join(args, " "))
		return nil
	}
	hostReportPath, err := dockerHostReportPath(repoRoot, opts.Report)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(hostReportPath), 0o755); err != nil {
		return fmt.Errorf("create review report directory: %w", err)
	}

	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdout = opts.Stdout
	cmd.Stderr = opts.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run docker review: %w", err)
	}
	return nil
}

func requireDockerEnv(names []string) error {
	var missing []string
	for _, name := range names {
		if strings.TrimSpace(os.Getenv(name)) == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required environment variable(s) for Docker review: %s", strings.Join(missing, ", "))
	}
	return nil
}

func dockerContainerReportPath(repoRoot, report string) (string, error) {
	if !filepath.IsAbs(report) {
		clean, err := cleanRelativeReportPath(report)
		if err != nil {
			return "", err
		}
		return filepath.ToSlash(clean), nil
	}
	rel, err := filepath.Rel(repoRoot, report)
	if err != nil {
		return "", fmt.Errorf("resolve report path: %w", err)
	}
	if rel == "." {
		return "", fmt.Errorf("report path must be a file inside repository, not repository root: %s", report)
	}
	if strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
		return "", fmt.Errorf("absolute report path must be inside repository: %s", report)
	}
	return filepath.ToSlash(filepath.Join("/workspace", rel)), nil
}

func dockerHostReportPath(repoRoot, report string) (string, error) {
	if !filepath.IsAbs(report) {
		clean, err := cleanRelativeReportPath(report)
		if err != nil {
			return "", err
		}
		return filepath.Join(repoRoot, clean), nil
	}
	rel, err := filepath.Rel(repoRoot, report)
	if err != nil {
		return "", fmt.Errorf("resolve report path: %w", err)
	}
	if rel == "." {
		return "", fmt.Errorf("report path must be a file inside repository, not repository root: %s", report)
	}
	if strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
		return "", fmt.Errorf("absolute report path must be inside repository: %s", report)
	}
	return report, nil
}

func cleanRelativeReportPath(report string) (string, error) {
	clean := filepath.Clean(filepath.FromSlash(report))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("relative report path must stay inside repository: %s", report)
	}
	return clean, nil
}

func dockerReviewArgs(repoRoot, reportPath string, opts DockerOptions) []string {
	args := []string{
		"run",
		"--rm",
		"--pull", opts.Pull,
		"--user", fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()),
		"-e", "CODEX_API_KEY",
		"-e", "GITHUB_TOKEN",
		"-v", repoRoot + ":/workspace",
		"-w", "/workspace",
		opts.Image,
	}
	reviewOpts := LocalOptions{
		Base:         opts.Base,
		Report:       opts.Report,
		Instructions: opts.Instructions,
		Structured:   opts.Structured,
		Full:         opts.Full,
	}
	return append(args, append([]string{"codex"}, dockerCodexReviewArgs(reviewOpts, reportPath)...)...)
}

func dockerCodexReviewArgs(opts LocalOptions, reportPath string) []string {
	args := codexReviewArgs(opts, reportPath)
	if len(args) == 0 || args[0] != "exec" {
		return args
	}
	return append([]string{"exec", "--sandbox", "danger-full-access"}, args[1:]...)
}
