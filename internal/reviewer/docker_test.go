package reviewer

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunDockerRequiresCredentials(t *testing.T) {
	t.Setenv("CODEX_API_KEY", "")
	t.Setenv("GITHUB_TOKEN", "")

	err := RunDocker(context.Background(), DockerOptions{Dir: t.TempDir()})
	if err == nil {
		t.Fatal("RunDocker() error = nil, want missing env error")
	}
	for _, want := range []string{"CODEX_API_KEY", "GITHUB_TOKEN"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("RunDocker() error missing %s: %v", want, err)
		}
	}
}

func TestRunDockerDryRunSkipsCredentialCheck(t *testing.T) {
	t.Setenv("CODEX_API_KEY", "")
	t.Setenv("GITHUB_TOKEN", "")

	err := RunDocker(context.Background(), DockerOptions{Dir: t.TempDir(), DryRun: true})
	if err == nil {
		t.Fatal("RunDocker() error = nil, want git repository error")
	}
	if strings.Contains(err.Error(), "CODEX_API_KEY") || strings.Contains(err.Error(), "GITHUB_TOKEN") {
		t.Fatalf("RunDocker() dry-run checked credentials: %v", err)
	}
}

func TestDockerCodexReviewArgsUseDockerSandboxBoundary(t *testing.T) {
	args := dockerCodexReviewArgs(LocalOptions{Base: "origin/main"}, "codex-review/branch-review.md")
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "exec --sandbox danger-full-access review --base origin/main") {
		t.Fatalf("docker codex args missing Docker sandbox boundary config: %v", args)
	}
	if strings.Contains(joined, "--dangerously-bypass-approvals-and-sandbox") {
		t.Fatalf("docker codex args use broad bypass flag: %v", args)
	}
}

func TestDockerCodexReviewArgsStructuredDiffReview(t *testing.T) {
	args := dockerCodexReviewArgs(LocalOptions{Base: "origin/main", Structured: true}, "codex-review/branch-review.md")
	joined := strings.Join(args, " ")
	for _, want := range []string{
		"exec --sandbox danger-full-access --output-last-message",
		"Review this branch against origin/main.",
		"git diff origin/main...HEAD",
		"Do a structured code review.",
		"Diff summary",
		"Areas checked",
		"Areas not checked / limits",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("docker codex args missing %q: %v", want, args)
		}
	}
	if strings.Contains(joined, "review --base") {
		t.Fatalf("structured docker review should use prompt path because codex review rejects --base with prompt: %v", args)
	}
}

func TestDockerReportPaths(t *testing.T) {
	repoRoot := filepath.Join(t.TempDir(), "repo")
	inside := filepath.Join(repoRoot, "codex-review", "branch-review.md")
	outside := filepath.Join(t.TempDir(), "branch-review.md")

	containerPath, err := dockerContainerReportPath(repoRoot, "codex-review/branch-review.md")
	if err != nil {
		t.Fatalf("dockerContainerReportPath(relative) error = %v", err)
	}
	if containerPath != "codex-review/branch-review.md" {
		t.Fatalf("container relative path = %q", containerPath)
	}

	containerPath, err = dockerContainerReportPath(repoRoot, inside)
	if err != nil {
		t.Fatalf("dockerContainerReportPath(abs inside) error = %v", err)
	}
	if containerPath != "/workspace/codex-review/branch-review.md" {
		t.Fatalf("container absolute path = %q", containerPath)
	}

	hostPath, err := dockerHostReportPath(repoRoot, "codex-review/branch-review.md")
	if err != nil {
		t.Fatalf("dockerHostReportPath(relative) error = %v", err)
	}
	if hostPath != filepath.Join(repoRoot, "codex-review", "branch-review.md") {
		t.Fatalf("host relative path = %q", hostPath)
	}

	hostPath, err = dockerHostReportPath(repoRoot, inside)
	if err != nil {
		t.Fatalf("dockerHostReportPath(abs inside) error = %v", err)
	}
	if hostPath != inside {
		t.Fatalf("host absolute path = %q", hostPath)
	}

	if _, err := dockerContainerReportPath(repoRoot, outside); err == nil {
		t.Fatal("dockerContainerReportPath(abs outside) error = nil")
	}
	if _, err := dockerHostReportPath(repoRoot, outside); err == nil {
		t.Fatal("dockerHostReportPath(abs outside) error = nil")
	}
	if _, err := dockerContainerReportPath(repoRoot, repoRoot); err == nil {
		t.Fatal("dockerContainerReportPath(repo root) error = nil")
	}
	if _, err := dockerHostReportPath(repoRoot, repoRoot); err == nil {
		t.Fatal("dockerHostReportPath(repo root) error = nil")
	}
	if _, err := dockerContainerReportPath(repoRoot, "../review.md"); err == nil {
		t.Fatal("dockerContainerReportPath(relative outside) error = nil")
	}
	if _, err := dockerHostReportPath(repoRoot, "../review.md"); err == nil {
		t.Fatal("dockerHostReportPath(relative outside) error = nil")
	}
}

func TestDockerReviewArgsFullReview(t *testing.T) {
	repoRoot := filepath.Join(t.TempDir(), "repo")
	args := dockerReviewArgs(repoRoot, "codex-review/full-review.md", DockerOptions{
		Image:        "reviewer:test",
		Pull:         "never",
		Instructions: "Review everything.",
		Full:         true,
	})
	joined := strings.Join(args, "\n")
	for _, want := range []string{
		"run\n",
		"--pull\nnever\n",
		"-e\nCODEX_API_KEY\n",
		"-e\nGITHUB_TOKEN\n",
		"-v\n" + repoRoot + ":/workspace\n",
		"reviewer:test\n",
		"codex\nexec\n",
		"--sandbox\ndanger-full-access\n",
		"--output-last-message\ncodex-review/full-review.md\n",
		"Review everything.",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("docker args missing %q:\n%s", want, joined)
		}
	}
	if strings.Contains(joined, "\nreview\n--base\n") {
		t.Fatalf("full docker review should not use codex review subcommand:\n%s", joined)
	}
}

func TestRunDockerDryRunBuildsCommand(t *testing.T) {
	dir := initLocalGitRepo(t)
	t.Setenv("CODEX_API_KEY", "")
	t.Setenv("GITHUB_TOKEN", "")

	var stdout bytes.Buffer
	err := RunDocker(context.Background(), DockerOptions{
		Dir:    dir,
		Image:  "reviewer:test",
		Pull:   "never",
		Base:   "origin/main",
		Report: "codex-review/branch-review.md",
		DryRun: true,
		Stdout: &stdout,
	})
	if err != nil {
		t.Fatalf("RunDocker(dry-run) error = %v", err)
	}
	out := stdout.String()
	for _, want := range []string{
		"codex-reviewer: running Docker review with reviewer:test",
		"docker run --rm --pull never",
		"codex exec --sandbox danger-full-access review --base origin/main",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("dry-run output missing %q:\n%s", want, out)
		}
	}
}

func TestRunDockerCreatesReportDirectory(t *testing.T) {
	dir := initLocalGitRepo(t)
	binDir := t.TempDir()
	report := filepath.Join("codex-review", "branch-review.md")
	writeLocalExecutable(t, filepath.Join(binDir, "docker"), `#!/bin/sh
exit 0
`)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("CODEX_API_KEY", "test-codex-key")
	t.Setenv("GITHUB_TOKEN", "test-github-token")

	err := RunDocker(context.Background(), DockerOptions{
		Dir:    dir,
		Image:  "reviewer:test",
		Pull:   "never",
		Base:   "origin/main",
		Report: report,
		Stdout: &bytes.Buffer{},
		Stderr: &bytes.Buffer{},
	})
	if err != nil {
		t.Fatalf("RunDocker() error = %v", err)
	}
	if info, err := os.Stat(filepath.Join(dir, "codex-review")); err != nil || !info.IsDir() {
		t.Fatalf("report directory not created: info=%v err=%v", info, err)
	}
}
