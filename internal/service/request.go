package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os/exec"
	"strings"
)

type ReviewRequest struct {
	RepoURL      string  `json:"repo_url"`
	BaseRef      string  `json:"base_ref"`
	HeadRef      string  `json:"head_ref"`
	HeadSHA      string  `json:"head_sha"`
	ProfileName  string  `json:"profile"`
	Profile      Profile `json:"profile_config"`
	Instructions string  `json:"instructions,omitempty"`
	ReturnFormat string  `json:"return_format"`
}

type SubmitOptions struct {
	Dir              string
	RepoURL          string
	BaseRef          string
	HeadRef          string
	HeadSHA          string
	ProfileName      string
	Instructions     string
	ReturnFormat     string
	RequireCleanTree bool
}

func BuildReviewRequest(ctx context.Context, opts SubmitOptions) (ReviewRequest, error) {
	if opts.Dir == "" {
		opts.Dir = "."
	}
	if opts.BaseRef == "" {
		opts.BaseRef = "origin/main"
	}
	if opts.HeadRef == "" {
		opts.HeadRef = "HEAD"
	}
	if opts.ReturnFormat == "" {
		opts.ReturnFormat = "markdown"
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
		ReturnFormat: opts.ReturnFormat,
	}, nil
}

func (r ReviewRequest) JSON() ([]byte, error) {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func scrubURLCredentials(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.User == nil {
		return raw
	}
	u.User = nil
	return u.String()
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
