package workflow

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

type fakeRunner struct {
	commands []string
	failAt   string
}

func (r *fakeRunner) Run(_ context.Context, command string, _, _ io.Writer) error {
	r.commands = append(r.commands, command)
	if command == r.failAt {
		return errors.New("boom")
	}
	return nil
}

func TestPlanBuildsDefaultWorkflow(t *testing.T) {
	steps, err := Plan(Options{
		CommitMessage: "Add feature",
		UnitTest:      "make test",
		Fix:           "codex exec 'fix findings from review.md'",
		E2E:           "make e2e",
		Push:          true,
	})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	names := stepNames(steps)
	want := []string{"status", "stage", "commit", "unit-test", "review", "fix", "unit-test-after-fix", "e2e-test", "push"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("step names = %v, want %v", names, want)
	}
	if !strings.Contains(steps[4].Command, "codex-reviewer review local") {
		t.Fatalf("review command = %q", steps[4].Command)
	}
}

func TestPlanRequiresCommitMessage(t *testing.T) {
	_, err := Plan(Options{})
	if err == nil {
		t.Fatal("Plan() error = nil, want error")
	}
}

func TestRunDryRunPrintsWithoutExecuting(t *testing.T) {
	var stdout bytes.Buffer
	runner := &fakeRunner{}
	err := Run(context.Background(), Options{
		CommitMessage: "Add feature",
		UnitTest:      "make test",
		DryRun:        true,
		Stdout:        &stdout,
		Runner:        runner,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(runner.commands) != 0 {
		t.Fatalf("dry run executed commands: %v", runner.commands)
	}
	if !strings.Contains(stdout.String(), "==> review") {
		t.Fatalf("dry-run output missing review step:\n%s", stdout.String())
	}
}

func TestRunStopsOnFailure(t *testing.T) {
	runner := &fakeRunner{failAt: "make test"}
	err := Run(context.Background(), Options{
		CommitMessage: "Add feature",
		UnitTest:      "make test",
		Runner:        runner,
	})
	if err == nil {
		t.Fatal("Run() error = nil, want failure")
	}
	if !strings.Contains(err.Error(), "unit-test failed") {
		t.Fatalf("Run() error = %v", err)
	}
}

func TestShellRunnerRun(t *testing.T) {
	var stdout bytes.Buffer
	if err := (shellRunner{}).Run(context.Background(), "printf ok", &stdout, &bytes.Buffer{}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if stdout.String() != "ok" {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestShellQuoteEscapesSingleQuotes(t *testing.T) {
	got := shellQuote("don't break")
	if got != `'don'\''t break'` {
		t.Fatalf("shellQuote() = %q", got)
	}
}

func TestShellQuoteEmpty(t *testing.T) {
	if got := shellQuote(""); got != "''" {
		t.Fatalf("shellQuote(\"\") = %q", got)
	}
}

func stepNames(steps []Step) []string {
	names := make([]string, 0, len(steps))
	for _, step := range steps {
		names = append(names, step.Name)
	}
	return names
}
