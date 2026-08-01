package reviewer

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
)

type RecommendOptions struct {
	Dir    string
	Base   string
	Stdout io.Writer
}

type Recommendation struct {
	Base       string
	Files      []string
	Additions  int
	Deletions  int
	Ignored    []string
	Signals    []string
	Mode       string
	Reasons    []string
	TokenRange string
}

func Recommend(ctx context.Context, opts RecommendOptions) (Recommendation, error) {
	if opts.Dir == "" {
		opts.Dir = "."
	}
	repoRoot, err := gitOutput(ctx, opts.Dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return Recommendation{}, fmt.Errorf("find git repository root: %w", err)
	}
	repoRoot = strings.TrimSpace(repoRoot)
	cfg, _, err := LoadRepoConfig(repoRoot, false)
	if err != nil {
		return Recommendation{}, err
	}
	base := firstNonEmpty(opts.Base, cfg.Base)
	if base == "" {
		base = resolveBase(ctx, repoRoot)
	}
	filesOut, err := gitOutput(ctx, repoRoot, append([]string{"diff", "--name-only", base + "...HEAD", "--", "."}, pathspecExcludes(cfg.Ignore)...)...)
	if err != nil {
		return Recommendation{}, fmt.Errorf("summarize changed files: %w", err)
	}
	numOut, err := gitOutput(ctx, repoRoot, append([]string{"diff", "--numstat", base + "...HEAD", "--", "."}, pathspecExcludes(cfg.Ignore)...)...)
	if err != nil {
		return Recommendation{}, fmt.Errorf("summarize diff size: %w", err)
	}
	rec := Recommendation{Base: base, Ignored: cfg.Ignore}
	for _, line := range strings.Split(filesOut, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			rec.Files = append(rec.Files, line)
		}
	}
	for _, line := range strings.Split(numOut, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		if n, err := strconv.Atoi(fields[0]); err == nil {
			rec.Additions += n
		}
		if n, err := strconv.Atoi(fields[1]); err == nil {
			rec.Deletions += n
		}
	}
	rec.Signals = riskSignals(rec.Files)
	rec.Mode, rec.Reasons, rec.TokenRange = chooseMode(rec)
	return rec, nil
}

func RunRecommend(ctx context.Context, opts RecommendOptions) error {
	if opts.Stdout == nil {
		opts.Stdout = os.Stdout
	}
	rec, err := Recommend(ctx, opts)
	if err != nil {
		return err
	}
	printRecommendation(opts.Stdout, rec)
	return nil
}

func riskSignals(files []string) []string {
	seen := map[string]bool{}
	var signals []string
	add := func(s string) {
		if !seen[s] {
			seen[s] = true
			signals = append(signals, s)
		}
	}
	for _, file := range files {
		lower := strings.ToLower(file)
		switch {
		case strings.Contains(lower, "auth") || strings.Contains(lower, "token") || strings.Contains(lower, "secret"):
			add("security-sensitive files")
		case strings.Contains(lower, "migration") || strings.Contains(lower, "schema"):
			add("persistence or migration changes")
		case strings.Contains(lower, "dockerfile") || strings.Contains(lower, ".github/workflows") || strings.Contains(lower, "makefile"):
			add("build or deployment changes")
		case strings.HasSuffix(lower, "_test.go") || strings.Contains(lower, "/test"):
			add("test-only or test-heavy changes")
		case strings.HasSuffix(lower, ".md"):
			add("documentation changes")
		}
	}
	sort.Strings(signals)
	return signals
}

func chooseMode(rec Recommendation) (string, []string, string) {
	files := len(rec.Files)
	lines := rec.Additions + rec.Deletions
	if files == 0 {
		return "pre-push", []string{"no branch diff detected; a fast pre-push check is enough"}, "1k-2k"
	}
	highRisk := len(rec.Signals) > 0 && !onlyDocOrTest(rec.Signals)
	switch {
	case files > 60 || lines > 3000:
		return "full", []string{"large diff benefits from broader repository context", "changed-file or line count is above structured-review range"}, "20k-40k"
	case files > 15 || lines > 800 || highRisk:
		return "structured", []string{"diff has enough size or risk to require explicit coverage and limits"}, "10k-20k"
	case files <= 5 && lines <= 250:
		return "native", []string{"small focused diff is suitable for the fastest branch review"}, "3k-8k"
	default:
		return "structured", []string{"moderate diff benefits from subsystem coverage in the report"}, "8k-14k"
	}
}

func onlyDocOrTest(signals []string) bool {
	if len(signals) == 0 {
		return false
	}
	for _, signal := range signals {
		if signal != "documentation changes" && signal != "test-only or test-heavy changes" {
			return false
		}
	}
	return true
}

func printRecommendation(out io.Writer, rec Recommendation) {
	_, _ = fmt.Fprintf(out, "Review recommendation for %s...HEAD\n", rec.Base)
	_, _ = fmt.Fprintf(out, "Changed files: %d\n", len(rec.Files))
	_, _ = fmt.Fprintf(out, "Diff size: +%d/-%d\n", rec.Additions, rec.Deletions)
	if len(rec.Ignored) > 0 {
		_, _ = fmt.Fprintf(out, "Ignored globs: %s\n", strings.Join(rec.Ignored, ", "))
	}
	if len(rec.Signals) == 0 {
		_, _ = fmt.Fprintln(out, "Risk signals: none")
	} else {
		_, _ = fmt.Fprintf(out, "Risk signals: %s\n", strings.Join(rec.Signals, ", "))
	}
	_, _ = fmt.Fprintf(out, "Recommended mode: %s\n", rec.Mode)
	_, _ = fmt.Fprintf(out, "Estimated token range: %s\n", rec.TokenRange)
	_, _ = fmt.Fprintln(out, "Reasons:")
	for _, reason := range rec.Reasons {
		_, _ = fmt.Fprintf(out, "- %s\n", reason)
	}
}
