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
	"strings"
	"time"
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

type execCommandRunner struct {
	stdout io.Writer
	stderr io.Writer
}

type ReviewMetadata struct {
	ReviewID   string `json:"review_id"`
	Status     string `json:"status"`
	Verdict    string `json:"verdict,omitempty"`
	Profile    string `json:"profile"`
	Model      string `json:"model"`
	BaseRef    string `json:"base_ref"`
	HeadRef    string `json:"head_ref"`
	HeadSHA    string `json:"head_sha"`
	ReportPath string `json:"report_path"`
	Error      string `json:"error,omitempty"`
	StartedAt  string `json:"started_at"`
	FinishedAt string `json:"finished_at"`
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
	if opts.Runner == nil {
		opts.Runner = execCommandRunner{stdout: opts.Stdout, stderr: opts.Stderr}
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
	if err := os.WriteFile(filepath.Join(opts.OutputDir, "request.json"), []byte(opts.RequestJSON), 0o644); err != nil {
		return fmt.Errorf("write request.json: %w", err)
	}
	if err := waitForLocalProxy(ctx, 30*time.Second); err != nil {
		return err
	}

	reportPath := filepath.Join(opts.OutputDir, "review.md")
	metadataPath := filepath.Join(opts.OutputDir, "metadata.json")
	metadata := ReviewMetadata{
		ReviewID:   opts.ReviewID,
		Status:     "running",
		Profile:    req.ProfileName,
		Model:      req.Profile.Model,
		BaseRef:    req.BaseRef,
		HeadRef:    req.HeadRef,
		HeadSHA:    req.HeadSHA,
		ReportPath: reportPath,
		StartedAt:  startedAt.Format(time.RFC3339),
	}

	runErr := runReviewCommands(ctx, opts, req, reportPath)
	metadata.FinishedAt = opts.Now().UTC().Format(time.RFC3339)
	if runErr != nil {
		metadata.Status = "failed"
		metadata.Error = runErr.Error()
		_ = writeMetadata(metadataPath, metadata)
		return runErr
	}

	verdict, err := ParseVerdictFile(reportPath)
	if err != nil {
		metadata.Status = "failed"
		metadata.Error = err.Error()
		_ = writeMetadata(metadataPath, metadata)
		return err
	}
	metadata.Status = "succeeded"
	metadata.Verdict = verdict
	if err := writeMetadata(metadataPath, metadata); err != nil {
		return err
	}
	fmt.Fprintf(opts.Stdout, "codex-reviewer: wrote %s and %s\n", reportPath, metadataPath)
	return nil
}

func runReviewCommands(ctx context.Context, opts RunnerOptions, req ReviewRequest, reportPath string) error {
	var rewrite string
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		rewrite = "url.https://x-access-token:" + token + "@github.com/.insteadOf"
		if err := opts.Runner.Run(ctx, "", "git", "config", "--global", rewrite, "https://github.com/"); err != nil {
			return fmt.Errorf("configure GitHub credentials: %w", err)
		}
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
	if err := opts.Runner.Run(ctx, opts.Workspace, "git", "remote", "set-url", "origin", req.RepoURL); err != nil {
		return fmt.Errorf("sanitize repository remote: %w", err)
	}
	if rewrite != "" {
		if err := opts.Runner.Run(ctx, "", "git", "config", "--global", "--unset-all", rewrite); err != nil {
			return fmt.Errorf("remove GitHub credential rewrite: %w", err)
		}
	}
	args := []string{
		"exec", "review",
		"--base", req.BaseRef,
		"--model", req.Profile.Model,
		"--output-last-message", reportPath,
	}
	if err := opts.Runner.Run(ctx, opts.Workspace, "codex", args...); err != nil {
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
	if req.Instructions != "" {
		b.WriteString(" Additional instructions: ")
		b.WriteString(req.Instructions)
	}
	return b.String()
}

func ParseVerdictFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read review report: %w", err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "#"))
		if line == "" {
			continue
		}
		switch {
		case strings.HasPrefix(line, "Block"):
			return "block", nil
		case strings.HasPrefix(line, "Approve with fixes"):
			return "approve_with_fixes", nil
		case strings.HasPrefix(line, "No blocking findings"):
			return "no_blocking_findings", nil
		default:
			return "unknown", nil
		}
	}
	return "unknown", nil
}

func writeMetadata(path string, metadata ReviewMetadata) error {
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func (r execCommandRunner) Run(ctx context.Context, dir, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Stdout = r.stdout
	cmd.Stderr = r.stderr
	return cmd.Run()
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
