package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/everydaydevops/codex-code-reviewer/internal/installer"
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
  codex-reviewer version

`, version)
}
