package service

import (
	"context"
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
