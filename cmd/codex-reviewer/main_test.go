package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/everydaydevops/codex-code-reviewer/internal/installer"
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

func TestRunReviewDockerWithFakeDocker(t *testing.T) {
	dir := initTestGitRepo(t)
	t.Chdir(dir)
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
		"review\n",
		"--base\norigin/main\n",
		"--output-last-message\ncodex-review/test.md\n",
		"focus on docker\n",
		"CODEX_API_KEY=test-codex-key\n",
		"GITHUB_TOKEN=test-github-token\n",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("docker args missing %q:\n%s", want, got)
		}
	}
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
