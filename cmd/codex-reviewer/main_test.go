package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/markcallen/codex-reviewer/internal/installer"
)

func TestRunWorkflowRunDryRun(t *testing.T) {
	runWorkflowRun([]string{
		"--dry-run",
		"--commit-message", "Add workflow",
		"--unit-test", "make test",
		"--fix", "codex exec fix",
		"--e2e-test", "make e2e",
		"--push",
	})
	runWorkflow([]string{
		"run",
		"--dry-run",
		"--commit-message", "Add workflow",
	})
}

func TestRunServiceJobManifestWritesOutput(t *testing.T) {
	t.Chdir(t.TempDir())
	output := filepath.Join(t.TempDir(), "job.json")
	runServiceJobManifest([]string{
		"--repo-url", "git@github.com:org/repo.git",
		"--base", "origin/main",
		"--head", "feature",
		"--head-sha", "abc123",
		"--require-clean-tree=false",
		"--review-id", "review-1",
		"--reviewer-image", "reviewer:test",
		"--sidecar-image", "sidecar:test",
		"--openai-secret", "openai-api",
		"--output", output,
	})
	runService([]string{
		"job-manifest",
		"--repo-url", "git@github.com:org/repo.git",
		"--head-sha", "abc123",
		"--require-clean-tree=false",
		"--review-id", "review-2",
		"--reviewer-image", "reviewer:test",
		"--sidecar-image", "sidecar:test",
		"--openai-secret", "openai-api",
		"--output", filepath.Join(t.TempDir(), "job-wrapper.json"),
	})

	data := readFile(t, output)
	for _, want := range []string{`"kind": "Job"`, `"name": "codex-review-review-1"`, `"image": "reviewer:test"`} {
		if !strings.Contains(data, want) {
			t.Fatalf("manifest missing %q:\n%s", want, data)
		}
	}
}

func TestRunServiceAPIConfiguresServer(t *testing.T) {
	oldListenAndServe := listenAndServe
	t.Cleanup(func() { listenAndServe = oldListenAndServe })
	called := false
	listenAndServe = func(addr string, handler http.Handler) error {
		called = true
		if addr != "127.0.0.1:0" {
			t.Fatalf("addr = %q", addr)
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("health status = %d", rec.Code)
		}
		return nil
	}

	runServiceAPI([]string{
		"--listen", "127.0.0.1:0",
		"--reviewer-image", "reviewer:test",
		"--sidecar-image", "sidecar:test",
		"--openai-secret", "openai-api",
	})
	if !called {
		t.Fatal("listenAndServe was not called")
	}
}

func TestRunServiceSubmitDryRunWritesOutput(t *testing.T) {
	t.Chdir(t.TempDir())
	output := filepath.Join(t.TempDir(), "request.json")
	runServiceSubmit([]string{
		"--dry-run",
		"--repo-url", "git@github.com:org/repo.git",
		"--base", "origin/main",
		"--head", "feature",
		"--head-sha", "abc123",
		"--require-clean-tree=false",
		"--output", output,
	})

	data := readFile(t, output)
	for _, want := range []string{`"repo_url": "git@github.com:org/repo.git"`, `"base_ref": "origin/main"`, `"profile": "standard"`} {
		if !strings.Contains(data, want) {
			t.Fatalf("request missing %q:\n%s", want, data)
		}
	}
	if !strings.Contains(data, `"usage_estimate"`) || !strings.Contains(data, `"cost_estimate"`) {
		t.Fatalf("request missing usage estimate:\n%s", data)
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(data), &decoded); err != nil {
		t.Fatalf("dry-run output is not JSON: %v\n%s", err, data)
	}
	if decoded["repo_url"] == nil || decoded["usage_estimate"] == nil {
		t.Fatalf("dry-run output missing request fields or estimate: %#v", decoded)
	}
}

func TestRunServiceTelemetryConfiguresServer(t *testing.T) {
	oldListenAndServe := listenAndServe
	t.Cleanup(func() { listenAndServe = oldListenAndServe })
	called := false
	listenAndServe = func(addr string, handler http.Handler) error {
		called = true
		if addr != "127.0.0.1:0" {
			t.Fatalf("addr = %q", addr)
		}
		req := httptest.NewRequest(http.MethodGet, "/telemetry/v1/spend", nil)
		req.Header.Set("Authorization", "Bearer test-token")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("spend status = %d, body = %s", rec.Code, rec.Body.String())
		}
		return nil
	}

	runServiceTelemetry([]string{"--listen", "127.0.0.1:0", "--token", "test-token"})
	if !called {
		t.Fatal("listenAndServe was not called")
	}
}

func TestRunServiceSubmitWaitWritesReport(t *testing.T) {
	t.Chdir(t.TempDir())
	reportPath := filepath.Join(t.TempDir(), "report.md")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/reviews":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"id":"review-1","status":"submitted","profile":"standard","report_url":"/reviews/review-1/report"}`)
		case "/reviews/review-1/report":
			fmt.Fprint(w, "No blocking findings\n")
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	runServiceSubmit([]string{
		"--api-url", server.URL,
		"--wait",
		"--timeout", "1s",
		"--repo-url", "git@github.com:org/repo.git",
		"--head-sha", "abc123",
		"--require-clean-tree=false",
		"--output", reportPath,
	})
	if got := readFile(t, reportPath); got != "No blocking findings\n" {
		t.Fatalf("report = %q", got)
	}
}

func TestRunServiceRunnerWithFakeCommands(t *testing.T) {
	binDir := t.TempDir()
	writeExecutable(t, filepath.Join(binDir, "git"), `#!/bin/sh
exit 0
`)
	writeExecutable(t, filepath.Join(binDir, "codex"), `#!/bin/sh
while [ "$#" -gt 0 ]; do
  if [ "$1" = "--output-last-message" ]; then
    shift
    printf 'Approve with fixes\n' > "$1"
    exit 0
  fi
  shift
done
exit 1
`)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("HTTPS_PROXY", "")
	t.Setenv("HTTP_PROXY", "")
	t.Setenv("ALL_PROXY", "")
	requestJSON := `{"repo_url":"git@github.com:org/repo.git","base_ref":"origin/main","head_ref":"feature","head_sha":"abc123","profile":"standard","profile_config":{"name":"standard","agent":"code_reviewer","model":"gpt-5.5","reasoning_effort":"high","prompt":"review-branch","timeout":"30m"},"return_format":"markdown"}`
	t.Setenv("REVIEW_REQUEST_JSON", requestJSON)
	t.Setenv("REVIEW_ID", "review-cli")
	t.Setenv("REVIEW_WORKSPACE", filepath.Join(t.TempDir(), "workspace"))
	out := t.TempDir()
	t.Setenv("REVIEW_OUTPUT_DIR", out)

	runServiceRunner(nil)
	if got := readFile(t, filepath.Join(out, "review.md")); !strings.Contains(got, "Approve with fixes") {
		t.Fatalf("review report = %q", got)
	}
}

func TestRunReviewLocalDryRun(t *testing.T) {
	dir := initTestGitRepo(t)
	t.Chdir(dir)

	runReviewLocal([]string{
		"--dry-run",
		"--base", "origin/main",
		"--report", "codex-review/test.md",
	})
	runReview([]string{
		"local",
		"--dry-run",
		"--base", "origin/main",
		"--report", "codex-review/wrapper.md",
	})
}

func TestRunReviewLocalUsesConfigBaseAndFlagOverride(t *testing.T) {
	dir := initTestGitRepo(t)
	t.Chdir(dir)
	writeFile(t, filepath.Join(dir, ".codex-reviewer.toml"), `version = "`+version+`"

[review]
base = "origin/config"
profile = "pr-readiness"
`)

	out := captureStdout(t, func() {
		runReviewLocal([]string{"--dry-run"})
	})
	if !strings.Contains(out, "Review this branch against origin/config") {
		t.Fatalf("config base not used:\n%s", out)
	}
	if !strings.Contains(out, "profile: pr-readiness") {
		t.Fatalf("config profile not used:\n%s", out)
	}

	out = captureStdout(t, func() {
		runReviewLocal([]string{"--dry-run", "--base", "origin/flag", "--profile", "standard"})
	})
	if !strings.Contains(out, "Review this branch against origin/flag") {
		t.Fatalf("flag base not used:\n%s", out)
	}
	if !strings.Contains(out, "profile: standard") {
		t.Fatalf("flag profile not used:\n%s", out)
	}
}

func TestRunReviewRecommendDryRun(t *testing.T) {
	dir := initTestGitRepo(t)
	t.Chdir(dir)
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Test User")
	writeFile(t, filepath.Join(dir, "README.md"), "initial\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "initial")
	writeFile(t, filepath.Join(dir, "README.md"), "changed\n")

	out := captureStdout(t, func() {
		runReviewRecommend([]string{"--base", "main"})
	})
	if !strings.Contains(out, "Recommended mode:") {
		t.Fatalf("recommend output missing mode:\n%s", out)
	}

	out = captureStdout(t, func() {
		runReviewLocal([]string{"--recommend", "--base", "main"})
	})
	if !strings.Contains(out, "Review recommendation for main...HEAD") {
		t.Fatalf("local recommend flag output missing summary:\n%s", out)
	}
}

func TestRunReviewPrePushRecommendUsesPrePushBase(t *testing.T) {
	dir := initTestGitRepo(t)
	t.Chdir(dir)
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Test User")
	writeFile(t, filepath.Join(dir, "README.md"), "initial\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "initial")
	// Create a local "release" branch at the initial commit so it is a valid git ref.
	runGit(t, dir, "branch", "release")
	// Advance main with another commit so release...HEAD has a diff.
	writeFile(t, filepath.Join(dir, "README.md"), "updated\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "update")

	writeFile(t, filepath.Join(dir, ".codex-reviewer.toml"), `version = "`+version+`"

[review]
base = "main"

[review.pre_push]
base = "release"
require_clean_tree = false
`)

	// Without --base flag the recommend path should prefer PrePushBase over review.base.
	out := captureStdout(t, func() {
		runReviewPrePush([]string{"--recommend"})
	})
	if !strings.Contains(out, "Review recommendation for release...HEAD") {
		t.Fatalf("pre-push --recommend should use pre_push base, got:\n%s", out)
	}
	if strings.Contains(out, "Review recommendation for main...HEAD") {
		t.Fatalf("pre-push --recommend should not fall back to review.base, got:\n%s", out)
	}
}

func TestRunReviewPrePushRecommendFlagBaseOverridesConfig(t *testing.T) {
	dir := initTestGitRepo(t)
	t.Chdir(dir)
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Test User")
	writeFile(t, filepath.Join(dir, "README.md"), "initial\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "initial")
	// Create a local "release" branch — this is what config specifies.
	runGit(t, dir, "branch", "release")

	writeFile(t, filepath.Join(dir, ".codex-reviewer.toml"), `version = "`+version+`"

[review.pre_push]
base = "release"
require_clean_tree = false
`)

	// Explicit --base flag should win over configured pre_push base.
	// Use "main" as the explicit base since it exists as a local branch.
	out := captureStdout(t, func() {
		runReviewPrePush([]string{"--recommend", "--base", "main"})
	})
	if !strings.Contains(out, "Review recommendation for main...HEAD") {
		t.Fatalf("--base flag should override config, got:\n%s", out)
	}
}

func TestRunReviewDockerDryRun(t *testing.T) {
	dir := initTestGitRepo(t)
	t.Chdir(dir)

	runReviewDocker([]string{
		"--dry-run",
		"--image", "reviewer:test",
		"--pull", "never",
		"--base", "origin/main",
		"--report", "codex-review/test.md",
	})
	runReview([]string{
		"docker",
		"--dry-run",
		"--image", "reviewer:test",
		"--pull", "never",
		"--base", "origin/main",
		"--report", "codex-review/wrapper.md",
	})
}

func TestDefaultReviewReportUsesBranchReportForBaseReview(t *testing.T) {
	got := defaultReviewReport("", "codex-review/full-review.md", "origin/main", false)
	if got != "codex-review/branch-review.md" {
		t.Fatalf("defaultReviewReport() = %q, want branch report", got)
	}
}

func TestDefaultReviewReportKeepsFullReportForFullReview(t *testing.T) {
	got := defaultReviewReport("", "codex-review/full-review.md", "origin/main", true)
	if got != "codex-review/full-review.md" {
		t.Fatalf("defaultReviewReport() = %q, want full report", got)
	}
}

func TestDefaultReviewReportKeepsExplicitReport(t *testing.T) {
	got := defaultReviewReport("codex-review/custom.md", "codex-review/full-review.md", "origin/main", false)
	if got != "codex-review/custom.md" {
		t.Fatalf("defaultReviewReport() = %q, want explicit report", got)
	}
}

func TestDefaultReviewReportKeepsCustomConfigReport(t *testing.T) {
	got := defaultReviewReport("", "codex-review/custom.md", "origin/main", false)
	if got != "codex-review/custom.md" {
		t.Fatalf("defaultReviewReport() = %q, want custom config report", got)
	}
}

func TestRunReviewDockerWithFakeDocker(t *testing.T) {
	dir := initTestGitRepo(t)
	t.Chdir(dir)
	installTestGlobalSetup(t)
	binDir := t.TempDir()
	argsFile := filepath.Join(t.TempDir(), "docker-args.txt")
	writeExecutable(t, filepath.Join(binDir, "docker"), `#!/bin/sh
printf '%s\n' "$@" > "$DOCKER_ARGS_FILE"
printf 'CODEX_API_KEY=%s\nGITHUB_TOKEN=%s\n' "$CODEX_API_KEY" "$GITHUB_TOKEN" >> "$DOCKER_ARGS_FILE"
`)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("DOCKER_ARGS_FILE", argsFile)
	t.Setenv("CODEX_API_KEY", "test-codex-key")
	t.Setenv("GITHUB_TOKEN", "test-github-token")

	runReviewDocker([]string{
		"--image", "reviewer:test",
		"--pull", "never",
		"--base", "origin/main",
		"--report", "codex-review/test.md",
		"--instructions", "focus on docker",
	})

	got := readFile(t, argsFile)
	for _, want := range []string{
		"run\n",
		"--pull\nnever\n",
		"-e\nCODEX_API_KEY\n",
		"-e\nGITHUB_TOKEN\n",
		"-v\n" + dir + ":/workspace\n",
		"reviewer:test\n",
		"codex\nexec\n",
		"--sandbox\ndanger-full-access\n",
		"--output-last-message\ncodex-review/test.md\n",
		"Review this branch against origin/main. Inspect the diff with `git diff origin/main...HEAD` and read relevant surrounding code before writing the report. Do not edit files.\n",
		"profile: standard\n",
		"focus on docker\n",
		"CODEX_API_KEY=test-codex-key\n",
		"GITHUB_TOKEN=test-github-token\n",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("docker args missing %q:\n%s", want, got)
		}
	}
}

func installTestGlobalSetup(t *testing.T) {
	t.Helper()
	codexHome := filepath.Join(t.TempDir(), "codex-home")
	if _, err := installer.InstallGlobal(installer.GlobalOptions{CodexHome: codexHome, Version: version}); err != nil {
		t.Fatalf("InstallGlobal() error = %v", err)
	}
	t.Setenv("CODEX_HOME", codexHome)
}

func TestRunReviewPrePushDryRun(t *testing.T) {
	dir := initTestGitRepo(t)
	t.Chdir(dir)
	writeFile(t, filepath.Join(dir, ".codex-reviewer.toml"), `version = "`+version+`"

[review.pre_push]
base = "origin/main"
block_on = "never"
require_clean_tree = false
`)

	runReviewPrePush([]string{
		"--dry-run",
		"--allow-dirty",
		"--base", "origin/main",
	})
}

func TestRunInstallSetupAndDoctor(t *testing.T) {
	target := t.TempDir()
	runInstall([]string{"--dry-run", target})
	runInstall([]string{"--quiet", target})
	runDoctor([]string{target})
	t.Chdir(target)
	runDoctor(nil)

	codexHome := filepath.Join(t.TempDir(), "codex-home")
	runSetup([]string{"--dry-run", "--codex-home", codexHome})
	runSetup([]string{"--yes", "--codex-home", codexHome})
}

func TestPrintHelpersAndPredicates(t *testing.T) {
	if allSkipped(nil) {
		t.Fatal("allSkipped(nil) = true, want false")
	}
	if !allSkipped([]installer.Action{{Status: "skip"}}) {
		t.Fatal("allSkipped(skip-only) = false, want true")
	}
	if got := firstNonEmpty("", "fallback"); got != "fallback" {
		t.Fatalf("firstNonEmpty() = %q", got)
	}
	if got := defaultDockerImageForVersion("v1.2.3"); got != "ghcr.io/markcallen/codex-reviewer:v1.2.3" {
		t.Fatalf("defaultDockerImageForVersion(v1.2.3) = %q", got)
	}
	if got := defaultDockerImageForVersion("v1.2.3-4-gabc1234-dirty"); got != "ghcr.io/markcallen/codex-reviewer:v1.2.3" {
		t.Fatalf("defaultDockerImageForVersion(describe dirty) = %q", got)
	}
	if got := defaultDockerImageForVersion("v1.2.3-rc.1"); got != "ghcr.io/markcallen/codex-reviewer:v1.2.3-rc.1" {
		t.Fatalf("defaultDockerImageForVersion(prerelease) = %q", got)
	}
	if got := defaultDockerImageForVersion("v1.2.3-dirty"); got != "ghcr.io/markcallen/codex-reviewer:v1.2.3" {
		t.Fatalf("defaultDockerImageForVersion(exact dirty) = %q", got)
	}
	if got := defaultDockerImageForVersion("dev"); got != "ghcr.io/markcallen/codex-reviewer:latest" {
		t.Fatalf("defaultDockerImageForVersion(dev) = %q", got)
	}
	printWarnings([]string{"be careful"})
	usage()
	reviewUsage()
	workflowUsage()
	serviceUsage()
	if !testConfirm(t, "yes\n") {
		t.Fatal("confirm(yes) = false")
	}
	if testConfirm(t, "no\n") {
		t.Fatal("confirm(no) = true")
	}

	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	os.Args = []string{"codex-reviewer", "version"}
	main()
}

func TestMainDispatchesWorkflow(t *testing.T) {
	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	os.Args = []string{"codex-reviewer", "workflow", "run", "--dry-run", "--commit-message", "Add dispatch"}
	main()
}

func initTestGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init", "-b", "main")
	return dir
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", path, err)
	}
}

func writeExecutable(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", path, err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	return string(data)
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	oldStdout := os.Stdout
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatalf("Pipe() error = %v", err)
	}
	os.Stdout = write
	fn()
	if err := write.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	os.Stdout = oldStdout
	data, err := io.ReadAll(read)
	if err != nil {
		t.Fatalf("ReadAll(stdout) error = %v", err)
	}
	_ = read.Close()
	return string(data)
}

func testConfirm(t *testing.T, input string) bool {
	t.Helper()
	oldStdin := os.Stdin
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatalf("Pipe() error = %v", err)
	}
	if _, err := write.WriteString(input); err != nil {
		t.Fatalf("WriteString() error = %v", err)
	}
	if err := write.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	os.Stdin = read
	t.Cleanup(func() {
		os.Stdin = oldStdin
		_ = read.Close()
	})
	return confirm("Continue? ")
}
