package service

import (
	"context"
	"os/exec"
	"strings"
	"testing"
)

func TestBuildReviewRequestUsesExplicitValues(t *testing.T) {
	req, err := BuildReviewRequest(context.Background(), SubmitOptions{
		Dir:              t.TempDir(),
		RepoURL:          "git@github.com:org/repo.git",
		BaseRef:          "main",
		HeadRef:          "feature",
		HeadSHA:          "abc123",
		ProfileName:      "security",
		Instructions:     "Focus on auth.",
		ReturnFormat:     "markdown",
		RequireCleanTree: false,
	})
	if err != nil {
		t.Fatalf("BuildReviewRequest() error = %v", err)
	}

	if req.RepoURL != "git@github.com:org/repo.git" {
		t.Fatalf("RepoURL = %q", req.RepoURL)
	}
	if req.BaseRef != "main" || req.HeadRef != "feature" || req.HeadSHA != "abc123" {
		t.Fatalf("unexpected refs: %#v", req)
	}
	if req.ProfileName != "security" || req.Profile.Agent != "security_reviewer" {
		t.Fatalf("unexpected profile: %#v", req.Profile)
	}
	if req.Instructions != "Focus on auth." {
		t.Fatalf("Instructions = %q", req.Instructions)
	}
}

func TestBuildReviewRequestDefaults(t *testing.T) {
	req, err := BuildReviewRequest(context.Background(), SubmitOptions{
		Dir:              t.TempDir(),
		RepoURL:          "git@github.com:org/repo.git",
		HeadSHA:          "abc123",
		RequireCleanTree: false,
	})
	if err != nil {
		t.Fatalf("BuildReviewRequest() error = %v", err)
	}

	if req.BaseRef != "origin/main" {
		t.Fatalf("BaseRef = %q, want origin/main", req.BaseRef)
	}
	if req.HeadRef != "HEAD" {
		t.Fatalf("HeadRef = %q, want HEAD", req.HeadRef)
	}
	if req.ProfileName != "standard" {
		t.Fatalf("ProfileName = %q, want standard", req.ProfileName)
	}
	if req.ReturnFormat != "markdown" {
		t.Fatalf("ReturnFormat = %q, want markdown", req.ReturnFormat)
	}
}

func TestBuildReviewRequestScrubsCredentialedURL(t *testing.T) {
	req, err := BuildReviewRequest(context.Background(), SubmitOptions{
		RepoURL:          "https://token:secret@github.com/org/repo.git",
		HeadSHA:          "abc123",
		RequireCleanTree: false,
	})
	if err != nil {
		t.Fatalf("BuildReviewRequest() error = %v", err)
	}
	if strings.Contains(req.RepoURL, "secret") || strings.Contains(req.RepoURL, "token:") {
		t.Fatalf("RepoURL still contains credentials: %q", req.RepoURL)
	}
	if !strings.Contains(req.RepoURL, "github.com/org/repo.git") {
		t.Fatalf("RepoURL lost the host/path: %q", req.RepoURL)
	}
}

func TestBuildReviewRequestPreservesSSHUsername(t *testing.T) {
	sshURL := "ssh://git@github.com/org/repo.git"
	req, err := BuildReviewRequest(context.Background(), SubmitOptions{
		RepoURL:          sshURL,
		HeadSHA:          "abc123",
		RequireCleanTree: false,
	})
	if err != nil {
		t.Fatalf("BuildReviewRequest() error = %v", err)
	}
	if req.RepoURL != sshURL {
		t.Fatalf("SSH URL was modified: got %q, want %q", req.RepoURL, sshURL)
	}
}

func TestBuildReviewRequestPreservesPlainURL(t *testing.T) {
	req, err := BuildReviewRequest(context.Background(), SubmitOptions{
		RepoURL:          "https://github.com/org/repo.git",
		HeadSHA:          "abc123",
		RequireCleanTree: false,
	})
	if err != nil {
		t.Fatalf("BuildReviewRequest() error = %v", err)
	}
	if req.RepoURL != "https://github.com/org/repo.git" {
		t.Fatalf("RepoURL changed unexpectedly: %q", req.RepoURL)
	}
}

func TestReviewRequestJSONIncludesProfileConfig(t *testing.T) {
	req, err := BuildReviewRequest(context.Background(), SubmitOptions{
		Dir:              t.TempDir(),
		RepoURL:          "git@github.com:org/repo.git",
		HeadSHA:          "abc123",
		RequireCleanTree: false,
	})
	if err != nil {
		t.Fatalf("BuildReviewRequest() error = %v", err)
	}
	data, err := req.JSON()
	if err != nil {
		t.Fatalf("JSON() error = %v", err)
	}
	got := string(data)
	for _, want := range []string{`"repo_url"`, `"profile_config"`, `"agent"`, `"model"`} {
		if !strings.Contains(got, want) {
			t.Fatalf("JSON() missing %s:\n%s", want, got)
		}
	}
}

func TestBuildReviewRequestReadsGitDefaults(t *testing.T) {
	dir := initRequestGitRepo(t)
	req, err := BuildReviewRequest(context.Background(), SubmitOptions{
		Dir:              dir,
		HeadSHA:          "abc123",
		RequireCleanTree: false,
	})
	if err != nil {
		t.Fatalf("BuildReviewRequest() error = %v", err)
	}
	if req.RepoURL != "git@github.com:org/repo.git" {
		t.Fatalf("RepoURL = %q", req.RepoURL)
	}
	if req.HeadRef != "HEAD" {
		t.Fatalf("HeadRef = %q", req.HeadRef)
	}
}

func initRequestGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-b", "main"},
		{"remote", "add", "origin", "git@github.com:org/repo.git"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}
	return dir
}
