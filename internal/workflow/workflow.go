package workflow

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

type Options struct {
	CommitMessage string
	UnitTest      string
	Review        string
	Fix           string
	E2E           string
	Push          bool
	DryRun        bool
	Stdout        io.Writer
	Stderr        io.Writer
	Runner        CommandRunner
}

type Step struct {
	Name    string
	Command string
}

type CommandRunner interface {
	Run(ctx context.Context, command string, stdout, stderr io.Writer) error
}

type shellRunner struct{}

func Plan(opts Options) ([]Step, error) {
	if strings.TrimSpace(opts.CommitMessage) == "" {
		return nil, fmt.Errorf("commit message is required")
	}
	steps := []Step{
		{Name: "status", Command: "git status --short"},
		{Name: "stage", Command: "git add ."},
		{Name: "commit", Command: shellQuoteCommand("git commit -m", opts.CommitMessage)},
	}
	if opts.UnitTest != "" {
		steps = append(steps, Step{Name: "unit-test", Command: opts.UnitTest})
	}
	review := opts.Review
	if review == "" {
		review = "codex-reviewer service submit --base origin/main --head HEAD --profile standard --wait --output review.md"
	}
	steps = append(steps, Step{Name: "review", Command: review})
	if opts.Fix != "" {
		steps = append(steps, Step{Name: "fix", Command: opts.Fix})
		if opts.UnitTest != "" {
			steps = append(steps, Step{Name: "unit-test-after-fix", Command: opts.UnitTest})
		}
	}
	if opts.E2E != "" {
		steps = append(steps, Step{Name: "e2e-test", Command: opts.E2E})
	}
	if opts.Push {
		steps = append(steps, Step{Name: "push", Command: "git push"})
	}
	return steps, nil
}

func Run(ctx context.Context, opts Options) error {
	if opts.Stdout == nil {
		opts.Stdout = os.Stdout
	}
	if opts.Stderr == nil {
		opts.Stderr = os.Stderr
	}
	if opts.Runner == nil {
		opts.Runner = shellRunner{}
	}
	steps, err := Plan(opts)
	if err != nil {
		return err
	}
	for _, step := range steps {
		fmt.Fprintf(opts.Stdout, "==> %s\n%s\n", step.Name, step.Command)
		if opts.DryRun {
			continue
		}
		if err := opts.Runner.Run(ctx, step.Command, opts.Stdout, opts.Stderr); err != nil {
			return fmt.Errorf("%s failed: %w", step.Name, err)
		}
	}
	return nil
}

func (shellRunner) Run(ctx context.Context, command string, stdout, stderr io.Writer) error {
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}

func shellQuoteCommand(prefix, value string) string {
	return prefix + " " + shellQuote(value)
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}
