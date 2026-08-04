package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/markcallen/codex-reviewer/internal/service"
)

type reviewSubmitOptions struct {
	submit service.SubmitOptions

	APIURL      string
	Output      string
	Wait        bool
	WaitTimeout time.Duration
	Track       bool

	SetupK8s       bool
	PortForward    bool
	KubeContext    string
	Namespace      string
	HelmRelease    string
	HelmChart      string
	RunnerImage    string
	SidecarImage   string
	ServiceAccount string
	LocalPort      int

	AuthMode           string
	CodexAuthFile      string
	CodexAuthSecret    string
	CodexAuthSecretKey string
	OpenAISecret       string
	OpenAISecretKey    string
	GitHubSecret       string
	GitHubSecretKey    string

	DryRun bool
}

func runReviewSubmit(args []string) {
	opts := defaultReviewSubmitOptions()
	fs := flag.NewFlagSet("review submit", flag.ExitOnError)
	fs.StringVar(&opts.APIURL, "api-url", opts.APIURL, "review API base URL; when empty, --setup-k8s deploys and port-forwards the API")
	fs.StringVar(&opts.submit.RepoURL, "repo-url", "", "repository URL; defaults to git remote.origin.url")
	fs.StringVar(&opts.submit.BaseRef, "base", "", "base branch/ref; defaults to origin/main")
	fs.StringVar(&opts.submit.HeadRef, "head", "", "head branch/ref; defaults to HEAD")
	fs.StringVar(&opts.submit.HeadSHA, "head-sha", "", "exact head SHA; defaults to resolving --head")
	fs.StringVar(&opts.submit.ProfileName, "profile", "", "review profile; defaults to standard")
	fs.StringVar(&opts.submit.Instructions, "instructions", "", "additional review instructions")
	fs.Var((*stringListFlag)(&opts.submit.Directives), "directive", "review directive; repeat to override configured directives")
	fs.Var((*stringListFlag)(&opts.submit.Ignore), "ignore", "review ignore glob; repeat to override configured ignore globs")
	fs.StringVar(&opts.submit.PolicyFile, "policy-file", "", "repository policy file to include in review context")
	fs.StringVar(&opts.Output, "output", opts.Output, "write review report to this path when --wait is set")
	fs.BoolVar(&opts.Wait, "wait", opts.Wait, "wait for the review report")
	fs.DurationVar(&opts.WaitTimeout, "timeout", opts.WaitTimeout, "maximum time to wait for the review report")
	fs.BoolVar(&opts.Track, "track", opts.Track, "write/update a non-secret review record under codex-review/k8s-reviews")
	fs.BoolVar(&opts.submit.RequireCleanTree, "require-clean-tree", true, "require a clean committed working tree")
	fs.BoolVar(&opts.SetupK8s, "setup-k8s", opts.SetupK8s, "create secrets, deploy the Helm chart, and port-forward when --api-url is empty")
	fs.BoolVar(&opts.PortForward, "port-forward", opts.PortForward, "start a temporary kubectl port-forward when --setup-k8s is used")
	fs.StringVar(&opts.KubeContext, "kube-context", opts.KubeContext, "Kubernetes context")
	fs.StringVar(&opts.Namespace, "namespace", opts.Namespace, "Kubernetes namespace")
	fs.StringVar(&opts.HelmRelease, "helm-release", opts.HelmRelease, "Helm release name")
	fs.StringVar(&opts.HelmChart, "helm-chart", opts.HelmChart, "Helm chart path")
	fs.StringVar(&opts.RunnerImage, "runner-image", opts.RunnerImage, "review runner image")
	fs.StringVar(&opts.SidecarImage, "sidecar-image", opts.SidecarImage, "OpenAI egress sidecar image")
	fs.StringVar(&opts.ServiceAccount, "service-account", opts.ServiceAccount, "Kubernetes service account")
	fs.IntVar(&opts.LocalPort, "local-port", opts.LocalPort, "local port for temporary API port-forward")
	fs.StringVar(&opts.AuthMode, "auth-mode", opts.AuthMode, "auth mode: auto, codex, or openai")
	fs.StringVar(&opts.CodexAuthFile, "codex-auth-file", opts.CodexAuthFile, "Codex auth.json path used when CODEX_AUTH is unset")
	fs.StringVar(&opts.CodexAuthSecret, "codex-auth-secret", opts.CodexAuthSecret, "Kubernetes Secret for Codex auth JSON")
	fs.StringVar(&opts.CodexAuthSecretKey, "codex-auth-secret-key", opts.CodexAuthSecretKey, "Secret key for Codex auth JSON")
	fs.StringVar(&opts.OpenAISecret, "openai-secret", opts.OpenAISecret, "Kubernetes Secret for OpenAI API key")
	fs.StringVar(&opts.OpenAISecretKey, "openai-secret-key", opts.OpenAISecretKey, "Secret key for OpenAI API key")
	fs.StringVar(&opts.GitHubSecret, "github-secret", opts.GitHubSecret, "Kubernetes Secret for GitHub token; omitted when GITHUB_TOKEN is unset")
	fs.StringVar(&opts.GitHubSecretKey, "github-secret-key", opts.GitHubSecretKey, "Secret key for GitHub token")
	fs.BoolVar(&opts.DryRun, "dry-run", false, "print the request and Kubernetes actions without running them")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: codex-reviewer review submit [flags]\n\n")
		fs.PrintDefaults()
	}
	_ = fs.Parse(args)
	if fs.NArg() != 0 {
		fs.Usage()
		os.Exit(2)
	}

	ctx, stop := interruptContext()
	defer stop()
	if err := reviewSubmit(ctx, opts); err != nil {
		fmt.Fprintf(os.Stderr, "review submit failed: %v\n", err)
		os.Exit(1)
	}
}

func defaultReviewSubmitOptions() reviewSubmitOptions {
	home, _ := os.UserHomeDir()
	return reviewSubmitOptions{
		APIURL:             defaultServiceAPIURL(),
		Output:             "codex-review/k8s-review.md",
		Wait:               true,
		WaitTimeout:        10 * time.Minute,
		Track:              true,
		SetupK8s:           true,
		PortForward:        true,
		KubeContext:        firstNonEmpty(os.Getenv("KUBE_CONTEXT"), "kind-codex-reviewer-e2e"),
		Namespace:          firstNonEmpty(os.Getenv("NAMESPACE"), "codex-reviewer-e2e"),
		HelmRelease:        firstNonEmpty(os.Getenv("HELM_RELEASE"), "codex-reviewer"),
		HelmChart:          firstNonEmpty(os.Getenv("HELM_CHART"), "deploy/helm/codex-reviewer"),
		RunnerImage:        firstNonEmpty(os.Getenv("RUNNER_IMAGE"), "codex-reviewer:phase1"),
		SidecarImage:       firstNonEmpty(os.Getenv("SIDECAR_IMAGE"), "openai-egress:phase1"),
		ServiceAccount:     firstNonEmpty(os.Getenv("SERVICE_ACCOUNT"), "codex-reviewer"),
		LocalPort:          18080,
		AuthMode:           firstNonEmpty(os.Getenv("AUTH_MODE"), "auto"),
		CodexAuthFile:      filepath.Join(home, ".codex", "auth.json"),
		CodexAuthSecret:    firstNonEmpty(os.Getenv("CODEX_AUTH_SECRET"), "codex-auth"),
		CodexAuthSecretKey: firstNonEmpty(os.Getenv("CODEX_AUTH_SECRET_KEY"), "auth.json"),
		OpenAISecret:       firstNonEmpty(os.Getenv("OPENAI_SECRET"), "openai-api"),
		OpenAISecretKey:    firstNonEmpty(os.Getenv("OPENAI_SECRET_KEY"), "api-key"),
		GitHubSecret:       firstNonEmpty(os.Getenv("GITHUB_SECRET"), "github-token"),
		GitHubSecretKey:    firstNonEmpty(os.Getenv("GITHUB_SECRET_KEY"), "token"),
	}
}

func reviewSubmit(ctx context.Context, opts reviewSubmitOptions) error {
	opts.submit.Dir = "."
	opts.submit.ReturnFormat = "markdown"
	req, err := service.BuildReviewRequest(ctx, opts.submit)
	if err != nil {
		return fmt.Errorf("build review request: %w", err)
	}
	if opts.DryRun {
		data, err := service.DryRunReviewRequestJSON(ctx, opts.submit.Dir, req)
		if err != nil {
			return err
		}
		fmt.Print(string(data))
		if opts.APIURL != "" {
			fmt.Fprintf(os.Stderr, "codex-reviewer: dry run: submit to %s\n", opts.APIURL)
			return nil
		}
	}

	apiURL := opts.APIURL
	var portForward *exec.Cmd
	if apiURL == "" {
		if !opts.SetupK8s {
			return errors.New("no review API URL configured; remove --setup-k8s=false or pass --api-url")
		}
		authMode, err := prepareK8sReviewService(ctx, opts)
		if err != nil {
			return err
		}
		if opts.DryRun {
			fmt.Fprintf(os.Stderr, "codex-reviewer: dry run: auth mode %s\n", authMode)
			return nil
		}
		if opts.PortForward {
			portForward, err = startReviewAPIPortForward(ctx, opts)
			if err != nil {
				return err
			}
			defer stopPortForward(portForward)
			apiURL = fmt.Sprintf("http://127.0.0.1:%d", opts.LocalPort)
		}
	}
	if apiURL == "" {
		return errors.New("review API URL is empty after setup")
	}

	resp, err := service.Client{BaseURL: apiURL}.Submit(ctx, req)
	if err != nil {
		return fmt.Errorf("submit review: %w", err)
	}
	if !opts.Wait {
		if opts.Track {
			recordPath, err := service.TrackReview(service.TrackReviewOptions{Dir: ".", APIURL: apiURL, Request: req, Response: resp})
			if err != nil {
				return fmt.Errorf("track review: %w", err)
			}
			fmt.Fprintf(os.Stderr, "codex-reviewer: tracked review in %s\n", recordPath)
		}
		data, err := json.MarshalIndent(resp, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}

	waitCtx, cancel := context.WithTimeout(ctx, opts.WaitTimeout)
	defer cancel()
	reportURL := resp.ReportURL
	if reportURL == "" {
		reportURL = "/reviews/" + resp.ID + "/report"
	}
	report, err := service.Client{BaseURL: apiURL}.WaitReport(waitCtx, reportURL, 5*time.Second)
	if err != nil {
		return fmt.Errorf("wait for review report: %w", err)
	}
	if opts.Output != "" {
		if err := writeOutputFile(opts.Output, report); err != nil {
			return fmt.Errorf("write %s: %w", opts.Output, err)
		}
	} else {
		fmt.Print(string(report))
	}
	if opts.Track {
		recordPath, err := service.TrackReview(service.TrackReviewOptions{
			Dir:        ".",
			APIURL:     apiURL,
			Request:    req,
			Response:   resp,
			Report:     report,
			ReportPath: opts.Output,
		})
		if err != nil {
			return fmt.Errorf("track review: %w", err)
		}
		fmt.Fprintf(os.Stderr, "codex-reviewer: tracked review in %s\n", recordPath)
	}
	return nil
}

func prepareK8sReviewService(ctx context.Context, opts reviewSubmitOptions) (string, error) {
	authMode, err := resolveReviewAuthMode(opts)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(opts.HelmChart); err != nil {
		return "", fmt.Errorf("helm chart %q is not available; run from the codex-reviewer checkout or pass --helm-chart", opts.HelmChart)
	}
	if err := ensureNamespace(ctx, opts); err != nil {
		return "", err
	}
	if authMode == "codex" {
		authJSON := strings.TrimSpace(os.Getenv("CODEX_AUTH"))
		if authJSON == "" {
			data, err := os.ReadFile(opts.CodexAuthFile)
			if err != nil {
				return "", fmt.Errorf("read Codex auth file: %w", err)
			}
			authJSON = strings.TrimSpace(string(data))
		}
		if err := applySecret(ctx, opts, opts.CodexAuthSecret, opts.CodexAuthSecretKey, []byte(authJSON)); err != nil {
			return "", err
		}
	} else {
		apiKey := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
		if apiKey == "" {
			return "", errors.New("OPENAI_API_KEY is required when auth mode is openai")
		}
		if err := applySecret(ctx, opts, opts.OpenAISecret, opts.OpenAISecretKey, []byte(apiKey)); err != nil {
			return "", err
		}
	}
	githubSecretName := ""
	if token := strings.TrimSpace(os.Getenv("GITHUB_TOKEN")); token != "" {
		githubSecretName = opts.GitHubSecret
		if err := applySecret(ctx, opts, opts.GitHubSecret, opts.GitHubSecretKey, []byte(token)); err != nil {
			return "", err
		}
	}
	if opts.DryRun {
		fmt.Fprintf(os.Stderr, "codex-reviewer: dry run: helm upgrade --install %s %s\n", opts.HelmRelease, opts.HelmChart)
		return authMode, nil
	}
	args := []string{
		"upgrade", "--install", opts.HelmRelease, opts.HelmChart,
		"--kube-context", opts.KubeContext,
		"--namespace", opts.Namespace,
		"--create-namespace",
		"--set-string", "fullnameOverride=" + opts.HelmRelease,
		"--set-string", "image.fullOverride=" + opts.RunnerImage,
		"--set-string", "reviewerJob.image.fullOverride=" + opts.RunnerImage,
		"--set-string", "reviewerJob.sidecarImage.fullOverride=" + opts.SidecarImage,
		"--set-string", "serviceAccount.name=" + opts.ServiceAccount,
		"--set-string", "auth.mode=" + authMode,
		"--set-string", "auth.openaiSecret.name=" + opts.OpenAISecret,
		"--set-string", "auth.openaiSecret.key=" + opts.OpenAISecretKey,
		"--set-string", "auth.codexAuthSecret.name=" + opts.CodexAuthSecret,
		"--set-string", "auth.codexAuthSecret.key=" + opts.CodexAuthSecretKey,
		"--set-string", "github.secret.name=" + githubSecretName,
		"--set-string", "github.secret.key=" + opts.GitHubSecretKey,
		"--wait",
	}
	if err := runCommand(ctx, "", "helm", args...); err != nil {
		return "", err
	}
	return authMode, nil
}

func ensureNamespace(ctx context.Context, opts reviewSubmitOptions) error {
	if opts.DryRun {
		fmt.Fprintf(os.Stderr, "codex-reviewer: dry run: kubectl create/update namespace %s\n", opts.Namespace)
		return nil
	}
	create := exec.CommandContext(ctx, "kubectl", "--context", opts.KubeContext, "create", "namespace", opts.Namespace, "--dry-run=client", "-o", "yaml")
	var stderr bytes.Buffer
	create.Stderr = &stderr
	yaml, err := create.Output()
	if err != nil {
		return fmt.Errorf("create namespace manifest for %s: %w: %s", opts.Namespace, err, strings.TrimSpace(stderr.String()))
	}
	apply := exec.CommandContext(ctx, "kubectl", "--context", opts.KubeContext, "apply", "-f", "-")
	apply.Stdin = bytes.NewReader(yaml)
	apply.Stderr = &stderr
	if err := apply.Run(); err != nil {
		return fmt.Errorf("apply namespace %s: %w: %s", opts.Namespace, err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

func resolveReviewAuthMode(opts reviewSubmitOptions) (string, error) {
	switch opts.AuthMode {
	case "", "auto":
		if strings.TrimSpace(os.Getenv("CODEX_AUTH")) != "" || fileExists(opts.CodexAuthFile) {
			return "codex", nil
		}
		return "openai", nil
	case "codex", "openai":
		return opts.AuthMode, nil
	default:
		return "", fmt.Errorf("unsupported auth mode %q", opts.AuthMode)
	}
}

func applySecret(ctx context.Context, opts reviewSubmitOptions, name, key string, value []byte) error {
	if opts.DryRun {
		fmt.Fprintf(os.Stderr, "codex-reviewer: dry run: kubectl create/update secret %s key %s\n", name, key)
		return nil
	}
	tmp, err := os.CreateTemp("", "codex-reviewer-secret-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(value); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	create := exec.CommandContext(ctx, "kubectl", "--context", opts.KubeContext, "-n", opts.Namespace, "create", "secret", "generic", name, "--from-file="+key+"="+tmp.Name(), "--dry-run=client", "-o", "yaml")
	var stderr bytes.Buffer
	create.Stderr = &stderr
	yaml, err := create.Output()
	if err != nil {
		return fmt.Errorf("create secret manifest for %s: %w: %s", name, err, strings.TrimSpace(stderr.String()))
	}
	apply := exec.CommandContext(ctx, "kubectl", "--context", opts.KubeContext, "apply", "-f", "-")
	apply.Stdin = bytes.NewReader(yaml)
	apply.Stderr = &stderr
	if err := apply.Run(); err != nil {
		return fmt.Errorf("apply secret %s: %w: %s", name, err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

func startReviewAPIPortForward(ctx context.Context, opts reviewSubmitOptions) (*exec.Cmd, error) {
	if err := waitForDeployment(ctx, opts); err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, "kubectl", "--context", opts.KubeContext, "-n", opts.Namespace, "port-forward", "svc/"+opts.HelmRelease, strconv.Itoa(opts.LocalPort)+":8080")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	address := fmt.Sprintf("127.0.0.1:%d", opts.LocalPort)
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", address, 500*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return cmd, nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	stopPortForward(cmd)
	return nil, fmt.Errorf("port-forward did not become ready: %s", strings.TrimSpace(stderr.String()))
}

func waitForDeployment(ctx context.Context, opts reviewSubmitOptions) error {
	return runCommand(ctx, "", "kubectl", "--context", opts.KubeContext, "-n", opts.Namespace, "rollout", "status", "deploy/"+opts.HelmRelease, "--timeout=2m")
}

func stopPortForward(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Kill()
	_ = cmd.Wait()
}

func runCommand(ctx context.Context, dir, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail != "" {
			return fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, detail)
		}
		return fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
