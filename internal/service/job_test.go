package service

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestJobManifestIncludesReviewerAndSidecar(t *testing.T) {
	req := testReviewRequest(t)
	data, err := JobManifest(req, JobOptions{
		ReviewID:         "Review_123",
		Namespace:        "reviews",
		ReviewerImage:    "registry.local/codex-reviewer:phase1",
		SidecarImage:     "registry.local/openai-egress:phase1",
		ServiceAccount:   "codex-reviewer",
		OpenAISecretName: "openai-api",
		GitHubSecretName: "github-token",
	})
	if err != nil {
		t.Fatalf("JobManifest() error = %v", err)
	}

	var manifest map[string]any
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("manifest is not JSON: %v\n%s", err, data)
	}
	if manifest["kind"] != "Job" {
		t.Fatalf("kind = %v, want Job", manifest["kind"])
	}
	metadata := manifest["metadata"].(map[string]any)
	if metadata["name"] != "codex-review-review-123" {
		t.Fatalf("metadata.name = %v", metadata["name"])
	}
	if metadata["namespace"] != "reviews" {
		t.Fatalf("metadata.namespace = %v", metadata["namespace"])
	}

	spec := manifest["spec"].(map[string]any)
	template := spec["template"].(map[string]any)
	podSpec := template["spec"].(map[string]any)
	if podSpec["serviceAccountName"] != "codex-reviewer" {
		t.Fatalf("serviceAccountName = %v", podSpec["serviceAccountName"])
	}

	containers := podSpec["containers"].([]any)
	if len(containers) != 1 {
		t.Fatalf("containers len = %d, want 1", len(containers))
	}
	reviewer := containers[0].(map[string]any)
	initContainers := podSpec["initContainers"].([]any)
	if len(initContainers) != 1 {
		t.Fatalf("initContainers len = %d, want 1", len(initContainers))
	}
	sidecar := initContainers[0].(map[string]any)
	if reviewer["name"] != "reviewer" || sidecar["name"] != "openai-egress" || sidecar["restartPolicy"] != "Always" {
		t.Fatalf("unexpected reviewer/sidecar: reviewer=%#v sidecar=%#v", reviewer, sidecar)
	}
	if _, ok := sidecar["readinessProbe"].(map[string]any); !ok {
		t.Fatalf("sidecar missing readiness probe: %#v", sidecar)
	}
	if !strings.Contains(string(data), "HTTPS_PROXY") {
		t.Fatalf("manifest missing proxy env:\n%s", data)
	}
	if !strings.Contains(string(data), "secretKeyRef") {
		t.Fatalf("manifest missing secret reference:\n%s", data)
	}
	if !strings.Contains(string(data), "GITHUB_TOKEN") {
		t.Fatalf("manifest missing GitHub token env:\n%s", data)
	}
	if !strings.Contains(string(data), "CODEX_API_KEY") {
		t.Fatalf("manifest missing Codex API key env:\n%s", data)
	}
	if strings.Contains(string(data), "sk-") {
		t.Fatalf("manifest appears to contain a raw API key:\n%s", data)
	}
}

func TestJobManifestUsesCodexAuthInsteadOfAPIKeyForReviewer(t *testing.T) {
	req := testReviewRequest(t)
	data, err := JobManifest(req, JobOptions{
		ReviewID:            "auth-review",
		ReviewerImage:       "registry.local/codex-reviewer:phase1",
		SidecarImage:        "registry.local/openai-egress:phase1",
		CodexAuthSecretName: "codex-auth",
	})
	if err != nil {
		t.Fatalf("JobManifest() error = %v", err)
	}
	text := string(data)
	if !strings.Contains(text, `"name": "CODEX_AUTH"`) {
		t.Fatalf("manifest missing CODEX_AUTH env:\n%s", text)
	}
	if strings.Contains(text, `"name": "CODEX_API_KEY"`) || strings.Contains(text, `"name": "OPENAI_API_KEY"`) {
		t.Fatalf("manifest should not inject API-key env when CODEX_AUTH is configured:\n%s", text)
	}
	if !strings.Contains(text, `"key": "auth.json"`) {
		t.Fatalf("manifest missing default Codex auth secret key:\n%s", text)
	}
}

func TestJobManifestValidatesRequiredInputs(t *testing.T) {
	req := testReviewRequest(t)
	_, err := JobManifest(req, JobOptions{ReviewID: "review-1"})
	if err == nil {
		t.Fatal("JobManifest() error = nil, want missing image error")
	}
}

func TestDNSLabelSanitizesReviewID(t *testing.T) {
	got := dnsLabel(" Feature/ABC_123 ")
	if got != "feature-abc-123" {
		t.Fatalf("dnsLabel() = %q", got)
	}
}

func testReviewRequest(t *testing.T) ReviewRequest {
	t.Helper()
	req, err := BuildReviewRequest(t.Context(), SubmitOptions{
		RepoURL:          "git@github.com:org/repo.git",
		HeadSHA:          "abc123",
		RequireCleanTree: false,
	})
	if err != nil {
		t.Fatalf("BuildReviewRequest() error = %v", err)
	}
	return req
}
