package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/everydaydevops/codex-code-reviewer/internal/installer"
	"github.com/everydaydevops/codex-code-reviewer/internal/reviewer"
	"github.com/everydaydevops/codex-code-reviewer/internal/service"
)

var version = "dev"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "install":
		runInstall(os.Args[2:])
	case "doctor":
		runDoctor(os.Args[2:])
	case "review":
		runReview(os.Args[2:])
	case "service":
		runService(os.Args[2:])
	case "version":
		fmt.Println(version)
	case "-h", "--help", "help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func runService(args []string) {
	if len(args) == 0 {
		serviceUsage()
		os.Exit(2)
	}
	switch args[0] {
	case "submit":
		runServiceSubmit(args[1:])
	case "-h", "--help", "help":
		serviceUsage()
	default:
		fmt.Fprintf(os.Stderr, "unknown service command: %s\n\n", args[0])
		serviceUsage()
		os.Exit(2)
	}
}

func runServiceSubmit(args []string) {
	var opts service.SubmitOptions
	var output string
	var dryRun bool
	var wait bool
	fs := flag.NewFlagSet("service submit", flag.ExitOnError)
	fs.StringVar(&opts.RepoURL, "repo-url", "", "repository URL; defaults to git remote.origin.url")
	fs.StringVar(&opts.BaseRef, "base", "", "base branch/ref; defaults to origin/main")
	fs.StringVar(&opts.HeadRef, "head", "", "head branch/ref; defaults to HEAD")
	fs.StringVar(&opts.HeadSHA, "head-sha", "", "exact head SHA; defaults to resolving --head")
	fs.StringVar(&opts.ProfileName, "profile", "", "review profile; defaults to standard")
	fs.StringVar(&opts.Instructions, "instructions", "", "additional review instructions")
	fs.StringVar(&opts.ReturnFormat, "return-format", "markdown", "review output format")
	fs.StringVar(&output, "output", "", "write dry-run request JSON to this path")
	fs.BoolVar(&dryRun, "dry-run", false, "build and print the review request without submitting it")
	fs.BoolVar(&wait, "wait", false, "wait for the remote review to finish")
	fs.BoolVar(&opts.RequireCleanTree, "require-clean-tree", true, "require a clean committed working tree")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: codex-reviewer service submit [flags]\n\n")
		fs.PrintDefaults()
	}
	fs.Parse(args)
	if fs.NArg() != 0 {
		fs.Usage()
		os.Exit(2)
	}

	opts.Dir = "."
	req, err := service.BuildReviewRequest(context.Background(), opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "build review request failed: %v\n", err)
		os.Exit(1)
	}
	data, err := req.JSON()
	if err != nil {
		fmt.Fprintf(os.Stderr, "encode review request failed: %v\n", err)
		os.Exit(1)
	}
	if output != "" {
		if err := os.WriteFile(output, data, 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "write %s failed: %v\n", output, err)
			os.Exit(1)
		}
	} else {
		fmt.Print(string(data))
	}
	if dryRun {
		return
	}
	_ = wait
	fmt.Fprintln(os.Stderr, "service submit is not connected to the review API yet; rerun with --dry-run")
	os.Exit(1)
}

func runReview(args []string) {
	if len(args) == 0 {
		reviewUsage()
		os.Exit(2)
	}
	switch args[0] {
	case "pre-push":
		runReviewPrePush(args[1:])
	case "-h", "--help", "help":
		reviewUsage()
	default:
		fmt.Fprintf(os.Stderr, "unknown review command: %s\n\n", args[0])
		reviewUsage()
		os.Exit(2)
	}
}

func runReviewPrePush(args []string) {
	var opts reviewer.PrePushOptions
	fs := flag.NewFlagSet("review pre-push", flag.ExitOnError)
	fs.StringVar(&opts.Base, "base", "", "base branch/ref to review against; defaults to config, upstream, origin/main, origin/master, then main")
	fs.StringVar(&opts.Report, "report", "", "review report path; defaults to config")
	fs.StringVar(&opts.BlockOn, "block-on", "", "when to block push: block or never; defaults to config")
	fs.BoolVar(&opts.AllowDirty, "allow-dirty", false, "allow review to run with a dirty working tree")
	fs.BoolVar(&opts.DryRun, "dry-run", false, "print the codex review command without running it")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: codex-reviewer review pre-push [flags]\n\n")
		fs.PrintDefaults()
	}
	fs.Parse(args)
	if fs.NArg() != 0 {
		fs.Usage()
		os.Exit(2)
	}

	opts.Dir = "."
	opts.Version = version
	opts.Stdout = os.Stdout
	opts.Stderr = os.Stderr
	if err := reviewer.RunPrePush(context.Background(), opts); err != nil {
		fmt.Fprintf(os.Stderr, "pre-push review failed: %v\n", err)
		os.Exit(1)
	}
}

func runInstall(args []string) {
	var opts installer.Options
	fs := flag.NewFlagSet("install", flag.ExitOnError)
	fs.BoolVar(&opts.DryRun, "dry-run", false, "print planned changes without writing files")
	fs.BoolVar(&opts.Quiet, "quiet", false, "suppress per-file install output")
	fs.StringVar(&opts.AGENTSFile, "agents-file", "AGENTS.md", "repository guidance file to create or extend")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: codex-reviewer install [flags] /path/to/project\n\n")
		fs.PrintDefaults()
	}
	fs.Parse(args)

	if fs.NArg() != 1 {
		fs.Usage()
		os.Exit(2)
	}

	opts.TargetDir = fs.Arg(0)
	opts.Version = version
	result, err := installer.Install(opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "install failed: %v\n", err)
		os.Exit(1)
	}
	if opts.Quiet {
		return
	}

	if opts.DryRun {
		fmt.Println("Dry run complete.")
	} else {
		fmt.Println("Install complete.")
	}
	fmt.Println()
	printActions(result.Actions)
	printWarnings(result.Warnings)

	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Println("  git status --short")
	fmt.Println("  codex-reviewer doctor .")
	fmt.Println("  codex review --base main")
}

func runDoctor(args []string) {
	var agentsFile string
	fs := flag.NewFlagSet("doctor", flag.ExitOnError)
	fs.StringVar(&agentsFile, "agents-file", "AGENTS.md", "repository guidance file to check")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: codex-reviewer doctor [flags] /path/to/project\n\n")
		fs.PrintDefaults()
	}
	fs.Parse(args)

	if fs.NArg() != 1 {
		fs.Usage()
		os.Exit(2)
	}

	report, err := installer.Doctor(installer.DoctorOptions{
		TargetDir:  fs.Arg(0),
		AGENTSFile: agentsFile,
		Version:    version,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "doctor failed: %v\n", err)
		os.Exit(1)
	}

	if report.OK {
		fmt.Println("Codex reviewer setup looks good.")
	} else {
		fmt.Println("Codex reviewer setup is incomplete.")
	}
	fmt.Println()
	printChecks(report.Checks)

	if !report.OK {
		fmt.Println()
		fmt.Println("Run:")
		fmt.Println("  codex-reviewer install " + fs.Arg(0))
		os.Exit(1)
	}
}

func printActions(actions []installer.Action) {
	for _, action := range actions {
		fmt.Printf("%-8s %s\n", action.Status, action.Path)
		if action.Detail != "" {
			fmt.Printf("         %s\n", action.Detail)
		}
	}
}

func printWarnings(warnings []string) {
	if len(warnings) == 0 {
		return
	}
	fmt.Println()
	fmt.Println("Warnings:")
	for _, warning := range warnings {
		fmt.Printf("- %s\n", warning)
	}
}

func printChecks(checks []installer.Check) {
	for _, check := range checks {
		fmt.Printf("%-7s %s\n", check.Status, check.Path)
		if check.Detail != "" {
			fmt.Printf("        %s\n", check.Detail)
		}
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `codex-reviewer %s

Usage:
  codex-reviewer install [flags] /path/to/project
  codex-reviewer doctor [flags] /path/to/project
  codex-reviewer review pre-push [flags]
  codex-reviewer service submit [flags]
  codex-reviewer version

`, version)
}

func reviewUsage() {
	fmt.Fprintf(os.Stderr, `codex-reviewer review

Usage:
  codex-reviewer review pre-push [flags]

`)
}

func serviceUsage() {
	fmt.Fprintf(os.Stderr, `codex-reviewer service

Usage:
  codex-reviewer service submit [flags]

`)
}
