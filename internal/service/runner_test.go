package service

import (
	"context"
	"encoding/json"
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
}

func (r *fakeCommandRunner) Run(_ context.Context, dir, name string, args ...string) error {
	r.commands = append(r.commands, recordedCommand{Dir: dir, Name: name, Args: append([]string(nil), args...)})
	if name == r.failName {
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

	err = RunReviewJob(context.Background(), RunnerOptions{
		ReviewID:    "review-1",
		RequestJSON: string(requestJSON),
		Workspace:   workspace,
		OutputDir:   out,
		Runner:      runner,
		Now:         func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("RunReviewJob() error = %v", err)
	}

	if len(runner.commands) != 5 {
		t.Fatalf("commands len = %d, want 5: %#v", len(runner.commands), runner.commands)
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
}

func TestParseVerdictFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "review.md")
	if err := os.WriteFile(path, []byte("# No blocking findings\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	got, err := ParseVerdictFile(path)
	if err != nil {
		t.Fatalf("ParseVerdictFile() error = %v", err)
	}
	if got != "no_blocking_findings" {
		t.Fatalf("ParseVerdictFile() = %q", got)
	}
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
