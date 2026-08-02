package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os/exec"
	"strings"

	"github.com/markcallen/codex-reviewer/internal/reviewer"
)

type ReviewRequest struct {
	RepoURL      string   `json:"repo_url"`
	BaseRef      string   `json:"base_ref"`
	HeadRef      string   `json:"head_ref"`
	HeadSHA      string   `json:"head_sha"`
	ProfileName  string   `json:"profile"`
	Profile      Profile  `json:"profile_config"`
	Instructions string   `json:"instructions,omitempty"`
	Directives   []string `json:"directives,omitempty"`
	Ignore       []string `json:"ignore,omitempty"`
	PolicyFile   string   `json:"policy_file,omitempty"`
	ReturnFormat string   `json:"return_format"`
}

type SubmitOptions struct {
	Dir              string
	RepoURL          string
	BaseRef          string
	HeadRef          string
	HeadSHA          string
	ProfileName      string
	Instructions     string
	Directives       []string
	Ignore           []string
	PolicyFile       string
	ReturnFormat     string
	RequireCleanTree bool
}

func BuildReviewRequest(ctx context.Context, opts SubmitOptions) (ReviewRequest, error) {
	if opts.Dir == "" {
		opts.Dir = "."
	}
	cfg, _ := loadSubmitRepoConfig(ctx, opts.Dir)
	if opts.BaseRef == "" {
		opts.BaseRef = firstNonEmpty(cfg.Base, "origin/main")
	}
	if opts.HeadRef == "" {
		opts.HeadRef = "HEAD"
	}
	if opts.ReturnFormat == "" {
		opts.ReturnFormat = "markdown"
	}
	if opts.ProfileName == "" {
		opts.ProfileName = cfg.Profile
	}
	if len(opts.Directives) == 0 {
		opts.Directives = cfg.Directives
	}
	if len(opts.Ignore) == 0 {
		opts.Ignore = cfg.Ignore
	}
	if opts.PolicyFile == "" {
		opts.PolicyFile = cfg.PolicyFile
	}

	profile, err := ResolveProfile(opts.ProfileName)
	if err != nil {
		return ReviewRequest{}, err
	}
	if opts.ProfileName == "" {
		opts.ProfileName = profile.Name
	}

	if opts.RequireCleanTree {
		status, err := gitOutput(ctx, opts.Dir, "status", "--porcelain")
		if err != nil {
			return ReviewRequest{}, fmt.Errorf("check working tree status: %w", err)
		}
		if strings.TrimSpace(status) != "" {
			return ReviewRequest{}, fmt.Errorf("working tree is dirty; commit or stash changes before submitting a service review")
		}
	}

	repoURL := strings.TrimSpace(opts.RepoURL)
	if repoURL == "" {
		var err error
		repoURL, err = gitOutput(ctx, opts.Dir, "config", "--get", "remote.origin.url")
		if err != nil {
			return ReviewRequest{}, fmt.Errorf("discover origin remote: %w", err)
		}
		repoURL = strings.TrimSpace(repoURL)
	}
	repoURL = scrubURLCredentials(repoURL)

	headSHA := strings.TrimSpace(opts.HeadSHA)
	if headSHA == "" {
		var err error
		headSHA, err = gitOutput(ctx, opts.Dir, "rev-parse", "--verify", opts.HeadRef)
		if err != nil {
			return ReviewRequest{}, fmt.Errorf("resolve head ref %q: %w", opts.HeadRef, err)
		}
		headSHA = strings.TrimSpace(headSHA)
	}

	return ReviewRequest{
		RepoURL:      repoURL,
		BaseRef:      opts.BaseRef,
		HeadRef:      opts.HeadRef,
		HeadSHA:      headSHA,
		ProfileName:  opts.ProfileName,
		Profile:      profile,
		Instructions: strings.TrimSpace(opts.Instructions),
		Directives:   cleanStringList(opts.Directives),
		Ignore:       cleanStringList(opts.Ignore),
		PolicyFile:   strings.TrimSpace(opts.PolicyFile),
		ReturnFormat: opts.ReturnFormat,
	}, nil
}

func loadSubmitRepoConfig(ctx context.Context, dir string) (reviewer.RepoConfig, bool) {
	cfg, found, err := reviewer.LoadRepoConfigForDir(ctx, dir, false)
	if err != nil {
		return reviewer.RepoConfig{}, false
	}
	return cfg, found
}

func cleanStringList(values []string) []string {
	var out []string
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			out = append(out, value)
		}
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func (r ReviewRequest) JSON() ([]byte, error) {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// scrubURLCredentials removes userinfo from http/https URLs. SSH URLs are left
// intact because the username (e.g. "git") is required for authentication.
func scrubURLCredentials(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.User == nil {
		return raw
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
		u.User = nil
		return u.String()
	}
	return raw
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
