package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/markcallen/codex-reviewer/internal/reviewer"
	"github.com/markcallen/codex-reviewer/internal/usage"
)

type RunnerOptions struct {
	ReviewID    string
	RequestJSON string
	Workspace   string
	OutputDir   string
	Stdout      io.Writer
	Stderr      io.Writer
	Runner      CommandRunner
	Now         func() time.Time
}

type CommandRunner interface {
	Run(ctx context.Context, dir, name string, args ...string) error
}

type InputCommandRunner interface {
	RunWithInput(ctx context.Context, dir, input, name string, args ...string) error
}

type execCommandRunner struct {
	stdout io.Writer
	stderr io.Writer
	debug  io.Writer
	usage  *tokenUsageCollector
}

type ReviewMetadata struct {
	ReviewID     string                 `json:"review_id"`
	Status       string                 `json:"status"`
	Verdict      string                 `json:"verdict,omitempty"`
	Profile      string                 `json:"profile"`
	Model        string                 `json:"model"`
	TokenUsage   usage.ActualTokenUsage `json:"token_usage"`
	BaseRef      string                 `json:"base_ref"`
	HeadRef      string                 `json:"head_ref"`
	HeadSHA      string                 `json:"head_sha"`
	ReportPath   string                 `json:"report_path"`
	DebugLogPath string                 `json:"debug_log_path"`
	CostStatus   string                 `json:"cost_status"`
	CostUSD      *float64               `json:"cost_usd,omitempty"`
	Error        string                 `json:"error,omitempty"`
	StartedAt    string                 `json:"started_at"`
	FinishedAt   string                 `json:"finished_at"`
}

func RunReviewJob(ctx context.Context, opts RunnerOptions) error {
	if opts.ReviewID == "" {
		opts.ReviewID = "review"
	}
	if opts.Workspace == "" {
		opts.Workspace = "/workspace"
	}
	if opts.OutputDir == "" {
		opts.OutputDir = "/out"
	}
	if opts.Stdout == nil {
		opts.Stdout = os.Stdout
	}
	if opts.Stderr == nil {
		opts.Stderr = os.Stderr
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}

	startedAt := opts.Now().UTC()
	var req ReviewRequest
	if err := json.Unmarshal([]byte(opts.RequestJSON), &req); err != nil {
		return fmt.Errorf("decode REVIEW_REQUEST_JSON: %w", err)
	}
	if req.RepoURL == "" {
		return fmt.Errorf("review request missing repo_url")
	}
	if req.HeadSHA == "" {
		return fmt.Errorf("review request missing head_sha")
	}
	if req.BaseRef == "" {
		req.BaseRef = "origin/main"
	}
	if req.ProfileName == "" {
		req.ProfileName = "standard"
	}
	if req.Profile.Model == "" {
		profile, err := ResolveProfile(req.ProfileName)
		if err != nil {
			return err
		}
		req.Profile = profile
	}

	if err := os.MkdirAll(opts.Workspace, 0o755); err != nil {
		return fmt.Errorf("create workspace: %w", err)
	}
	if err := os.MkdirAll(opts.OutputDir, 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	if err := installCodexAuthFromEnv(); err != nil {
		return err
	}
	debugLogPath := filepath.Join(opts.OutputDir, "debug.log")
	debugLog, err := os.OpenFile(debugLogPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("create debug log: %w", err)
	}
	defer debugLog.Close() //nolint:errcheck
	fmt.Fprintf(opts.Stderr, "Debug log: %s\n", debugLogPath)
	writeDebugLog(debugLog, "run started review_id=%s workspace=%s output_dir=%s repo_url=%s base_ref=%s head_ref=%s head_sha=%s profile=%s model=%s started_at=%s\n",
		opts.ReviewID, opts.Workspace, opts.OutputDir, req.RepoURL, req.BaseRef, req.HeadRef, req.HeadSHA, req.ProfileName, req.Profile.Model, startedAt.Format(time.RFC3339))
	usageCollector := &tokenUsageCollector{}
	if opts.Runner == nil {
		opts.Runner = execCommandRunner{stdout: opts.Stdout, stderr: opts.Stderr, debug: debugLog, usage: usageCollector}
	}
	opts.Runner = debugCommandRunner{runner: opts.Runner, debug: debugLog, now: opts.Now}

	if err := os.WriteFile(filepath.Join(opts.OutputDir, "request.json"), []byte(opts.RequestJSON), 0o600); err != nil {
		return fmt.Errorf("write request.json: %w", err)
	}
	if err := waitForLocalProxy(ctx, 5*time.Minute); err != nil {
		return err
	}

	reportPath := filepath.Join(opts.OutputDir, "review.md")
	metadataPath := filepath.Join(opts.OutputDir, "metadata.json")
	metadata := ReviewMetadata{
		ReviewID:     opts.ReviewID,
		Status:       "running",
		Profile:      req.ProfileName,
		Model:        req.Profile.Model,
		TokenUsage:   usage.ActualTokenUsage{Status: "unavailable"},
		BaseRef:      req.BaseRef,
		HeadRef:      req.HeadRef,
		HeadSHA:      req.HeadSHA,
		ReportPath:   reportPath,
		DebugLogPath: debugLogPath,
		CostStatus:   "unavailable",
		StartedAt:    startedAt.Format(time.RFC3339),
	}

	runErr := runReviewCommands(ctx, opts, req, reportPath)
	metadata.FinishedAt = opts.Now().UTC().Format(time.RFC3339)
	if runErr != nil {
		metadata.Status = "failed"
		metadata.Error = redactDebugText(runErr.Error())
		writeDebugLog(debugLog, "run failed error=%s finished_at=%s\n", metadata.Error, metadata.FinishedAt)
		_ = writeMetadata(metadataPath, metadata)
		return runErr
	}

	verdict, err := ParseVerdictFile(reportPath)
	if err != nil {
		metadata.Status = "failed"
		metadata.Error = redactDebugText(err.Error())
		writeDebugLog(debugLog, "run failed error=%s finished_at=%s\n", metadata.Error, metadata.FinishedAt)
		_ = writeMetadata(metadataPath, metadata)
		return err
	}
	metadata.Status = "succeeded"
	metadata.Verdict = verdict
	if actual, ok := usageCollector.Usage(); ok {
		metadata.TokenUsage = actual
		cost := usage.ActualCost(actual, usage.DefaultPricing(req.Profile.Model))
		metadata.CostStatus = "available"
		metadata.CostUSD = &cost
		writeDebugLog(debugLog, "token usage status=available input=%d cached_input=%d output=%d cost_usd=%.4f\n",
			actual.InputTokens, actual.CachedInputTokens, actual.OutputTokens, cost)
	} else {
		metadata.TokenUsage = usage.ActualTokenUsage{Status: "unavailable"}
		metadata.CostStatus = "unavailable"
		writeDebugLog(debugLog, "token usage status=unavailable\n")
	}
	if err := writeMetadata(metadataPath, metadata); err != nil {
		return err
	}
	reportData, err := os.ReadFile(reportPath)
	if err != nil {
		return fmt.Errorf("read review report for stdout: %w", err)
	}
	writeDebugLog(debugLog, "run succeeded verdict=%s finished_at=%s\n", verdict, metadata.FinishedAt)
	_, _ = opts.Stdout.Write(reportData)
	return nil
}

func installCodexAuthFromEnv() error {
	auth := strings.TrimSpace(os.Getenv("CODEX_AUTH"))
	if auth == "" {
		return nil
	}
	var decoded any
	if err := json.Unmarshal([]byte(auth), &decoded); err != nil {
		return fmt.Errorf("decode CODEX_AUTH JSON: %w", err)
	}
	codexHome := os.Getenv("CODEX_HOME")
	if codexHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("discover home directory for CODEX_AUTH: %w", err)
		}
		codexHome = filepath.Join(home, ".codex")
	}
	if err := os.MkdirAll(codexHome, 0o700); err != nil {
		return fmt.Errorf("create CODEX_HOME: %w", err)
	}
	authPath := filepath.Join(codexHome, "auth.json")
	if err := os.WriteFile(authPath, []byte(auth+"\n"), 0o600); err != nil {
		return fmt.Errorf("write Codex auth file: %w", err)
	}
	_ = os.Unsetenv("CODEX_AUTH")
	_ = os.Unsetenv("CODEX_API_KEY")
	_ = os.Unsetenv("OPENAI_API_KEY")
	return nil
}

func runReviewCommands(ctx context.Context, opts RunnerOptions, req ReviewRequest, reportPath string) error {
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		rewrite := "url.https://x-access-token:" + token + "@github.com/.insteadOf"
		if err := opts.Runner.Run(ctx, "", "git", "config", "--global", rewrite, "https://github.com/"); err != nil {
			return fmt.Errorf("configure GitHub credentials: %w", err)
		}
		defer opts.Runner.Run(context.Background(), "", "git", "config", "--global", "--unset-all", rewrite) //nolint:errcheck
	}
	if err := opts.Runner.Run(ctx, "", "git", "clone", "--no-checkout", req.RepoURL, opts.Workspace); err != nil {
		return fmt.Errorf("clone repository: %w", err)
	}
	if !looksLikeSHA(req.BaseRef) {
		if err := opts.Runner.Run(ctx, opts.Workspace, "git", "fetch", "origin", remoteFetchRef(req.BaseRef)); err != nil {
			return fmt.Errorf("fetch base ref: %w", err)
		}
	}
	if req.HeadRef != "" && req.HeadRef != "HEAD" && req.HeadRef != req.HeadSHA {
		if err := opts.Runner.Run(ctx, opts.Workspace, "git", "fetch", "origin", req.HeadRef); err != nil {
			return fmt.Errorf("fetch head ref: %w", err)
		}
	}
	if err := opts.Runner.Run(ctx, opts.Workspace, "git", "checkout", "--detach", req.HeadSHA); err != nil {
		return fmt.Errorf("checkout head SHA: %w", err)
	}
	req = mergeRunnerRepoConfig(req, opts.Workspace)
	if err := opts.Runner.Run(ctx, opts.Workspace, "git", "remote", "set-url", "origin", req.RepoURL); err != nil {
		return fmt.Errorf("sanitize repository remote: %w", err)
	}
	args := []string{
		"exec", "review",
		"--json",
		"--base", req.BaseRef,
		"--model", req.Profile.Model,
		"--output-last-message", reportPath,
	}
	inputRunner, ok := opts.Runner.(InputCommandRunner)
	if !ok {
		return fmt.Errorf("runner does not support stdin for codex review")
	}
	if err := inputRunner.RunWithInput(ctx, opts.Workspace, reviewPrompt(req), "codex", args...); err != nil {
		return fmt.Errorf("run codex review: %w", err)
	}
	return nil
}

func looksLikeSHA(value string) bool {
	if len(value) < 7 || len(value) > 40 {
		return false
	}
	for _, r := range value {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F') {
			continue
		}
		return false
	}
	return true
}

func remoteFetchRef(ref string) string {
	return strings.TrimPrefix(ref, "origin/")
}

func waitForLocalProxy(ctx context.Context, timeout time.Duration) error {
	address := localProxyAddress()
	if address == "" {
		return nil
	}
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		dialer := net.Dialer{Timeout: time.Second}
		conn, err := dialer.DialContext(ctx, "tcp", address)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		lastErr = err
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	return fmt.Errorf("wait for local proxy %s: %w", address, lastErr)
}

func localProxyAddress() string {
	for _, env := range []string{"HTTPS_PROXY", "HTTP_PROXY", "ALL_PROXY"} {
		raw := os.Getenv(env)
		if raw == "" {
			continue
		}
		parsed, err := url.Parse(raw)
		if err != nil || parsed.Host == "" {
			continue
		}
		host, port, err := net.SplitHostPort(parsed.Host)
		if err != nil {
			continue
		}
		if host == "127.0.0.1" || host == "localhost" || host == "::1" {
			return net.JoinHostPort(host, port)
		}
	}
	return ""
}

func reviewPrompt(req ReviewRequest) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Spawn the %s subagent and review this change.", req.Profile.Agent)
	fmt.Fprintf(&b, " Use the %s review profile.", req.ProfileName)
	b.WriteString(" Return a prioritized Markdown report that starts with Block, Approve with fixes, or No blocking findings.")
	b.WriteString(" Focus on correctness, security/privacy, regressions, missing tests, and maintainability. Do not edit files.")
	if len(req.Directives) > 0 {
		b.WriteString(" Repository review directives: ")
		b.WriteString(strings.Join(req.Directives, " "))
	}
	if len(req.Ignore) > 0 {
		b.WriteString(" Repository ignore globs from .codex-reviewer.toml: ")
		b.WriteString(strings.Join(req.Ignore, ", "))
		b.WriteString(". Treat these as advisory exclusions for generated, vendored, dependency, or build-output paths unless surrounding context is needed for a finding.")
	}
	if req.PolicyFile != "" {
		b.WriteString(" Repository policy file: ")
		b.WriteString(req.PolicyFile)
		b.WriteString(". Read it before writing findings when it exists.")
	}
	if req.Instructions != "" {
		b.WriteString(" Additional instructions: ")
		b.WriteString(req.Instructions)
	}
	return b.String()
}

func mergeRunnerRepoConfig(req ReviewRequest, workspace string) ReviewRequest {
	cfg, found, err := reviewer.LoadRepoConfig(workspace, false)
	if err != nil || !found {
		return req
	}
	if len(req.Directives) == 0 {
		req.Directives = cleanStringList(cfg.Directives)
	}
	if len(req.Ignore) == 0 {
		req.Ignore = cleanStringList(cfg.Ignore)
	}
	if req.PolicyFile == "" {
		req.PolicyFile = strings.TrimSpace(cfg.PolicyFile)
	}
	return req
}

func ParseVerdictFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read review report: %w", err)
	}
	return ParseVerdict(data), nil
}

func ParseVerdict(data []byte) string {
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "#"))
		if line == "" {
			continue
		}
		switch {
		case strings.HasPrefix(line, "Block"):
			return "block"
		case strings.HasPrefix(line, "Approve with fixes"):
			return "approve_with_fixes"
		case strings.HasPrefix(line, "No blocking findings"):
			return "no_blocking_findings"
		default:
			return "unknown"
		}
	}
	return "unknown"
}

func writeMetadata(path string, metadata ReviewMetadata) error {
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func (r execCommandRunner) Run(ctx context.Context, dir, name string, args ...string) error {
	return r.RunWithInput(ctx, dir, "", name, args...)
}

func (r execCommandRunner) RunWithInput(ctx context.Context, dir, input, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	if input != "" {
		cmd.Stdin = strings.NewReader(input)
	}
	// Child stdout must not be forwarded to r.stdout (the service report channel).
	// Codex emits JSON event/usage lines to stdout when --json is used; forwarding
	// them would pollute the Markdown report returned to callers. The final report
	// is written to r.stdout explicitly after RunReviewJob reads the output file.
	cmd.Stdout = commandOutputWriterWithUsage(nil, r.debug, "stdout", r.usage)
	cmd.Stderr = commandOutputWriterWithUsage(r.stderr, r.debug, "stderr", r.usage)
	return cmd.Run()
}

type debugCommandRunner struct {
	runner CommandRunner
	debug  io.Writer
	now    func() time.Time
}

func (r debugCommandRunner) Run(ctx context.Context, dir, name string, args ...string) error {
	return r.RunWithInput(ctx, dir, "", name, args...)
}

func (r debugCommandRunner) RunWithInput(ctx context.Context, dir, input, name string, args ...string) error {
	startedAt := r.now().UTC()
	writeDebugLog(r.debug, "command start at=%s dir=%s argv=%s\n", startedAt.Format(time.RFC3339), dir, commandString(name, args))
	var err error
	if input != "" {
		inputRunner, ok := r.runner.(InputCommandRunner)
		if !ok {
			err = fmt.Errorf("runner does not support stdin")
		} else {
			err = inputRunner.RunWithInput(ctx, dir, input, name, args...)
		}
	} else {
		err = r.runner.Run(ctx, dir, name, args...)
	}
	finishedAt := r.now().UTC()
	if err != nil {
		writeDebugLog(r.debug, "command failed at=%s duration=%s error=%s\n", finishedAt.Format(time.RFC3339), finishedAt.Sub(startedAt), err.Error())
		return err
	}
	writeDebugLog(r.debug, "command finished at=%s duration=%s\n", finishedAt.Format(time.RFC3339), finishedAt.Sub(startedAt))
	return nil
}

func commandOutputWriterWithUsage(dst, debug io.Writer, stream string, collector *tokenUsageCollector) io.Writer {
	writers := make([]io.Writer, 0, 3)
	if dst != nil {
		writers = append(writers, dst)
	}
	if collector != nil {
		writers = append(writers, collector)
	}
	if debug == nil {
		if len(writers) == 0 {
			return io.Discard
		}
		return io.MultiWriter(writers...)
	}
	writers = append(writers, redactingDebugWriter{dst: debug, prefix: stream + ": "})
	return io.MultiWriter(writers...)
}

type redactingDebugWriter struct {
	dst    io.Writer
	prefix string
}

func (w redactingDebugWriter) Write(p []byte) (int, error) {
	if len(p) > 0 {
		_, err := fmt.Fprint(w.dst, w.prefix, redactDebugText(string(p)))
		if err != nil {
			return 0, err
		}
	}
	return len(p), nil
}

func commandString(name string, args []string) string {
	parts := append([]string{name}, args...)
	return redactDebugText(strings.Join(parts, " "))
}

func writeDebugLog(w io.Writer, format string, args ...any) {
	if w == nil {
		return
	}
	_, _ = fmt.Fprintf(w, redactDebugText(format), redactDebugArgs(args)...)
}

func redactDebugArgs(args []any) []any {
	redacted := make([]any, len(args))
	for i, arg := range args {
		switch v := arg.(type) {
		case string:
			redacted[i] = redactDebugText(v)
		default:
			redacted[i] = arg
		}
	}
	return redacted
}

var debugRedactionRules = []struct {
	pattern     *regexp.Regexp
	replacement string
}{
	{regexp.MustCompile(`(?i)(authorization\s*[:=]\s*(?:bearer|basic|token)?\s*)[A-Za-z0-9._~+/=-]+`), `${1}[REDACTED]`},
	{regexp.MustCompile(`(?i)(x-api-key\s*[:=]\s*)[A-Za-z0-9._~+/=-]+`), `${1}[REDACTED]`},
	{regexp.MustCompile(`(?i)((?:api[_-]?key|access[_-]?token|auth[_-]?token|secret|password|passwd|pwd)\s*[:=]\s*)[^\s"'` + "`" + `,;]+`), `${1}[REDACTED]`},
	{regexp.MustCompile(`(?i)("(?:api[_-]?key|access[_-]?token|auth[_-]?token|secret|password|authorization)"\s*:\s*")[^"]+(")`), `${1}[REDACTED]${2}`},
	{regexp.MustCompile(`(?i)(x-access-token:)[^@/\s]+(@github\.com)`), `${1}[REDACTED]${2}`},
	{regexp.MustCompile(`(?i)(https?://)[^/\s:@]+:[^@\s/]+@`), `${1}[REDACTED]@`},
	{regexp.MustCompile(`(?i)\b(?:sk-[A-Za-z0-9_-]{12,}|gh[opsu]_[A-Za-z0-9_]{12,}|github_pat_[A-Za-z0-9_]{12,})\b`), `[REDACTED]`},
}

func redactDebugText(value string) string {
	redacted := value
	for _, rule := range debugRedactionRules {
		redacted = rule.pattern.ReplaceAllString(redacted, rule.replacement)
	}
	return redacted
}

func DecodeReviewRequest(data []byte) (ReviewRequest, error) {
	var req ReviewRequest
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		return ReviewRequest{}, err
	}
	return req, nil
}
