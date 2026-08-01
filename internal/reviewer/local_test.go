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
	for _, want := range []string{
		"exec --output-last-message codex-review/branch-review.md",
		"Review this branch against origin/main.",
		"git diff origin/main...HEAD",
		"Review Scope",
		"Diff summary",
		"Areas checked",
		"Areas not checked / limits",
		"Tests to run",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("args missing %q: %v", want, args)
		}
	}
	if strings.Contains(joined, "review --base") {
		t.Fatalf("base review should use prompt path for auditable output: %v", args)
	}
}

func TestLocalReviewArgsDiffReviewWithInstructions(t *testing.T) {
	args := localReviewArgs(LocalOptions{Base: "origin/main", Instructions: "Focus on security."}, "codex-review/branch-review.md")
	joined := strings.Join(args, " ")
	for _, want := range []string{"exec --output-last-message", "Review this branch against origin/main.", "git diff origin/main...HEAD", "Focus on security."} {
		if !strings.Contains(joined, want) {
			t.Fatalf("args missing %q: %v", want, args)
		}
	}
	if strings.Contains(joined, "review --base") {
		t.Fatalf("custom diff review should use prompt path because codex review rejects --base with prompt: %v", args)
	}
}

func TestLocalReviewArgsStructuredDiffReview(t *testing.T) {
	args := localReviewArgs(LocalOptions{Base: "origin/main", Structured: true}, "codex-review/branch-review.md")
	joined := strings.Join(args, " ")
	for _, want := range []string{
		"exec --output-last-message",
		"Review this branch against origin/main.",
		"git diff origin/main...HEAD",
		"Do a structured code review.",
		"Areas checked",
		"Areas not checked / limits",
		"Do not stop after the first few findings.",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("args missing %q: %v", want, args)
		}
	}
	if strings.Contains(joined, "review --base") {
		t.Fatalf("structured diff review should use prompt path because codex review rejects --base with prompt: %v", args)
	}
}

func TestLocalReviewArgsStructuredDiffReviewWithInstructions(t *testing.T) {
	args := localReviewArgs(LocalOptions{
		Base:         "origin/main",
		Instructions: "Pay special attention to Step CA.",
		Structured:   true,
	}, "codex-review/branch-review.md")
	joined := strings.Join(args, " ")
	for _, want := range []string{
		"Do a structured code review.",
		"Additional caller instructions:",
		"Pay special attention to Step CA.",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("args missing %q: %v", want, args)
		}
	}
}

func TestLocalReviewArgsRepoPolicyProfileAndIgnores(t *testing.T) {
	args := localReviewArgs(LocalOptions{
		Base:       "origin/main",
		Ignore:     []string{"dist/**", "*.lock"},
		Directives: []string{"Follow release policy."},
		Profile:    "repo-policy",
		PolicyFile: "docs/review-policy.md",
	}, "codex-review/branch-review.md")
	joined := strings.Join(args, " ")
	for _, want := range []string{
		"git diff origin/main...HEAD -- . :(exclude)dist/** :(exclude)*.lock",
		"Repo-policy profile",
		"Follow release policy.",
		"Review Scope metadata to include in the report:",
		"default policy files to check when present: AGENTS.md, docs/code_review.md",
		"changed policy globs to check when present: .codex/rules/**, .claude/rules/**, AGENTS.md, docs/code_review.md",
		"policy file: docs/review-policy.md",
		"ignored diff globs: dist/**, *.lock",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("args missing %q: %v", want, args)
		}
	}
}

func TestLocalReviewArgsProfiles(t *testing.T) {
	tests := []struct {
		name    string
		profile string
		want    []string
	}{
		{
			name:    "standard",
			profile: "standard",
			want:    []string{"Standard profile", "profile: standard"},
		},
		{
			name:    "pr-readiness",
			profile: "pr-readiness",
			want:    []string{"PR-readiness profile", "introduced TODOs", "hook/workflow setup", "profile: pr-readiness"},
		},
		{
			name:    "strict",
			profile: "strict",
			want:    []string{"Strict profile", "repo-policy passes", "concrete actionable P3", "profile: strict"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := localReviewArgs(LocalOptions{Base: "origin/main", Profile: tt.profile}, "codex-review/branch-review.md")
			joined := strings.Join(args, " ")
			for _, want := range tt.want {
				if !strings.Contains(joined, want) {
					t.Fatalf("args missing %q: %v", want, args)
				}
			}
		})
	}
}

func TestLocalReviewArgsDiffReviewWithInstructionsNoEdit(t *testing.T) {
	args := localReviewArgs(LocalOptions{Base: "origin/main", Instructions: "Focus on security."}, "codex-review/branch-review.md")
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "Do not edit files") {
		t.Fatalf("branch review prompt missing no-edit instruction: %v", args)
	}
}

func TestLocalReviewArgsStructuredDiffReviewNoEdit(t *testing.T) {
	args := localReviewArgs(LocalOptions{Base: "origin/main", Structured: true}, "codex-review/branch-review.md")
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "Do not edit files") {
		t.Fatalf("structured diff review prompt missing no-edit instruction: %v", args)
	}
}

func TestLocalReviewArgsFullOverridesBase(t *testing.T) {
	args := localReviewArgs(LocalOptions{Base: "origin/main", Full: true}, "codex-review/full-review.md")
	joined := strings.Join(args, " ")
	if strings.Contains(joined, "review --base") {
		t.Fatalf("--full should force full review: %v", args)
	}
}

func TestLocalReviewArgsStructuredFullOverridesBase(t *testing.T) {
	args := localReviewArgs(LocalOptions{Base: "origin/main", Full: true, Structured: true}, "codex-review/full-review.md")
	joined := strings.Join(args, " ")
	if strings.Contains(joined, "review --base") {
		t.Fatalf("--full should force full review: %v", args)
	}
	if !strings.Contains(joined, "Do a structured code review.") {
		t.Fatalf("structured full review prompt missing: %v", args)
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
	if !strings.Contains(out, "codex-reviewer: dry run: codex exec --output-last-message") {
		t.Fatalf("dry-run output missing command:\n%s", out)
	}
	if !strings.Contains(out, "Areas checked") {
		t.Fatalf("dry-run output missing base review prompt:\n%s", out)
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
	if got := readLocalFile(t, logPath); !strings.Contains(got, "Review this branch against origin/main") {
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
