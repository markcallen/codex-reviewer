//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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
	requireTool(t, "helm")
	requireTool(t, "gh")

	reviewerImage := requireEnv(t, "CODEX_REVIEWER_REVIEWER_IMAGE")
	sidecarImage := requireEnv(t, "CODEX_REVIEWER_SIDECAR_IMAGE")
	openAISecret := envDefault("CODEX_REVIEWER_OPENAI_SECRET", "")
	codexAuthSecret := envDefault("CODEX_REVIEWER_CODEX_AUTH_SECRET", "")
	githubSecret := requireEnv(t, "CODEX_REVIEWER_GITHUB_SECRET")
	namespace := envDefault("CODEX_REVIEWER_NAMESPACE", "codex-reviewer-e2e")
	cluster := envDefault("CODEX_REVIEWER_KIND_CLUSTER", "codex-reviewer-e2e")
	kubeContext := envDefault("CODEX_REVIEWER_KUBE_CONTEXT", "kind-"+cluster)

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
	defer cancel()

	ensureKindCluster(t, ctx, cluster)
	ensureNamespace(t, ctx, kubeContext, namespace)
	if os.Getenv("CODEX_AUTH") != "" {
		if codexAuthSecret == "" {
			codexAuthSecret = "codex-auth"
		}
		ensureSecretFromEnv(t, ctx, kubeContext, namespace, codexAuthSecret, "auth.json", "CODEX_AUTH")
		openAISecret = ""
	} else {
		if openAISecret == "" {
			openAISecret = requireEnv(t, "CODEX_REVIEWER_OPENAI_SECRET")
		}
		ensureSecretFromEnv(t, ctx, kubeContext, namespace, openAISecret, "api-key", "OPENAI_API_KEY")
		codexAuthSecret = ""
	}
	ensureSecretFromEnv(t, ctx, kubeContext, namespace, githubSecret, "token", "GITHUB_TOKEN")

	fixtures := loadFixtures(t)
	cases := selectRepos(t, fixtures)
	if len(cases) > 0 {
		t.Run("api-"+sanitizeName(cases[0].Name), func(t *testing.T) {
			branch := defaultBranch(t, ctx, cases[0].Name)
			req := service.ReviewRequest{
				RepoURL:      cases[0].URL,
				BaseRef:      branch.BaseRef,
				HeadRef:      branch.Name,
				HeadSHA:      branch.HeadSHA,
				ProfileName:  "standard",
				Profile:      mustProfile(t, "standard"),
				ReturnFormat: "markdown",
			}
			release := envDefault("CODEX_REVIEWER_HELM_RELEASE", "codex-reviewer")
			deployReviewAPI(t, ctx, kubeContext, namespace, release, reviewerImage, sidecarImage, openAISecret, codexAuthSecret, githubSecret)
			apiURL, stopForward := startPortForward(t, ctx, kubeContext, namespace, release)
			t.Cleanup(stopForward)

			resp, err := service.Client{BaseURL: apiURL}.Submit(ctx, req)
			if err != nil {
				t.Fatalf("Submit() error = %v", err)
			}
			t.Logf("submitted api review id=%s job=%s", resp.ID, resp.JobName)
			t.Cleanup(func() {
				deleteJobIfExists(t, context.Background(), kubeContext, namespace, resp.JobName)
			})
			report, err := service.Client{BaseURL: apiURL}.WaitReport(ctx, resp.ReportURL, 5*time.Second)
			if err != nil {
				t.Fatalf("WaitReport() error = %v", err)
			}
			assertReviewRan(t, string(report))
			status, err := service.Client{BaseURL: apiURL}.Status(ctx, resp.ID)
			if err != nil {
				t.Fatalf("Status() error = %v", err)
			}
			if status.Status != "succeeded" {
				t.Fatalf("status before restart = %#v, want succeeded", status)
			}

			stopForward()
			restartDeployment(t, ctx, kubeContext, namespace, release)
			apiURL, stopForward = startPortForward(t, ctx, kubeContext, namespace, release)
			t.Cleanup(stopForward)
			status, err = service.Client{BaseURL: apiURL}.Status(ctx, resp.ID)
			if err != nil {
				t.Fatalf("Status() after restart error = %v", err)
			}
			if status.Status != "succeeded" || status.JobName != resp.JobName {
				t.Fatalf("status after restart = %#v, want succeeded job %s", status, resp.JobName)
			}
			report, err = service.Client{BaseURL: apiURL}.Report(ctx, resp.ReportURL)
			if err != nil {
				t.Fatalf("Report() after restart error = %v", err)
			}
			assertReviewRan(t, string(report))
		})
	}
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
				ReviewID:            "e2e-" + sanitizeName(repo.Name),
				Namespace:           namespace,
				ReviewerImage:       reviewerImage,
				SidecarImage:        sidecarImage,
				OpenAISecretName:    openAISecret,
				CodexAuthSecretName: codexAuthSecret,
				GitHubSecretName:    githubSecret,
				ServiceAccount:      envDefault("CODEX_REVIEWER_SERVICE_ACCOUNT", ""),
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

func deployReviewAPI(t *testing.T, ctx context.Context, kubeContext, namespace, release, reviewerImage, sidecarImage, openAISecret, codexAuthSecret, githubSecret string) {
	t.Helper()
	authMode := "openai"
	if codexAuthSecret != "" {
		authMode = "codex"
	}
	chart := envDefault("CODEX_REVIEWER_HELM_CHART", "deploy/helm/codex-reviewer")
	serviceAccount := envDefault("CODEX_REVIEWER_SERVICE_ACCOUNT", "codex-reviewer")
	args := []string{
		"upgrade", "--install", release, chart,
		"--kube-context", kubeContext,
		"--namespace", namespace,
		"--create-namespace",
		"--set-string", "fullnameOverride=" + release,
		"--set-string", "image.fullOverride=" + reviewerImage,
		"--set-string", "reviewerJob.image.fullOverride=" + reviewerImage,
		"--set-string", "reviewerJob.sidecarImage.fullOverride=" + sidecarImage,
		"--set-string", "serviceAccount.name=" + serviceAccount,
		"--set-string", "auth.mode=" + authMode,
		"--set-string", "auth.openaiSecret.name=" + openAISecret,
		"--set-string", "auth.openaiSecret.key=api-key",
		"--set-string", "auth.codexAuthSecret.name=" + codexAuthSecret,
		"--set-string", "auth.codexAuthSecret.key=auth.json",
		"--set-string", "github.secret.name=" + githubSecret,
		"--set-string", "github.secret.key=token",
		"--wait",
	}
	run(t, ctx, "", "helm", args...)
	run(t, ctx, "", "kubectl", "--context", kubeContext, "-n", namespace, "rollout", "status", "deployment/"+release, "--timeout=2m")
}

func startPortForward(t *testing.T, ctx context.Context, kubeContext, namespace, serviceName string) (string, func()) {
	t.Helper()
	port := freeLocalPort(t)
	cmd := command(ctx, "", "kubectl", "--context", kubeContext, "-n", namespace, "port-forward", "svc/"+serviceName, strconv.Itoa(port)+":8080")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("kubectl port-forward failed: %v", err)
	}
	stopped := false
	stop := func() {
		if stopped {
			return
		}
		stopped = true
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
	}
	deadline := time.Now().Add(30 * time.Second)
	address := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", address, 500*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return "http://" + address, stop
		}
		time.Sleep(500 * time.Millisecond)
	}
	stop()
	t.Fatalf("kubectl port-forward did not become ready: %s", strings.TrimSpace(stderr.String()))
	return "", nil
}

func freeLocalPort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen on free local port: %v", err)
	}
	defer listener.Close() //nolint:errcheck
	_, rawPort, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatalf("SplitHostPort(%s) error = %v", listener.Addr(), err)
	}
	port, err := strconv.Atoi(rawPort)
	if err != nil {
		t.Fatalf("Atoi(%s) error = %v", rawPort, err)
	}
	return port
}

func restartDeployment(t *testing.T, ctx context.Context, kubeContext, namespace, deployment string) {
	t.Helper()
	run(t, ctx, "", "kubectl", "--context", kubeContext, "-n", namespace, "rollout", "restart", "deployment/"+deployment)
	run(t, ctx, "", "kubectl", "--context", kubeContext, "-n", namespace, "rollout", "status", "deployment/"+deployment, "--timeout=2m")
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
