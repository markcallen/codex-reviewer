package reviewer

import (
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

func TestLocalReviewArgsFullOverridesBase(t *testing.T) {
	args := localReviewArgs(LocalOptions{Base: "origin/main", Full: true}, "codex-review/full-review.md")
	joined := strings.Join(args, " ")
	if strings.Contains(joined, "review --base") {
		t.Fatalf("--full should force full review: %v", args)
	}
}
