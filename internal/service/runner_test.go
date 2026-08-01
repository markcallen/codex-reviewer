package service

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type recordedCommand struct {
	Dir  string
	Name string
	Args []string
}

type fakeCommandRunner struct {
	commands   []recordedCommand
	reportPath string
	failName   string
	failErr    error
}

func (r *fakeCommandRunner) Run(_ context.Context, dir, name string, args ...string) error {
	r.commands = append(r.commands, recordedCommand{Dir: dir, Name: name, Args: append([]string(nil), args...)})
	if name == r.failName {
		if r.failErr != nil {
			return r.failErr
		}
		return errFakeCommand
	}
	if name == "codex" {
		for i, arg := range args {
			if arg == "--output-last-message" && i+1 < len(args) {
				r.reportPath = args[i+1]
				return os.WriteFile(r.reportPath, []byte("Approve with fixes\n\n- P1 example\n"), 0o644)
			}
		}
	}
	return nil
}

var errFakeCommand = &fakeError{"fake command failed"}

type fakeError struct {
	message string
}

func (e *fakeError) Error() string {
	return e.message
}

func TestRunReviewJobRunsGitAndCodex(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")
	out := t.TempDir()
	workspace := filepath.Join(t.TempDir(), "workspace")
	req := testRunnerRequest(t)
	requestJSON, err := req.JSON()
	if err != nil {
		t.Fatalf("JSON() error = %v", err)
	}
	runner := &fakeCommandRunner{}
	now := time.Date(2026, 7, 25, 1, 2, 3, 0, time.UTC)
	var stderr bytes.Buffer

	err = RunReviewJob(context.Background(), RunnerOptions{
		ReviewID:    "review-1",
		RequestJSON: string(requestJSON),
		Workspace:   workspace,
		OutputDir:   out,
		Runner:      runner,
		Stderr:      &stderr,
		Now:         func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("RunReviewJob() error = %v", err)
	}

	if len(runner.commands) != 6 {
		t.Fatalf("commands len = %d, want 6: %#v", len(runner.commands), runner.commands)
	}
	last := runner.commands[len(runner.commands)-1]
	if last.Name != "codex" {
		t.Fatalf("last command = %s, want codex", last.Name)
	}
	args := strings.Join(last.Args, " ")
	for _, want := range []string{"exec review", "--base origin/main", "--model gpt-5.5", "--output-last-message", "code_reviewer"} {
		if !strings.Contains(args, want) {
			t.Fatalf("codex args missing %q: %v", want, last.Args)
		}
	}

	metadata := readMetadata(t, filepath.Join(out, "metadata.json"))
	if metadata.Status != "succeeded" {
		t.Fatalf("metadata.Status = %q", metadata.Status)
	}
	if metadata.Verdict != "approve_with_fixes" {
		t.Fatalf("metadata.Verdict = %q", metadata.Verdict)
	}
	if metadata.Profile != "standard" || metadata.Model != "gpt-5.5" {
		t.Fatalf("metadata profile/model = %#v", metadata)
	}
	if metadata.TokenUsage.Status != "unavailable" || metadata.CostStatus != "unavailable" {
		t.Fatalf("metadata usage/cost availability = %#v", metadata)
	}
	debugLogPath := filepath.Join(out, "debug.log")
	if metadata.DebugLogPath != debugLogPath {
		t.Fatalf("metadata.DebugLogPath = %q, want %q", metadata.DebugLogPath, debugLogPath)
	}
	if !strings.Contains(stderr.String(), "Debug log: "+debugLogPath) {
		t.Fatalf("stderr missing debug log path: %q", stderr.String())
	}
	debugLog := readFile(t, debugLogPath)
	for _, want := range []string{
		"run started review_id=review-1",
		"repo_url=git@github.com:org/repo.git",
		"command start",
		"argv=codex exec review",
		"run succeeded verdict=approve_with_fixes",
	} {
		if !strings.Contains(debugLog, want) {
			t.Fatalf("debug log missing %q:\n%s", want, debugLog)
		}
	}
}

func TestRunReviewJobWritesReportToStdout(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")
	out := t.TempDir()
	req := testRunnerRequest(t)
	requestJSON, err := req.JSON()
	if err != nil {
		t.Fatalf("JSON() error = %v", err)
	}
	var stdout bytes.Buffer
	err = RunReviewJob(context.Background(), RunnerOptions{
		ReviewID:    "review-stdout",
		RequestJSON: string(requestJSON),
		Workspace:   filepath.Join(t.TempDir(), "workspace"),
		OutputDir:   out,
		Runner:      &fakeCommandRunner{},
		Stdout:      &stdout,
	})
	if err != nil {
		t.Fatalf("RunReviewJob() error = %v", err)
	}
	if !strings.Contains(stdout.String(), "Approve with fixes") {
		t.Fatalf("stdout does not contain report content: %q", stdout.String())
	}
}

// TestRunReviewJobExecRunnerIsolatesChildStdout uses the real execCommandRunner
// (no custom Runner provided) with fake git and codex shell scripts so that the
// test exercises the actual exec path.  The fake codex emits JSON event lines to
// its stdout — exactly what the real codex --json flag does — and the test
// asserts that none of that noise reaches opts.Stdout.
func TestRunReviewJobExecRunnerIsolatesChildStdout(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")
	binDir := t.TempDir()
	out := t.TempDir()
	workspace := filepath.Join(t.TempDir(), "workspace")

	// Fake git: always exits 0 without doing anything.
	writeTestScript(t, filepath.Join(binDir, "git"), "#!/bin/sh\nexit 0\n")

	// Fake codex: emits JSON noise to stdout, then writes the report file.
	writeTestScript(t, filepath.Join(binDir, "codex"), `#!/bin/sh
printf '{"type":"usage","usage":{"input_tokens":10}}\n'
i=0
while [ "$i" -lt "$#" ]; do
  i=$((i+1))
  eval "arg=\$$i"
  if [ "$arg" = "--output-last-message" ]; then
    i=$((i+1))
    eval "rp=\$$i"
    printf 'Approve with fixes\n\n- P1 fixed\n' > "$rp"
  fi
done
`)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	req := testRunnerRequest(t)
	requestJSON, err := req.JSON()
	if err != nil {
		t.Fatalf("JSON() error = %v", err)
	}

	var stdout bytes.Buffer
	err = RunReviewJob(context.Background(), RunnerOptions{
		ReviewID:    "exec-isolation",
		RequestJSON: string(requestJSON),
		Workspace:   workspace,
		OutputDir:   out,
		// No Runner: uses real execCommandRunner so stdout isolation is exercised.
		Stdout: &stdout,
		Stderr: &bytes.Buffer{},
	})
	if err != nil {
		t.Fatalf("RunReviewJob() error = %v", err)
	}
	got := stdout.String()
	if !strings.Contains(got, "Approve with fixes") {
		t.Fatalf("stdout missing report content: %q", got)
	}
	if strings.Contains(got, `"type"`) || strings.Contains(got, `"usage"`) {
		t.Fatalf("stdout contains JSON noise from child process: %q", got)
	}
}

func writeTestScript(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("writeTestScript(%s): %v", path, err)
	}
}

func TestRunReviewJobWritesFailureMetadata(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")
	out := t.TempDir()
	req := testRunnerRequest(t)
	requestJSON, err := req.JSON()
	if err != nil {
		t.Fatalf("JSON() error = %v", err)
	}

	err = RunReviewJob(context.Background(), RunnerOptions{
		RequestJSON: string(requestJSON),
		Workspace:   filepath.Join(t.TempDir(), "workspace"),
		OutputDir:   out,
		Runner:      &fakeCommandRunner{failName: "git"},
	})
	if err == nil {
		t.Fatal("RunReviewJob() error = nil, want failure")
	}
	metadata := readMetadata(t, filepath.Join(out, "metadata.json"))
	if metadata.Status != "failed" {
		t.Fatalf("metadata.Status = %q, want failed", metadata.Status)
	}
	if !strings.Contains(metadata.Error, "clone repository") {
		t.Fatalf("metadata.Error = %q", metadata.Error)
	}
	if metadata.DebugLogPath != filepath.Join(out, "debug.log") {
		t.Fatalf("metadata.DebugLogPath = %q", metadata.DebugLogPath)
	}
}

func TestRunReviewJobRedactsFailureMetadataAndDebugLog(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")
	out := t.TempDir()
	req := testRunnerRequest(t)
	requestJSON, err := req.JSON()
	if err != nil {
		t.Fatalf("JSON() error = %v", err)
	}

	err = RunReviewJob(context.Background(), RunnerOptions{
		RequestJSON: string(requestJSON),
		Workspace:   filepath.Join(t.TempDir(), "workspace"),
		OutputDir:   out,
		Runner: &fakeCommandRunner{
			failName: "git",
			failErr:  &fakeError{message: "Authorization: Bearer sk-secret123456789 api_key=plain-secret"},
		},
	})
	if err == nil {
		t.Fatal("RunReviewJob() error = nil, want failure")
	}
	metadata := readMetadata(t, filepath.Join(out, "metadata.json"))
	debugLog := readFile(t, metadata.DebugLogPath)
	for _, content := range []string{metadata.Error, debugLog} {
		if strings.Contains(content, "sk-secret123456789") || strings.Contains(content, "plain-secret") {
			t.Fatalf("content contains unredacted secret:\n%s", content)
		}
		if !strings.Contains(content, "[REDACTED]") {
			t.Fatalf("content missing redaction marker:\n%s", content)
		}
	}
}

func TestRunReviewJobConfiguresGitHubTokenWhenPresent(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "test-token")
	out := t.TempDir()
	req := testRunnerRequest(t)
	requestJSON, err := req.JSON()
	if err != nil {
		t.Fatalf("JSON() error = %v", err)
	}
	runner := &fakeCommandRunner{}

	err = RunReviewJob(context.Background(), RunnerOptions{
		RequestJSON: string(requestJSON),
		Workspace:   filepath.Join(t.TempDir(), "workspace"),
		OutputDir:   out,
		Runner:      runner,
	})
	if err != nil {
		t.Fatalf("RunReviewJob() error = %v", err)
	}
	if len(runner.commands) == 0 {
		t.Fatal("no commands were recorded")
	}
	first := runner.commands[0]
	if first.Name != "git" || len(first.Args) < 3 || first.Args[0] != "config" {
		t.Fatalf("first command did not configure git credentials: %#v", first)
	}
	if !strings.Contains(strings.Join(first.Args, " "), "x-access-token") {
		t.Fatalf("git credential command missing token username: %#v", first.Args)
	}
	codexIndex := commandIndex(runner.commands, "codex")
	if codexIndex < 0 {
		t.Fatalf("codex command missing: %#v", runner.commands)
	}
	setURLIndex := commandIndexWithArgs(runner.commands, "git", "remote", "set-url", "origin", req.RepoURL)
	if setURLIndex < 0 || setURLIndex > codexIndex {
		t.Fatalf("remote URL was not sanitized before codex: %#v", runner.commands)
	}
	unsetIndex := commandIndexWithArgs(runner.commands, "git", "config", "--global", "--unset-all")
	if unsetIndex < 0 || unsetIndex < codexIndex {
		t.Fatalf("credential rewrite was not cleaned up after codex: %#v", runner.commands)
	}
}

func TestRunReviewJobDoesNotFetchRawBaseSHA(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")
	out := t.TempDir()
	req := testRunnerRequest(t)
	req.BaseRef = "0123456789abcdef0123456789abcdef01234567"
	requestJSON, err := req.JSON()
	if err != nil {
		t.Fatalf("JSON() error = %v", err)
	}
	runner := &fakeCommandRunner{}

	err = RunReviewJob(context.Background(), RunnerOptions{
		RequestJSON: string(requestJSON),
		Workspace:   filepath.Join(t.TempDir(), "workspace"),
		OutputDir:   out,
		Runner:      runner,
	})
	if err != nil {
		t.Fatalf("RunReviewJob() error = %v", err)
	}
	for _, command := range runner.commands {
		if command.Name == "git" && len(command.Args) == 3 && command.Args[0] == "fetch" && command.Args[2] == req.BaseRef {
			t.Fatalf("raw SHA base should not be fetched as a remote ref: %#v", runner.commands)
		}
	}
}

func TestParseVerdictFile(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "block", body: "# Block\n", want: "block"},
		{name: "approve with fixes", body: "Approve with fixes\n", want: "approve_with_fixes"},
		{name: "no blocking findings", body: "# No blocking findings\n", want: "no_blocking_findings"},
		{name: "unknown", body: "Looks fine\n", want: "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "review.md")
			if err := os.WriteFile(path, []byte(tt.body), 0o644); err != nil {
				t.Fatalf("WriteFile() error = %v", err)
			}
			got, err := ParseVerdictFile(path)
			if err != nil {
				t.Fatalf("ParseVerdictFile() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("ParseVerdictFile() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDecodeReviewRequestRejectsUnknownFields(t *testing.T) {
	_, err := DecodeReviewRequest([]byte(`{"repo_url":"git@example.com/repo.git","unknown":true}`))
	if err == nil {
		t.Fatal("DecodeReviewRequest() error = nil, want unknown field error")
	}
	if !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("DecodeReviewRequest() error = %v", err)
	}
}

func TestReviewPromptIncludesAdditionalInstructions(t *testing.T) {
	req := testRunnerRequest(t)
	req.Instructions = "Focus on migrations."
	prompt := reviewPrompt(req)
	if !strings.Contains(prompt, "code_reviewer") || !strings.Contains(prompt, "Focus on migrations.") {
		t.Fatalf("reviewPrompt() = %q", prompt)
	}
}

func TestLocalProxyAddressOnlyUsesLoopbackProxy(t *testing.T) {
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:8888")
	t.Setenv("HTTP_PROXY", "http://proxy.example.com:8080")
	if got := localProxyAddress(); got != "127.0.0.1:8888" {
		t.Fatalf("localProxyAddress() = %q", got)
	}
}

func TestLocalProxyAddressIgnoresRemoteProxy(t *testing.T) {
	t.Setenv("HTTPS_PROXY", "http://proxy.example.com:8080")
	t.Setenv("HTTP_PROXY", "")
	t.Setenv("ALL_PROXY", "")
	if got := localProxyAddress(); got != "" {
		t.Fatalf("localProxyAddress() = %q, want empty", got)
	}
}

func TestRemoteFetchRefTrimsOriginPrefix(t *testing.T) {
	if got := remoteFetchRef("origin/main"); got != "main" {
		t.Fatalf("remoteFetchRef(origin/main) = %q", got)
	}
	if got := remoteFetchRef("feature/test"); got != "feature/test" {
		t.Fatalf("remoteFetchRef(feature/test) = %q", got)
	}
}

func TestWaitForLocalProxyConnectsToLoopbackListener(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	defer listener.Close()
	t.Setenv("HTTPS_PROXY", "http://"+listener.Addr().String())

	errCh := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err == nil {
			_ = conn.Close()
		}
		errCh <- err
	}()

	if err := waitForLocalProxy(context.Background(), time.Second); err != nil {
		t.Fatalf("waitForLocalProxy() error = %v", err)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("Accept() error = %v", err)
	}
}

func TestExecCommandRunnerRun(t *testing.T) {
	var stdout bytes.Buffer
	runner := execCommandRunner{stdout: &stdout, stderr: &bytes.Buffer{}}
	if err := runner.Run(context.Background(), "", "sh", "-c", "printf ok"); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	// Child stdout must NOT reach r.stdout; only the final report is written there.
	if stdout.Len() != 0 {
		t.Fatalf("stdout should be empty (child stdout not forwarded), got %q", stdout.String())
	}
}

func TestExecCommandRunnerWritesRedactedDebugOutput(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	var debug bytes.Buffer
	collector := &tokenUsageCollector{}
	runner := execCommandRunner{stdout: &stdout, stderr: &stderr, debug: &debug, usage: collector}

	err := runner.Run(context.Background(), "", "sh", "-c", "printf 'Authorization: Bearer sk-secret123456789\\n'; printf '{\"usage\":{\"input_tokens\":12,\"cached_input_tokens\":3,\"output_tokens\":7}}\\n'; printf 'api_key=plain-secret\\n' >&2")
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	// Child stdout is not forwarded to r.stdout; verify stderr still passes through.
	if !strings.Contains(stderr.String(), "plain-secret") {
		t.Fatalf("stderr should preserve command output: %q", stderr.String())
	}
	debugLog := debug.String()
	if strings.Contains(debugLog, "sk-secret123456789") || strings.Contains(debugLog, "plain-secret") {
		t.Fatalf("debug log contains unredacted secret:\n%s", debugLog)
	}
	if !strings.Contains(debugLog, "stdout: Authorization: Bearer [REDACTED]") || !strings.Contains(debugLog, "stderr: api_key=[REDACTED]") {
		t.Fatalf("debug log missing redacted command output:\n%s", debugLog)
	}
	actual, ok := collector.Usage()
	if !ok {
		t.Fatal("collector did not parse usage")
	}
	if actual.Status != "available" || actual.InputTokens != 12 || actual.CachedInputTokens != 3 || actual.OutputTokens != 7 {
		t.Fatalf("actual usage = %#v", actual)
	}
}

func TestRedactDebugTextRedactsObviousCredentials(t *testing.T) {
	input := `Authorization: Bearer sk-secret123456789 api_key=plain-secret "password":"json-secret" https://user:token@example.com/repo.git`
	got := redactDebugText(input)
	for _, secret := range []string{"sk-secret123456789", "plain-secret", "json-secret", "user:token"} {
		if strings.Contains(got, secret) {
			t.Fatalf("redacted text contains %q: %s", secret, got)
		}
	}
	if strings.Count(got, "[REDACTED]") < 4 {
		t.Fatalf("redacted text missing markers: %s", got)
	}
}

func commandIndex(commands []recordedCommand, name string) int {
	for i, command := range commands {
		if command.Name == name {
			return i
		}
	}
	return -1
}

func commandIndexWithArgs(commands []recordedCommand, name string, args ...string) int {
	for i, command := range commands {
		if command.Name != name || len(command.Args) < len(args) {
			continue
		}
		matched := true
		for j, arg := range args {
			if command.Args[j] != arg {
				matched = false
				break
			}
		}
		if matched {
			return i
		}
	}
	return -1
}

func testRunnerRequest(t *testing.T) ReviewRequest {
	t.Helper()
	req, err := BuildReviewRequest(t.Context(), SubmitOptions{
		RepoURL:          "git@github.com:org/repo.git",
		BaseRef:          "origin/main",
		HeadRef:          "feature",
		HeadSHA:          "abc123",
		RequireCleanTree: false,
	})
	if err != nil {
		t.Fatalf("BuildReviewRequest() error = %v", err)
	}
	return req
}

func readMetadata(t *testing.T, path string) ReviewMetadata {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	var metadata ReviewMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		t.Fatalf("Unmarshal metadata error = %v\n%s", err, data)
	}
	return metadata
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	return string(data)
}
