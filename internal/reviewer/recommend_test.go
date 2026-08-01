package reviewer

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRecommendStructuredForRiskyDiff(t *testing.T) {
	dir := initRecommendationRepo(t)
	if err := os.MkdirAll(filepath.Join(dir, "internal", "auth"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "internal", "auth", "token.go"), []byte("package auth\nfunc Token() string { return \"x\" }\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "change auth")

	rec, err := Recommend(context.Background(), RecommendOptions{Dir: dir, Base: "main"})
	if err != nil {
		t.Fatalf("Recommend() error = %v", err)
	}
	if rec.Mode != "structured" {
		t.Fatalf("mode = %q, want structured: %+v", rec.Mode, rec)
	}
	if !strings.Contains(strings.Join(rec.Signals, ","), "security-sensitive files") {
		t.Fatalf("signals = %#v", rec.Signals)
	}
	if rec.TokenRange == "" {
		t.Fatal("TokenRange is empty")
	}
}

func TestRunRecommendPrintsSummary(t *testing.T) {
	dir := initRecommendationRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("updated\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "update docs")
	var out bytes.Buffer
	if err := RunRecommend(context.Background(), RecommendOptions{Dir: dir, Base: "main", Stdout: &out}); err != nil {
		t.Fatalf("RunRecommend() error = %v", err)
	}
	for _, want := range []string{"Review recommendation for main...HEAD", "Changed files: 1", "Recommended mode:", "Estimated token range:"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("output missing %q:\n%s", want, out.String())
		}
	}
}

func initRecommendationRepo(t *testing.T) string {
	t.Helper()
	dir := initGitRepo(t)
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("initial\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "initial")
	runGit(t, dir, "checkout", "-b", "feature")
	return dir
}
