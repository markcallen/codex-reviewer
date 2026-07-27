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

func TestRunPrePushRejectsVersionMismatch(t *testing.T) {
	dir := initGitRepo(t)
	writeReviewerConfig(t, dir, `version = "v1.0.0"

[review.pre_push]
base = "origin/main"
`)

	err := RunPrePush(context.Background(), PrePushOptions{
		Dir:        dir,
		Version:    "v2.0.0",
		AllowDirty: true,
		DryRun:     true,
	})
	if err == nil {
		t.Fatal("RunPrePush() error = nil, want version mismatch")
	}
	if !strings.Contains(err.Error(), "version mismatch") {
		t.Fatalf("RunPrePush() error = %v, want version mismatch", err)
	}
}

func TestRunPrePushDryRunUsesConfig(t *testing.T) {
	dir := initGitRepo(t)
	writeReviewerConfig(t, dir, `version = "v1.2.3"

[review.pre_push]
base = "origin/main"
block_on = "never"
report = ".git/codex-review/custom.md"
require_clean_tree = false
`)

	var stdout bytes.Buffer
	err := RunPrePush(context.Background(), PrePushOptions{
		Dir:     dir,
		Version: "v1.2.3",
		DryRun:  true,
		Stdout:  &stdout,
	})
	if err != nil {
		t.Fatalf("RunPrePush() error = %v", err)
	}

	out := stdout.String()
	assertContains(t, out, "running AI code review against origin/main")
	assertContains(t, out, "dry run: codex exec review --base origin/main")
	assertContains(t, out, filepath.Join(dir, ".git/codex-review/custom.md"))
}

func TestResolveBaseFallsBackToMain(t *testing.T) {
	dir := initGitRepo(t)
	if got := resolveBase(context.Background(), dir); got != "main" {
		t.Fatalf("resolveBase() = %q, want main", got)
	}
}

func TestReportHasBlockVerdict(t *testing.T) {
	dir := t.TempDir()
	blockPath := filepath.Join(dir, "block.md")
	if err := os.WriteFile(blockPath, []byte("\n# Block\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	blocked, err := reportHasBlockVerdict(blockPath)
	if err != nil {
		t.Fatalf("reportHasBlockVerdict() error = %v", err)
	}
	if !blocked {
		t.Fatal("reportHasBlockVerdict() = false, want true")
	}

	okPath := filepath.Join(dir, "ok.md")
	if err := os.WriteFile(okPath, []byte("No blocking findings\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	blocked, err = reportHasBlockVerdict(okPath)
	if err != nil {
		t.Fatalf("reportHasBlockVerdict() error = %v", err)
	}
	if blocked {
		t.Fatal("reportHasBlockVerdict() = true, want false")
	}
}

func TestLoadConfigRejectsMissingVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), ConfigFile)
	if err := os.WriteFile(path, []byte("[review.pre_push]\nbase = \"origin/main\"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	_, err := loadConfig(path)
	if err == nil || !strings.Contains(err.Error(), "missing required top-level version") {
		t.Fatalf("loadConfig() error = %v, want missing version", err)
	}
}

func TestRunPrePushRunsCodexAndBlocksOnBlockVerdict(t *testing.T) {
	dir := initGitRepo(t)
	writeReviewerConfig(t, dir, `version = "v1.2.3"

[review.pre_push]
base = "origin/main"
block_on = "block"
require_clean_tree = false
`)
	binDir := t.TempDir()
	writeExecutable(t, filepath.Join(binDir, "codex"), `#!/bin/sh
while [ "$#" -gt 0 ]; do
  if [ "$1" = "--output-last-message" ]; then
    shift
    printf 'Block\n' > "$1"
    exit 0
  fi
  shift
done
exit 1
`)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	err := RunPrePush(context.Background(), PrePushOptions{
		Dir:     dir,
		Version: "v1.2.3",
		Stdout:  &bytes.Buffer{},
	})
	if err == nil || !strings.Contains(err.Error(), "verdict is Block") {
		t.Fatalf("RunPrePush() error = %v, want block verdict", err)
	}
}

func writeExecutable(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", path, err)
	}
}

func initGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init", "-b", "main")
	return dir
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}

func writeReviewerConfig(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, ConfigFile), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", ConfigFile, err)
	}
}

func assertContains(t *testing.T, got, want string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Fatalf("content missing %q:\n%s", want, got)
	}
}
