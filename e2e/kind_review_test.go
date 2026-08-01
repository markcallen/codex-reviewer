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

	"github.com/markcallen/codex-reviewer/internal/service"
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

type branchInfo struct {
	Name    string
	HeadSHA string
	BaseRef string
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
	kubeContext := envDefault("CODEX_REVIEWER_KUBE_CONTEXT", "kind-"+cluster)

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
	defer cancel()

	ensureKindCluster(t, ctx, cluster)
	ensureNamespace(t, ctx, kubeContext, namespace)
	ensureSecretFromEnv(t, ctx, kubeContext, namespace, openAISecret, "api-key", "OPENAI_API_KEY")
	ensureSecretFromEnv(t, ctx, kubeContext, namespace, githubSecret, "token", "GITHUB_TOKEN")

	fixtures := loadFixtures(t)
	cases := selectRepos(t, fixtures)
	for _, repo := range cases {
		t.Run(repo.Name, func(t *testing.T) {
			branch := defaultBranch(t, ctx, repo.Name)
			t.Logf("repo=%s url=%s disk_usage_kb=%d branch=%s sha=%s base=%s", repo.Name, repo.URL, repo.DiskUsageKB, branch.Name, branch.HeadSHA, branch.BaseRef)
			req := service.ReviewRequest{
				RepoURL:      repo.URL,
				BaseRef:      branch.BaseRef,
				HeadRef:      branch.Name,
				HeadSHA:      branch.HeadSHA,
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
			jobName := service.JobName("e2e-" + sanitizeName(repo.Name))
			t.Logf("job=%s namespace=%s reviewer_image=%s sidecar_image=%s", jobName, namespace, reviewerImage, sidecarImage)
			deleteJobIfExists(t, ctx, kubeContext, namespace, jobName)
			applyManifest(t, ctx, kubeContext, manifest)
			t.Cleanup(func() {
				deleteJobIfExists(t, context.Background(), kubeContext, namespace, jobName)
			})
			waitForReviewerContainer(t, ctx, kubeContext, namespace, jobName, 40*time.Minute)
			logs := output(t, ctx, "", "kubectl", "--context", kubeContext, "-n", namespace, "logs", "job/"+jobName, "-c", "reviewer")
			t.Logf("code review output for %s@%s:\n%s", repo.Name, branch.HeadSHA, string(logs))
			assertReviewRan(t, string(logs))
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

func ensureNamespace(t *testing.T, ctx context.Context, kubeContext, namespace string) {
	t.Helper()
	if err := command(ctx, "", "kubectl", "--context", kubeContext, "get", "namespace", namespace).Run(); err == nil {
		return
	}
	run(t, ctx, "", "kubectl", "--context", kubeContext, "create", "namespace", namespace)
}

func ensureSecretFromEnv(t *testing.T, ctx context.Context, kubeContext, namespace, secret, key, envName string) {
	t.Helper()
	value := requireEnv(t, envName)
	tmp := t.TempDir()
	path := filepath.Join(tmp, key)
	if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", path, err)
	}
	cmd := command(ctx, "", "kubectl", "--context", kubeContext, "-n", namespace, "create", "secret", "generic", secret, "--from-file="+key+"="+path, "--dry-run=client", "-o", "yaml")
	yaml, err := cmd.Output()
	if err != nil {
		t.Fatalf("kubectl create secret %s failed: %v", secret, err)
	}
	applyManifest(t, ctx, kubeContext, yaml)
}

func applyManifest(t *testing.T, ctx context.Context, kubeContext string, manifest []byte) {
	t.Helper()
	cmd := command(ctx, "", "kubectl", "--context", kubeContext, "apply", "-f", "-")
	cmd.Stdin = bytes.NewReader(manifest)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("kubectl apply failed: %v\n%s\nmanifest:\n%s", err, out, manifest)
	}
}

func waitForReviewerContainer(t *testing.T, ctx context.Context, kubeContext, namespace, jobName string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		out, err := command(ctx, "", "kubectl", "--context", kubeContext, "-n", namespace, "get", "pods", "-l", "job-name="+jobName, "-o", "jsonpath={.items[0].status.containerStatuses[?(@.name=='reviewer')].state.terminated.exitCode}").Output()
		if err != nil {
			time.Sleep(5 * time.Second)
			continue
		}
		exitCode := strings.TrimSpace(string(out))
		switch exitCode {
		case "", "<no value>":
			time.Sleep(5 * time.Second)
			continue
		case "0":
			return
		default:
			logs := output(t, ctx, "", "kubectl", "--context", kubeContext, "-n", namespace, "logs", "job/"+jobName, "-c", "reviewer")
			t.Fatalf("reviewer container exited with %s\n%s", exitCode, logs)
		}
	}
	describe, _ := command(ctx, "", "kubectl", "--context", kubeContext, "-n", namespace, "describe", "job/"+jobName).CombinedOutput()
	logs, _ := command(ctx, "", "kubectl", "--context", kubeContext, "-n", namespace, "logs", "job/"+jobName, "-c", "reviewer").CombinedOutput()
	t.Fatalf("reviewer container did not finish within %s\n%s\n%s", timeout, describe, logs)
}

func deleteJobIfExists(t *testing.T, ctx context.Context, kubeContext, namespace, jobName string) {
	t.Helper()
	_ = command(ctx, "", "kubectl", "--context", kubeContext, "-n", namespace, "delete", "job", jobName, "--ignore-not-found=true", "--wait=true").Run()
}

func assertReviewRan(t *testing.T, logs string) {
	t.Helper()
	if strings.Contains(logs, "Block") || strings.Contains(logs, "Approve with fixes") || strings.Contains(logs, "No blocking findings") {
		return
	}
	if strings.Contains(logs, "Review comment:") ||
		strings.Contains(logs, "no code changes to review") ||
		strings.Contains(logs, "no introduced code changes to review") ||
		strings.Contains(logs, "did not find any discrete, actionable bugs") ||
		strings.Contains(logs, "did not find any discrete issue introduced by the patch") ||
		strings.Contains(logs, "No actionable bugs were found in the diff") {
		return
	}
	if strings.Contains(logs, "codex-reviewer: wrote") {
		return
	}
	t.Fatalf("reviewer logs did not include a review verdict or report write confirmation:\n%s", logs)
}

func defaultBranch(t *testing.T, ctx context.Context, repo string) branchInfo {
	t.Helper()
	branchOut := output(t, ctx, "", "gh", "repo", "view", repo, "--json", "defaultBranchRef", "--jq", ".defaultBranchRef.name")
	branch := strings.TrimSpace(string(branchOut))
	if branch == "" {
		t.Fatalf("default branch was empty for %s", repo)
	}
	shaOut := output(t, ctx, "", "gh", "api", "repos/"+repo+"/branches/"+branch, "--jq", ".commit.sha")
	headSHA := strings.TrimSpace(string(shaOut))
	if headSHA == "" {
		t.Fatalf("default branch SHA was empty for %s branch %s", repo, branch)
	}
	parentOut := output(t, ctx, "", "gh", "api", "repos/"+repo+"/commits/"+headSHA, "--jq", ".parents[0].sha // \"\"")
	parentSHA := strings.TrimSpace(string(parentOut))
	baseRef := "origin/" + branch
	if parentSHA != "" {
		baseRef = parentSHA
	}
	return branchInfo{Name: branch, HeadSHA: headSHA, BaseRef: baseRef}
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

func selectRepos(t *testing.T, fixtures repoSet) []repoFixture {
	t.Helper()
	if names := os.Getenv("CODEX_REVIEWER_E2E_REPOS"); names != "" {
		byName := map[string]repoFixture{}
		for _, repo := range append(append([]repoFixture{}, fixtures.Small...), fixtures.Large...) {
			byName[repo.Name] = repo
		}
		var selected []repoFixture
		for _, name := range strings.Split(names, ",") {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			repo, ok := byName[name]
			if !ok {
				if !strings.Contains(name, "/") {
					t.Fatalf("CODEX_REVIEWER_E2E_REPOS includes invalid repo %q; expected owner/repo", name)
				}
				repo = repoFixture{
					Name: name,
					URL:  "https://github.com/" + name,
				}
			}
			selected = append(selected, repo)
		}
		if len(selected) == 0 {
			t.Fatal("CODEX_REVIEWER_E2E_REPOS did not include any repos")
		}
		return selected
	}
	if os.Getenv("CODEX_REVIEWER_E2E_SMALL_ONLY") == "1" {
		return []repoFixture{fixtures.Small[0]}
	}
	return []repoFixture{fixtures.Small[0], fixtures.Large[0]}
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
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + ('a' - 'A'))
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	return b.String()
}
