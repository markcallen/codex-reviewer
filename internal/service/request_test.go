package service

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
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

func TestBuildReviewRequestUsesRepoConfigDefaults(t *testing.T) {
	dir := initRequestGitRepo(t)
	writeRequestFile(t, dir, ".codex-reviewer.toml", `version = "v1.2.3"

[review]
base = "origin/release"
profile = "pr-readiness"
directives = ["Focus public APIs.", "Check release notes."]
ignore = ["dist/**", "vendor/**"]
policy_file = "docs/review-policy.md"
`)
	req, err := BuildReviewRequest(context.Background(), SubmitOptions{
		Dir:              dir,
		RepoURL:          "git@github.com:org/repo.git",
		HeadSHA:          "abc123",
		RequireCleanTree: false,
	})
	if err != nil {
		t.Fatalf("BuildReviewRequest() error = %v", err)
	}
	if req.BaseRef != "origin/release" {
		t.Fatalf("BaseRef = %q", req.BaseRef)
	}
	if req.ProfileName != "pr-readiness" || req.Profile.Name != "pr-readiness" {
		t.Fatalf("unexpected profile: %#v", req)
	}
	if strings.Join(req.Directives, "|") != "Focus public APIs.|Check release notes." {
		t.Fatalf("Directives = %#v", req.Directives)
	}
	if strings.Join(req.Ignore, "|") != "dist/**|vendor/**" {
		t.Fatalf("Ignore = %#v", req.Ignore)
	}
	if req.PolicyFile != "docs/review-policy.md" {
		t.Fatalf("PolicyFile = %q", req.PolicyFile)
	}
}

func TestBuildReviewRequestExplicitValuesOverrideRepoConfig(t *testing.T) {
	dir := initRequestGitRepo(t)
	writeRequestFile(t, dir, ".codex-reviewer.toml", `version = "v1.2.3"

[review]
base = "origin/release"
profile = "pr-readiness"
directives = ["Config directive."]
ignore = ["dist/**"]
policy_file = "docs/config-policy.md"
`)
	req, err := BuildReviewRequest(context.Background(), SubmitOptions{
		Dir:              dir,
		RepoURL:          "git@github.com:org/repo.git",
		BaseRef:          "origin/main",
		HeadSHA:          "abc123",
		ProfileName:      "security",
		Directives:       []string{"Explicit directive."},
		Ignore:           []string{"generated/**"},
		PolicyFile:       "docs/explicit-policy.md",
		RequireCleanTree: false,
	})
	if err != nil {
		t.Fatalf("BuildReviewRequest() error = %v", err)
	}
	if req.BaseRef != "origin/main" || req.ProfileName != "security" {
		t.Fatalf("explicit base/profile not honored: %#v", req)
	}
	if strings.Join(req.Directives, "|") != "Explicit directive." {
		t.Fatalf("Directives = %#v", req.Directives)
	}
	if strings.Join(req.Ignore, "|") != "generated/**" {
		t.Fatalf("Ignore = %#v", req.Ignore)
	}
	if req.PolicyFile != "docs/explicit-policy.md" {
		t.Fatalf("PolicyFile = %q", req.PolicyFile)
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
	req.Directives = []string{"Focus auth."}
	req.Ignore = []string{"dist/**"}
	req.PolicyFile = "docs/review-policy.md"
	data, err := req.JSON()
	if err != nil {
		t.Fatalf("JSON() error = %v", err)
	}
	got := string(data)
	for _, want := range []string{`"repo_url"`, `"profile_config"`, `"agent"`, `"model"`, `"directives"`, `"ignore"`, `"policy_file"`} {
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

func writeRequestFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", path, err)
	}
}
