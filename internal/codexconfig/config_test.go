package codexconfig

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadReviewerConfigReadsCodexReviewerSection(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CODEX_HOME", dir)
	data := []byte(`model = "gpt-5.5"

[codex_reviewer]
backend = "k8s"
report = "codex-review/custom.md"
k8s_api_url = "http://localhost:8080"
`)
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), data, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cfg := LoadReviewerConfig()
	if cfg.Backend != "k8s" {
		t.Fatalf("Backend = %q, want k8s", cfg.Backend)
	}
	if cfg.Report != "codex-review/custom.md" {
		t.Fatalf("Report = %q, want custom path", cfg.Report)
	}
	if cfg.K8sAPIURL != "http://localhost:8080" {
		t.Fatalf("K8sAPIURL = %q, want configured URL", cfg.K8sAPIURL)
	}
}

func TestLoadReviewerConfigDefaultsToLocal(t *testing.T) {
	t.Setenv("CODEX_HOME", t.TempDir())

	cfg := LoadReviewerConfig()
	if cfg.Backend != "local" {
		t.Fatalf("Backend = %q, want local", cfg.Backend)
	}
	if cfg.Report != "codex-review/full-review.md" {
		t.Fatalf("Report = %q, want default report", cfg.Report)
	}
}
