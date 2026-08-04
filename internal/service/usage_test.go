package service

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestDryRunReviewRequestJSONIncludesEstimateAndRequestFields(t *testing.T) {
	req := testReviewRequest(t)
	data, err := DryRunReviewRequestJSON(context.Background(), t.TempDir(), req)
	if err != nil {
		t.Fatalf("DryRunReviewRequestJSON() error = %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("dry-run JSON invalid: %v\n%s", err, data)
	}
	if got["repo_url"] != "https://github.com/org/repo.git" || got["base_ref"] != "origin/main" {
		t.Fatalf("request fields missing: %#v", got)
	}
	estimate, ok := got["usage_estimate"].(map[string]any)
	if !ok {
		t.Fatalf("usage_estimate missing: %#v", got)
	}
	if estimate["token_estimate"] == nil || estimate["cost_estimate"] == nil {
		t.Fatalf("estimate incomplete: %#v", estimate)
	}
}

func TestNonEmptyPromptPartsDropsEmptyValues(t *testing.T) {
	got := strings.Join(nonEmptyPromptParts("agent", "", "  ", "prompt", "\n"), "|")
	if got != "agent|prompt" {
		t.Fatalf("nonEmptyPromptParts() = %q, want agent|prompt", got)
	}
}
