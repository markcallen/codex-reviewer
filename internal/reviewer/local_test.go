package reviewer

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalReviewArgsFullReviewByDefault(t *testing.T) {
	args := localReviewArgs(LocalOptions{}, "codex-review/full-review.md")
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "exec --output-last-message codex-review/full-review.md") {
		t.Fatalf("args missing full review output path: %v", args)
	}
	if strings.Contains(joined, "review --base") {
		t.Fatalf("full review should not use codex review subcommand: %v", args)
	}
	if !strings.Contains(joined, "full code review") {
		t.Fatalf("full review prompt missing: %v", args)
	}
}

func TestLocalReviewArgsDiffReviewWithBase(t *testing.T) {
	args := localReviewArgs(LocalOptions{Base: "origin/main"}, "codex-review/branch-review.md")
	joined := strings.Join(args, " ")
	for _, want := range []string{"exec review", "--base origin/main", "--output-last-message codex-review/branch-review.md"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("args missing %q: %v", want, args)
		}
	}
}

func TestLocalReviewArgsDiffReviewWithInstructions(t *testing.T) {
	args := localReviewArgs(LocalOptions{Base: "origin/main", Instructions: "Focus on security."}, "codex-review/branch-review.md")
	joined := strings.Join(args, " ")
	for _, want := range []string{"exec review", "--base origin/main", "--output-last-message", "Focus on security."} {
		if !strings.Contains(joined, want) {
			t.Fatalf("args missing %q: %v", want, args)
		}
	}
}

func TestLocalReviewArgsFullOverridesBase(t *testing.T) {
	args := localReviewArgs(LocalOptions{Base: "origin/main", Full: true}, "codex-review/full-review.md")
	joined := strings.Join(args, " ")
	if strings.Contains(joined, "review --base") {
		t.Fatalf("--full should force full review: %v", args)
	}
}

func TestLocalReviewArgsCanBypassSandbox(t *testing.T) {
	args := localReviewArgs(LocalOptions{Base: "origin/main", BypassSandbox: true}, "codex-review/branch-review.md")
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "exec --dangerously-bypass-approvals-and-sandbox review --base origin/main") {
		t.Fatalf("args missing sandbox bypass: %v", args)
	}
}

func TestRunLocalDryRun(t *testing.T) {
	dir := initLocalGitRepo(t)
	var stdout bytes.Buffer
	err := RunLocal(context.Background(), LocalOptions{
		Dir:    dir,
		Base:   "origin/main",
		Report: "codex-review/test.md",
		DryRun: true,
		Stdout: &stdout,
	})
	if err != nil {
		t.Fatalf("RunLocal() error = %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "codex-reviewer: dry run: codex exec review --base origin/main") {
		t.Fatalf("dry-run output missing command:\n%s", out)
	}
}

func TestRunLocalRunsCodex(t *testing.T) {
	dir := initLocalGitRepo(t)
	binDir := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "codex.log")
	writeLocalExecutable(t, filepath.Join(binDir, "codex"), `#!/bin/sh
echo "$@" > "`+logPath+`"
while [ "$#" -gt 0 ]; do
  if [ "$1" = "--output-last-message" ]; then
    shift
    printf 'No blocking findings\n' > "$1"
    exit 0
  fi
  shift
done
exit 1
`)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	err := RunLocal(context.Background(), LocalOptions{
		Dir:    dir,
		Base:   "origin/main",
		Report: "codex-review/test.md",
		Stdout: &bytes.Buffer{},
	})
	if err != nil {
		t.Fatalf("RunLocal() error = %v", err)
	}
	if got := readLocalFile(t, filepath.Join(dir, "codex-review/test.md")); got != "No blocking findings\n" {
		t.Fatalf("report = %q", got)
	}
	if got := readLocalFile(t, logPath); !strings.Contains(got, "exec review --base origin/main") {
		t.Fatalf("codex args = %q", got)
	}
}

func initLocalGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cmd := exec.Command("git", "init", "-b", "main")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v\n%s", err, out)
	}
	return dir
}

func writeLocalExecutable(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", path, err)
	}
}

func readLocalFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	return string(data)
}
