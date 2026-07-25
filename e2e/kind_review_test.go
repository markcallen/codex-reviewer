//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/everydaydevops/codex-code-reviewer/internal/service"
)

type repoFixture struct {
	Name        string `json:"name"`
	URL         string `json:"url"`
	DiskUsageKB int    `json:"disk_usage_kb"`
}

type repoSet struct {
	Small []repoFixture `json:"small"`
	Large []repoFixture `json:"large"`
}

func TestKindReviewsSmallAndLargePrivateRepos(t *testing.T) {
	if os.Getenv("RUN_KIND_E2E") != "1" {
		t.Skip("set RUN_KIND_E2E=1 to run kind review e2e")
	}
	requireTool(t, "kind")
	requireTool(t, "kubectl")
	requireTool(t, "gh")

	reviewerImage := requireEnv(t, "CODEX_REVIEWER_REVIEWER_IMAGE")
	sidecarImage := requireEnv(t, "CODEX_REVIEWER_SIDECAR_IMAGE")
	openAISecret := requireEnv(t, "CODEX_REVIEWER_OPENAI_SECRET")
	githubSecret := requireEnv(t, "CODEX_REVIEWER_GITHUB_SECRET")
	namespace := envDefault("CODEX_REVIEWER_NAMESPACE", "codex-reviewer-e2e")
	cluster := envDefault("CODEX_REVIEWER_KIND_CLUSTER", "codex-reviewer-e2e")

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
	defer cancel()

	ensureKindCluster(t, ctx, cluster)
	ensureNamespace(t, ctx, namespace)

	fixtures := loadFixtures(t)
	cases := []repoFixture{fixtures.Small[0], fixtures.Large[0]}
	for _, repo := range cases {
		t.Run(repo.Name, func(t *testing.T) {
			branch, headSHA := defaultBranch(t, ctx, repo.Name)
			req := service.ReviewRequest{
				RepoURL:      repo.URL,
				BaseRef:      "origin/" + branch,
				HeadRef:      branch,
				HeadSHA:      headSHA,
				ProfileName:  "standard",
				Profile:      mustProfile(t, "standard"),
				ReturnFormat: "markdown",
			}
			manifest, err := service.JobManifest(req, service.JobOptions{
				ReviewID:         "e2e-" + sanitizeName(repo.Name),
				Namespace:        namespace,
				ReviewerImage:    reviewerImage,
				SidecarImage:     sidecarImage,
				OpenAISecretName: openAISecret,
				GitHubSecretName: githubSecret,
				ServiceAccount:   envDefault("CODEX_REVIEWER_SERVICE_ACCOUNT", ""),
			})
			if err != nil {
				t.Fatalf("JobManifest() error = %v", err)
			}
			applyManifest(t, ctx, manifest)
			waitForJob(t, ctx, namespace, "codex-review-e2e-"+sanitizeName(repo.Name))
		})
	}
}

func ensureKindCluster(t *testing.T, ctx context.Context, cluster string) {
	t.Helper()
	if err := command(ctx, "", "kind", "get", "clusters").Run(); err == nil {
		out := output(t, ctx, "", "kind", "get", "clusters")
		for _, name := range bytes.Split(out, []byte("\n")) {
			if string(bytes.TrimSpace(name)) == cluster {
				return
			}
		}
	}
	run(t, ctx, "", "kind", "create", "cluster", "--name", cluster)
}

func ensureNamespace(t *testing.T, ctx context.Context, namespace string) {
	t.Helper()
	if err := command(ctx, "", "kubectl", "get", "namespace", namespace).Run(); err == nil {
		return
	}
	run(t, ctx, "", "kubectl", "create", "namespace", namespace)
}

func applyManifest(t *testing.T, ctx context.Context, manifest []byte) {
	t.Helper()
	cmd := command(ctx, "", "kubectl", "apply", "-f", "-")
	cmd.Stdin = bytes.NewReader(manifest)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("kubectl apply failed: %v\n%s\nmanifest:\n%s", err, out, manifest)
	}
}

func waitForJob(t *testing.T, ctx context.Context, namespace, name string) {
	t.Helper()
	run(t, ctx, "", "kubectl", "-n", namespace, "wait", "--for=condition=complete", "job/"+name, "--timeout=40m")
}

func defaultBranch(t *testing.T, ctx context.Context, repo string) (string, string) {
	t.Helper()
	out := output(t, ctx, "", "gh", "repo", "view", repo, "--json", "defaultBranchRef", "--jq", ".defaultBranchRef.name + \"\\t\" + .defaultBranchRef.target.oid")
	parts := strings.Split(string(bytes.TrimSpace(out)), "\t")
	if len(parts) != 2 {
		t.Fatalf("unexpected default branch output for %s: %q", repo, out)
	}
	return parts[0], parts[1]
}

func loadFixtures(t *testing.T) repoSet {
	t.Helper()
	path := filepath.Join("repos.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	var fixtures repoSet
	if err := json.Unmarshal(data, &fixtures); err != nil {
		t.Fatalf("Unmarshal fixtures error = %v", err)
	}
	if len(fixtures.Small) < 5 || len(fixtures.Large) < 5 {
		t.Fatalf("fixtures must include at least 5 small and 5 large repos: %#v", fixtures)
	}
	return fixtures
}

func mustProfile(t *testing.T, name string) service.Profile {
	t.Helper()
	profile, err := service.ResolveProfile(name)
	if err != nil {
		t.Fatalf("ResolveProfile(%q) error = %v", name, err)
	}
	return profile
}

func requireTool(t *testing.T, name string) {
	t.Helper()
	if _, err := exec.LookPath(name); err != nil {
		t.Fatalf("%s is required: %v", name, err)
	}
}

func requireEnv(t *testing.T, name string) string {
	t.Helper()
	value := os.Getenv(name)
	if value == "" {
		t.Fatalf("%s is required", name)
	}
	return value
}

func envDefault(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func output(t *testing.T, ctx context.Context, dir, name string, args ...string) []byte {
	t.Helper()
	out, err := command(ctx, dir, name, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v failed: %v\n%s", name, args, err, out)
	}
	return out
}

func run(t *testing.T, ctx context.Context, dir, name string, args ...string) {
	t.Helper()
	out, err := command(ctx, dir, name, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v failed: %v\n%s", name, args, err, out)
	}
}

func command(ctx context.Context, dir, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	return cmd
}

func sanitizeName(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	return b.String()
}
