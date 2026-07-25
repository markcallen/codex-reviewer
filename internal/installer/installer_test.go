package installer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallCreatesFreshProjectFiles(t *testing.T) {
	dir := t.TempDir()

	result, err := Install(Options{TargetDir: dir})
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	requireFileContains(t, dir, ".codex/config.toml", `model = "gpt-5.5"`)
	requireFileContains(t, dir, ".codex-reviewer.toml", `version = "dev"`)
	requireFileContains(t, dir, ".codex/agents/code-reviewer.toml", `name = "code_reviewer"`)
	requireFileContains(t, dir, "AGENTS.md", "Project review expectations")
	requireFileContains(t, dir, "docs/code_review.md", "Code review checklist for Codex")
	requireFileContains(t, dir, "prompts/review-branch.md", "Review this branch against main")

	if len(result.Actions) == 0 {
		t.Fatal("Install() returned no actions")
	}
}

func TestInstallCreatesReviewerConfigWithRuntimeVersion(t *testing.T) {
	dir := t.TempDir()

	if _, err := Install(Options{TargetDir: dir, Version: "v1.2.3"}); err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	requireFileContains(t, dir, ".codex-reviewer.toml", `version = "v1.2.3"`)
	requireFileContains(t, dir, ".codex-reviewer.toml", `[review.pre_push]`)
}

func TestInstallMergesExistingConfigAndAGENTS(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".codex/config.toml", `model = "gpt-5"

[agents]
max_threads = 2
`)
	writeFile(t, dir, "AGENTS.md", `# Existing guidance

Do project-specific things.
`)

	if _, err := Install(Options{TargetDir: dir}); err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	config := readFile(t, dir, ".codex/config.toml")
	assertContains(t, config, `model = "gpt-5"`)
	assertContains(t, config, `review_model = "gpt-5.5"`)
	assertContains(t, config, `max_threads = 2`)
	assertContains(t, config, `max_depth = 1`)
	if strings.Count(config, "[agents]") != 1 {
		t.Fatalf("config should contain one [agents] table, got:\n%s", config)
	}

	agents := readFile(t, dir, "AGENTS.md")
	assertContains(t, agents, "Do project-specific things.")
	assertContains(t, agents, "BEGIN CODEX REVIEWER INSTALLER: agents-review-expectations")
	assertContains(t, agents, "Follow `docs/code_review.md` for code reviews.")
}

func TestInstallIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	if _, err := Install(Options{TargetDir: dir}); err != nil {
		t.Fatalf("first Install() error = %v", err)
	}
	if _, err := Install(Options{TargetDir: dir}); err != nil {
		t.Fatalf("second Install() error = %v", err)
	}

	agents := readFile(t, dir, "AGENTS.md")
	if strings.Count(agents, "Project review expectations") != 1 {
		t.Fatalf("AGENTS.md should not duplicate review guidance:\n%s", agents)
	}

	doc := readFile(t, dir, "docs/code_review.md")
	if strings.Count(doc, "Code review checklist for Codex") != 1 {
		t.Fatalf("docs/code_review.md should not duplicate checklist:\n%s", doc)
	}
}

func TestInstallUpdatesExistingReviewerConfigVersion(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".codex-reviewer.toml", `version = "v1.0.0"

[review.pre_push]
base = "origin/main"
`)

	if _, err := Install(Options{TargetDir: dir, Version: "v2.0.0"}); err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	requireFileContains(t, dir, ".codex-reviewer.toml", `version = "v2.0.0"`)
	requireFileContains(t, dir, ".codex-reviewer.toml", `base = "origin/main"`)
}

func TestInstallDryRunDoesNotWrite(t *testing.T) {
	dir := t.TempDir()
	result, err := Install(Options{TargetDir: dir, DryRun: true})
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if len(result.Actions) == 0 {
		t.Fatal("dry run returned no planned actions")
	}
	if _, err := os.Stat(filepath.Join(dir, ".codex")); !os.IsNotExist(err) {
		t.Fatalf("dry run should not create .codex, stat err = %v", err)
	}
}

func TestDoctorReportsInstalledProjectOK(t *testing.T) {
	dir := t.TempDir()
	if _, err := Install(Options{TargetDir: dir}); err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	report, err := Doctor(DoctorOptions{TargetDir: dir})
	if err != nil {
		t.Fatalf("Doctor() error = %v", err)
	}
	if !report.OK {
		t.Fatalf("Doctor() OK = false, checks = %#v", report.Checks)
	}
}

func TestDoctorReportsIncompleteProject(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".codex/config.toml", `model = "gpt-5"
`)

	report, err := Doctor(DoctorOptions{TargetDir: dir})
	if err != nil {
		t.Fatalf("Doctor() error = %v", err)
	}
	if report.OK {
		t.Fatalf("Doctor() OK = true for incomplete project, checks = %#v", report.Checks)
	}
	if !hasCheckStatus(report.Checks, ".codex/config.toml", "incomplete") {
		t.Fatalf("Doctor() did not report incomplete config: %#v", report.Checks)
	}
	if !hasCheckStatus(report.Checks, ".codex-reviewer.toml", "missing") {
		t.Fatalf("Doctor() did not report missing codex-reviewer config: %#v", report.Checks)
	}
	if !hasCheckStatus(report.Checks, "AGENTS.md", "missing") {
		t.Fatalf("Doctor() did not report missing AGENTS.md: %#v", report.Checks)
	}
}

func TestDoctorReportsReviewerVersionMismatch(t *testing.T) {
	dir := t.TempDir()
	if _, err := Install(Options{TargetDir: dir, Version: "v1.0.0"}); err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	report, err := Doctor(DoctorOptions{TargetDir: dir, Version: "v2.0.0"})
	if err != nil {
		t.Fatalf("Doctor() error = %v", err)
	}
	if report.OK {
		t.Fatalf("Doctor() OK = true for version mismatch, checks = %#v", report.Checks)
	}
	if !hasCheckStatus(report.Checks, ".codex-reviewer.toml", "mismatch") {
		t.Fatalf("Doctor() did not report reviewer version mismatch: %#v", report.Checks)
	}
}

func requireFileContains(t *testing.T, dir, rel, want string) {
	t.Helper()
	assertContains(t, readFile(t, dir, rel), want)
}

func assertContains(t *testing.T, got, want string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Fatalf("content missing %q:\n%s", want, got)
	}
}

func readFile(t *testing.T, dir, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", rel, err)
	}
	return string(data)
}

func writeFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", rel, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", rel, err)
	}
}

func hasCheckStatus(checks []Check, path, status string) bool {
	for _, check := range checks {
		if check.Path == path && check.Status == status {
			return true
		}
	}
	return false
}
