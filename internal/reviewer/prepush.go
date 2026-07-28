package reviewer

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/everydaydevops/codex-code-reviewer/internal/versionutil"
)

const ConfigFile = ".codex-reviewer.toml"

type PrePushOptions struct {
	Dir        string
	Version    string
	Base       string
	Report     string
	BlockOn    string
	AllowDirty bool
	DryRun     bool
	Stdout     io.Writer
	Stderr     io.Writer
}

type prePushConfig struct {
	Version          string
	Base             string
	BlockOn          string
	Report           string
	RequireCleanTree bool
}

func RunPrePush(ctx context.Context, opts PrePushOptions) error {
	if opts.Version == "" {
		opts.Version = "dev"
	}
	if opts.Stdout == nil {
		opts.Stdout = os.Stdout
	}
	if opts.Stderr == nil {
		opts.Stderr = os.Stderr
	}

	repoRoot, err := gitOutput(ctx, opts.Dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return fmt.Errorf("find git repository root: %w", err)
	}
	repoRoot = strings.TrimSpace(repoRoot)

	cfg, err := loadConfig(filepath.Join(repoRoot, ConfigFile))
	if err != nil {
		return err
	}
	installedVersion := versionutil.ReleaseTag(cfg.Version)
	runningVersion := versionutil.ReleaseTag(opts.Version)
	if installedVersion != runningVersion {
		return fmt.Errorf("%s version mismatch: installed %q, running %q; run codex-reviewer install . with the expected binary", ConfigFile, installedVersion, runningVersion)
	}

	base := firstNonEmpty(opts.Base, cfg.Base)
	if base == "" {
		base = resolveBase(ctx, repoRoot)
	}
	report := firstNonEmpty(opts.Report, cfg.Report)
	if report == "" {
		report = ".git/codex-review/pre-push-review.md"
	}
	blockOn := firstNonEmpty(opts.BlockOn, cfg.BlockOn)
	if blockOn == "" {
		blockOn = "block"
	}
	if blockOn != "block" && blockOn != "never" {
		return fmt.Errorf("invalid block_on %q: expected block or never", blockOn)
	}

	if cfg.RequireCleanTree && !opts.AllowDirty {
		dirty, err := gitOutput(ctx, repoRoot, "status", "--porcelain")
		if err != nil {
			return fmt.Errorf("check working tree status: %w", err)
		}
		if strings.TrimSpace(dirty) != "" {
			return errors.New("working tree is dirty; commit or stash changes before running pre-push review")
		}
	}

	reportPath := report
	if !filepath.IsAbs(reportPath) {
		reportPath = filepath.Join(repoRoot, filepath.FromSlash(reportPath))
	}

	args := []string{
		"exec", "review",
		"--base", base,
		"--output-last-message", reportPath,
	}
	_, _ = fmt.Fprintf(opts.Stdout, "codex-reviewer: running AI code review against %s\n", base)
	_, _ = fmt.Fprintf(opts.Stdout, "codex-reviewer: report path %s\n", reportPath)

	if opts.DryRun {
		fmt.Fprintf(opts.Stdout, "codex-reviewer: dry run: codex %s\n", strings.Join(args, " "))
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(reportPath), 0o755); err != nil {
		return fmt.Errorf("create review report directory: %w", err)
	}

	cmd := exec.CommandContext(ctx, "codex", args...)
	configureReviewCommand(cmd)
	cmd.Dir = repoRoot
	cmd.Stdout = opts.Stdout
	cmd.Stderr = opts.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run codex review: %w", err)
	}

	if blockOn == "block" {
		blocked, err := reportHasBlockVerdict(reportPath)
		if err != nil {
			return err
		}
		if blocked {
			return fmt.Errorf("codex review verdict is Block; see %s", reportPath)
		}
	}

	fmt.Fprintf(opts.Stdout, "codex-reviewer: review complete\n")
	return nil
}

func loadConfig(path string) (prePushConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return prePushConfig{}, fmt.Errorf("%s is missing; run codex-reviewer install ./", ConfigFile)
		}
		return prePushConfig{}, fmt.Errorf("read %s: %w", ConfigFile, err)
	}

	cfg := prePushConfig{
		BlockOn:          "block",
		Report:           ".git/codex-review/pre-push-review.md",
		RequireCleanTree: true,
	}
	section := ""
	for _, line := range strings.Split(string(data), "\n") {
		line = stripComment(strings.TrimSpace(line))
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "["), "]"))
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if section == "" && key == "version" {
			cfg.Version = unquote(value)
		}
		if section == "review.pre_push" {
			switch key {
			case "base":
				cfg.Base = unquote(value)
			case "block_on":
				cfg.BlockOn = unquote(value)
			case "report":
				cfg.Report = unquote(value)
			case "require_clean_tree":
				cfg.RequireCleanTree = value != "false"
			}
		}
	}
	if cfg.Version == "" {
		return prePushConfig{}, fmt.Errorf("%s is missing required top-level version", ConfigFile)
	}
	return cfg, nil
}

func resolveBase(ctx context.Context, repoRoot string) string {
	if upstream, err := gitOutput(ctx, repoRoot, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}"); err == nil {
		if upstream = strings.TrimSpace(upstream); upstream != "" {
			return upstream
		}
	}
	if _, err := gitOutput(ctx, repoRoot, "rev-parse", "--verify", "origin/main"); err == nil {
		return "origin/main"
	}
	if _, err := gitOutput(ctx, repoRoot, "rev-parse", "--verify", "origin/master"); err == nil {
		return "origin/master"
	}
	return "main"
}

func gitOutput(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail != "" {
			return "", fmt.Errorf("%w: %s", err, detail)
		}
		return "", err
	}
	return stdout.String(), nil
}

func reportHasBlockVerdict(path string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, fmt.Errorf("read review report: %w", err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		line = strings.TrimPrefix(line, "#")
		line = strings.TrimSpace(line)
		return strings.HasPrefix(line, "Block"), nil
	}
	return false, nil
}

func stripComment(line string) string {
	inQuote := false
	for idx, r := range line {
		switch r {
		case '"':
			inQuote = !inQuote
		case '#':
			if !inQuote {
				return strings.TrimSpace(line[:idx])
			}
		}
	}
	return line
}

func unquote(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
		return strings.ReplaceAll(value[1:len(value)-1], `\"`, `"`)
	}
	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
