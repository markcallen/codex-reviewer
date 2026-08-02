package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/markcallen/codex-reviewer/internal/codexconfig"
	"github.com/markcallen/codex-reviewer/internal/installer"
	"github.com/markcallen/codex-reviewer/internal/reviewer"
	"github.com/markcallen/codex-reviewer/internal/service"
	"github.com/markcallen/codex-reviewer/internal/versionutil"
	"github.com/markcallen/codex-reviewer/internal/workflow"
)

var version = "dev"
var listenAndServe = http.ListenAndServe

type stringListFlag []string

func (f *stringListFlag) String() string {
	return strings.Join(*f, ",")
}

func (f *stringListFlag) Set(value string) error {
	*f = append(*f, value)
	return nil
}

func interruptContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "setup":
		runSetup(os.Args[2:])
	case "install":
		runInstall(os.Args[2:])
	case "doctor":
		runDoctor(os.Args[2:])
	case "review":
		runReview(os.Args[2:])
	case "service":
		runService(os.Args[2:])
	case "workflow":
		runWorkflow(os.Args[2:])
	case "version":
		fmt.Println(version)
	case "-h", "--help", "help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func runWorkflow(args []string) {
	if len(args) == 0 {
		workflowUsage()
		os.Exit(2)
	}
	switch args[0] {
	case "run":
		runWorkflowRun(args[1:])
	case "-h", "--help", "help":
		workflowUsage()
	default:
		fmt.Fprintf(os.Stderr, "unknown workflow command: %s\n\n", args[0])
		workflowUsage()
		os.Exit(2)
	}
}

func runWorkflowRun(args []string) {
	var opts workflow.Options
	fs := flag.NewFlagSet("workflow run", flag.ExitOnError)
	fs.StringVar(&opts.CommitMessage, "commit-message", "", "commit message for git commit")
	fs.StringVar(&opts.UnitTest, "unit-test", "", "unit test command")
	fs.StringVar(&opts.Review, "review", "", "review command; defaults to local review")
	fs.StringVar(&opts.Fix, "fix", "", "fix command to run after review")
	fs.StringVar(&opts.E2E, "e2e-test", "", "e2e test command")
	fs.BoolVar(&opts.Push, "push", false, "push to the configured remote after e2e tests pass")
	fs.BoolVar(&opts.DryRun, "dry-run", false, "print commands without running them")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: codex-reviewer workflow run [flags]\n\n")
		fs.PrintDefaults()
	}
	_ = fs.Parse(args)
	if fs.NArg() != 0 {
		fs.Usage()
		os.Exit(2)
	}

	opts.Stdout = os.Stdout
	opts.Stderr = os.Stderr
	ctx, stop := interruptContext()
	defer stop()
	if err := workflow.Run(ctx, opts); err != nil {
		fmt.Fprintf(os.Stderr, "workflow failed: %v\n", err)
		os.Exit(1)
	}
}

func runService(args []string) {
	if len(args) == 0 {
		serviceUsage()
		os.Exit(2)
	}
	switch args[0] {
	case "api":
		runServiceAPI(args[1:])
	case "submit":
		runServiceSubmit(args[1:])
	case "job-manifest":
		runServiceJobManifest(args[1:])
	case "runner":
		runServiceRunner(args[1:])
	case "telemetry":
		runServiceTelemetry(args[1:])
	case "-h", "--help", "help":
		serviceUsage()
	default:
		fmt.Fprintf(os.Stderr, "unknown service command: %s\n\n", args[0])
		serviceUsage()
		os.Exit(2)
	}
}

func runServiceTelemetry(args []string) {
	var listen string
	var token string
	var maxBodyBytes int64
	fs := flag.NewFlagSet("service telemetry", flag.ExitOnError)
	fs.StringVar(&listen, "listen", ":8081", "HTTP listen address")
	fs.StringVar(&token, "token", os.Getenv("CODEX_REVIEWER_TELEMETRY_TOKEN"), "bearer token required for telemetry ingestion and queries; defaults to CODEX_REVIEWER_TELEMETRY_TOKEN")
	fs.Int64Var(&maxBodyBytes, "max-body-bytes", 256<<10, "maximum telemetry event payload size")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: codex-reviewer service telemetry [flags]\n\n")
		fs.PrintDefaults()
	}
	_ = fs.Parse(args)
	if fs.NArg() != 0 {
		fs.Usage()
		os.Exit(2)
	}
	server, err := service.NewTelemetryServer(service.TelemetryOptions{Token: token, MaxBodyBytes: maxBodyBytes})
	if err != nil {
		fmt.Fprintf(os.Stderr, "configure telemetry service failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stdout, "codex-reviewer telemetry: listening on %s\n", listen)
	if err := listenAndServe(listen, server.Handler()); err != nil {
		fmt.Fprintf(os.Stderr, "telemetry service failed: %v\n", err)
		os.Exit(1)
	}
}

func runServiceAPI(args []string) {
	var listen string
	var jobOpts service.JobOptions
	fs := flag.NewFlagSet("service api", flag.ExitOnError)
	fs.StringVar(&listen, "listen", ":8080", "HTTP listen address")
	fs.StringVar(&jobOpts.Namespace, "namespace", "", "Kubernetes namespace for review jobs")
	fs.StringVar(&jobOpts.ReviewerImage, "reviewer-image", "", "review runner image")
	fs.StringVar(&jobOpts.SidecarImage, "sidecar-image", "", "OpenAI egress sidecar image")
	fs.StringVar(&jobOpts.ServiceAccount, "service-account", "", "Kubernetes service account for review jobs")
	fs.StringVar(&jobOpts.OpenAISecretName, "openai-secret", "", "Kubernetes Secret containing the model API key")
	fs.StringVar(&jobOpts.OpenAISecretKey, "openai-secret-key", "api-key", "Secret key containing the model API key")
	fs.StringVar(&jobOpts.CodexAuthSecretName, "codex-auth-secret", "", "optional Kubernetes Secret containing Codex auth.json literal content in CODEX_AUTH")
	fs.StringVar(&jobOpts.CodexAuthSecretKey, "codex-auth-secret-key", "auth.json", "Secret key containing Codex auth.json literal content")
	fs.StringVar(&jobOpts.GitHubSecretName, "github-secret", "", "optional Kubernetes Secret containing a GitHub token for private repo clones")
	fs.StringVar(&jobOpts.GitHubSecretKey, "github-secret-key", "token", "Secret key containing the GitHub token")
	fs.StringVar(&jobOpts.ProxyURL, "proxy-url", "", "proxy URL exposed by the sidecar")
	fs.IntVar(&jobOpts.ActiveDeadlineSeconds, "active-deadline-seconds", 0, "job activeDeadlineSeconds")
	fs.IntVar(&jobOpts.TTLSeconds, "ttl-seconds", 0, "ttlSecondsAfterFinished")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: codex-reviewer service api [flags]\n\n")
		fs.PrintDefaults()
	}
	_ = fs.Parse(args)
	if fs.NArg() != 0 {
		fs.Usage()
		os.Exit(2)
	}
	server, err := service.NewAPIServer(service.APIOptions{JobOptions: jobOpts})
	if err != nil {
		fmt.Fprintf(os.Stderr, "configure service api failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stdout, "codex-reviewer: listening on %s\n", listen)
	if err := listenAndServe(listen, server.Handler()); err != nil {
		fmt.Fprintf(os.Stderr, "service api failed: %v\n", err)
		os.Exit(1)
	}
}

func runServiceRunner(args []string) {
	fs := flag.NewFlagSet("service runner", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: codex-reviewer service runner\n\n")
		fmt.Fprintf(fs.Output(), "Reads REVIEW_REQUEST_JSON, REVIEW_ID, REVIEW_WORKSPACE, and REVIEW_OUTPUT_DIR from the environment.\n")
	}
	_ = fs.Parse(args)
	if fs.NArg() != 0 {
		fs.Usage()
		os.Exit(2)
	}

	requestJSON := os.Getenv("REVIEW_REQUEST_JSON")
	if requestJSON == "" {
		fmt.Fprintln(os.Stderr, "REVIEW_REQUEST_JSON is required")
		os.Exit(1)
	}
	opts := service.RunnerOptions{
		ReviewID:    os.Getenv("REVIEW_ID"),
		RequestJSON: requestJSON,
		Workspace:   firstNonEmpty(os.Getenv("REVIEW_WORKSPACE"), "/workspace"),
		OutputDir:   firstNonEmpty(os.Getenv("REVIEW_OUTPUT_DIR"), "/out"),
		Stdout:      os.Stdout,
		Stderr:      os.Stderr,
	}
	if err := service.RunReviewJob(context.Background(), opts); err != nil {
		fmt.Fprintf(os.Stderr, "review runner failed: %v\n", err)
		os.Exit(1)
	}
}

func runServiceJobManifest(args []string) {
	var submitOpts service.SubmitOptions
	var jobOpts service.JobOptions
	var output string
	fs := flag.NewFlagSet("service job-manifest", flag.ExitOnError)
	fs.StringVar(&submitOpts.RepoURL, "repo-url", "", "repository URL; defaults to git remote.origin.url")
	fs.StringVar(&submitOpts.BaseRef, "base", "", "base branch/ref; defaults to origin/main")
	fs.StringVar(&submitOpts.HeadRef, "head", "", "head branch/ref; defaults to HEAD")
	fs.StringVar(&submitOpts.HeadSHA, "head-sha", "", "exact head SHA; defaults to resolving --head")
	fs.StringVar(&submitOpts.ProfileName, "profile", "", "review profile; defaults to standard")
	fs.StringVar(&submitOpts.Instructions, "instructions", "", "additional review instructions")
	fs.Var((*stringListFlag)(&submitOpts.Directives), "directive", "review directive; repeat to override configured directives")
	fs.Var((*stringListFlag)(&submitOpts.Ignore), "ignore", "review ignore glob; repeat to override configured ignore globs")
	fs.StringVar(&submitOpts.PolicyFile, "policy-file", "", "repository policy file to include in review context")
	fs.BoolVar(&submitOpts.RequireCleanTree, "require-clean-tree", true, "require a clean committed working tree")
	fs.StringVar(&jobOpts.ReviewID, "review-id", "", "review id used in the Kubernetes Job name")
	fs.StringVar(&jobOpts.Namespace, "namespace", "", "Kubernetes namespace")
	fs.StringVar(&jobOpts.ReviewerImage, "reviewer-image", "", "review runner image")
	fs.StringVar(&jobOpts.SidecarImage, "sidecar-image", "", "OpenAI egress sidecar image")
	fs.StringVar(&jobOpts.ServiceAccount, "service-account", "", "Kubernetes service account")
	fs.StringVar(&jobOpts.OpenAISecretName, "openai-secret", "", "Kubernetes Secret containing the model API key")
	fs.StringVar(&jobOpts.OpenAISecretKey, "openai-secret-key", "api-key", "Secret key containing the model API key")
	fs.StringVar(&jobOpts.CodexAuthSecretName, "codex-auth-secret", "", "optional Kubernetes Secret containing Codex auth.json literal content in CODEX_AUTH")
	fs.StringVar(&jobOpts.CodexAuthSecretKey, "codex-auth-secret-key", "auth.json", "Secret key containing Codex auth.json literal content")
	fs.StringVar(&jobOpts.GitHubSecretName, "github-secret", "", "optional Kubernetes Secret containing a GitHub token for private repo clones")
	fs.StringVar(&jobOpts.GitHubSecretKey, "github-secret-key", "token", "Secret key containing the GitHub token")
	fs.StringVar(&jobOpts.ProxyURL, "proxy-url", "", "proxy URL exposed by the sidecar")
	fs.IntVar(&jobOpts.ActiveDeadlineSeconds, "active-deadline-seconds", 0, "job activeDeadlineSeconds")
	fs.IntVar(&jobOpts.TTLSeconds, "ttl-seconds", 0, "ttlSecondsAfterFinished")
	fs.StringVar(&output, "output", "", "write manifest JSON to this path")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: codex-reviewer service job-manifest [flags]\n\n")
		fs.PrintDefaults()
	}
	_ = fs.Parse(args)
	if fs.NArg() != 0 {
		fs.Usage()
		os.Exit(2)
	}

	submitOpts.Dir = "."
	req, err := service.BuildReviewRequest(context.Background(), submitOpts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "build review request failed: %v\n", err)
		os.Exit(1)
	}
	data, err := service.JobManifest(req, jobOpts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "build job manifest failed: %v\n", err)
		os.Exit(1)
	}
	if output != "" {
		if err := writeOutputFile(output, data); err != nil {
			fmt.Fprintf(os.Stderr, "write %s failed: %v\n", output, err)
			os.Exit(1)
		}
		return
	}
	fmt.Print(string(data))
}

func runServiceSubmit(args []string) {
	var opts service.SubmitOptions
	var output string
	var apiURL string
	var dryRun bool
	var wait bool
	var track bool
	var waitTimeout time.Duration
	reviewerCfg := codexconfig.LoadReviewerConfig()
	defaultAPIURL := os.Getenv("CODEX_REVIEWER_API_URL")
	if defaultAPIURL == "" && reviewerCfg.Backend == "k8s" {
		defaultAPIURL = reviewerCfg.K8sAPIURL
	}
	fs := flag.NewFlagSet("service submit", flag.ExitOnError)
	fs.StringVar(&apiURL, "api-url", defaultAPIURL, "review API base URL; defaults to CODEX_REVIEWER_API_URL or [codex_reviewer].k8s_api_url when backend is k8s")
	fs.StringVar(&opts.RepoURL, "repo-url", "", "repository URL; defaults to git remote.origin.url")
	fs.StringVar(&opts.BaseRef, "base", "", "base branch/ref; defaults to origin/main")
	fs.StringVar(&opts.HeadRef, "head", "", "head branch/ref; defaults to HEAD")
	fs.StringVar(&opts.HeadSHA, "head-sha", "", "exact head SHA; defaults to resolving --head")
	fs.StringVar(&opts.ProfileName, "profile", "", "review profile; defaults to standard")
	fs.StringVar(&opts.Instructions, "instructions", "", "additional review instructions")
	fs.Var((*stringListFlag)(&opts.Directives), "directive", "review directive; repeat to override configured directives")
	fs.Var((*stringListFlag)(&opts.Ignore), "ignore", "review ignore glob; repeat to override configured ignore globs")
	fs.StringVar(&opts.PolicyFile, "policy-file", "", "repository policy file to include in review context")
	fs.StringVar(&opts.ReturnFormat, "return-format", "markdown", "review output format")
	fs.StringVar(&output, "output", "", "write dry-run request JSON to this path")
	fs.BoolVar(&dryRun, "dry-run", false, "build and print the review request without submitting it")
	fs.BoolVar(&wait, "wait", false, "wait for the remote review to finish")
	fs.BoolVar(&track, "track", true, "write a non-secret review record under codex-review/k8s-reviews")
	fs.DurationVar(&waitTimeout, "timeout", 10*time.Minute, "maximum time to wait for the review report when --wait is set")
	fs.BoolVar(&opts.RequireCleanTree, "require-clean-tree", true, "require a clean committed working tree")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: codex-reviewer service submit [flags]\n\n")
		fs.PrintDefaults()
	}
	_ = fs.Parse(args)
	if fs.NArg() != 0 {
		fs.Usage()
		os.Exit(2)
	}

	opts.Dir = "."
	req, err := service.BuildReviewRequest(context.Background(), opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "build review request failed: %v\n", err)
		os.Exit(1)
	}
	if dryRun {
		data, err := service.DryRunReviewRequestJSON(context.Background(), opts.Dir, req)
		if err != nil {
			fmt.Fprintf(os.Stderr, "encode review request failed: %v\n", err)
			os.Exit(1)
		}
		if output != "" {
			if err := writeOutputFile(output, data); err != nil {
				fmt.Fprintf(os.Stderr, "write %s failed: %v\n", output, err)
				os.Exit(1)
			}
		} else {
			fmt.Print(string(data))
		}
		return
	}
	if apiURL == "" {
		fmt.Fprintln(os.Stderr, "service submit requires --api-url, CODEX_REVIEWER_API_URL, or [codex_reviewer].k8s_api_url with backend = \"k8s\"; use --dry-run to inspect the request")
		os.Exit(1)
	}
	resp, err := service.Client{BaseURL: apiURL}.Submit(context.Background(), req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "submit review failed: %v\n", err)
		os.Exit(1)
	}
	if wait {
		waitCtx, waitCancel := context.WithTimeout(context.Background(), waitTimeout)
		defer waitCancel()
		report, err := service.Client{BaseURL: apiURL}.WaitReport(waitCtx, resp.ReportURL, 5*time.Second)
		if err != nil {
			fmt.Fprintf(os.Stderr, "wait for review report failed: %v\n", err)
			os.Exit(1)
		}
		if output != "" {
			if err := writeOutputFile(output, report); err != nil {
				fmt.Fprintf(os.Stderr, "write %s failed: %v\n", output, err)
				os.Exit(1)
			}
		} else {
			fmt.Print(string(report))
		}
		if track {
			recordPath, err := service.TrackReview(service.TrackReviewOptions{
				Dir:        ".",
				APIURL:     apiURL,
				Request:    req,
				Response:   resp,
				Report:     report,
				ReportPath: output,
			})
			if err != nil {
				fmt.Fprintf(os.Stderr, "track review failed: %v\n", err)
				os.Exit(1)
			}
			fmt.Fprintf(os.Stderr, "codex-reviewer: tracked review in %s\n", recordPath)
		}
		return
	}
	if track {
		recordPath, err := service.TrackReview(service.TrackReviewOptions{
			Dir:      ".",
			APIURL:   apiURL,
			Request:  req,
			Response: resp,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "track review failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "codex-reviewer: tracked review in %s\n", recordPath)
	}
	respData, err := json.MarshalIndent(resp, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "encode review response failed: %v\n", err)
		os.Exit(1)
	}
	respData = append(respData, '\n')
	fmt.Print(string(respData))
}

func writeOutputFile(path string, data []byte) error {
	if dir := filepath.Dir(path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(path, data, 0o644)
}

func runReview(args []string) {
	if len(args) == 0 {
		reviewUsage()
		os.Exit(2)
	}
	switch args[0] {
	case "local":
		runReviewLocal(args[1:])
	case "docker":
		runReviewDocker(args[1:])
	case "pre-push":
		runReviewPrePush(args[1:])
	case "recommend":
		runReviewRecommend(args[1:])
	case "-h", "--help", "help":
		reviewUsage()
	default:
		fmt.Fprintf(os.Stderr, "unknown review command: %s\n\n", args[0])
		reviewUsage()
		os.Exit(2)
	}
}

func runReviewDocker(args []string) {
	cfg := codexconfig.LoadReviewerConfig()
	var opts reviewer.DockerOptions
	var report string
	defaultImage := defaultDockerImageForVersion(version)
	fs := flag.NewFlagSet("review docker", flag.ExitOnError)
	fs.StringVar(&opts.Image, "image", defaultImage, "review runner image")
	fs.StringVar(&opts.Pull, "pull", "missing", "Docker pull policy: always, missing, or never")
	fs.StringVar(&opts.Base, "base", "", "optional base branch/ref for a diff review; omit for full repository review")
	fs.StringVar(&report, "report", "", "review report path")
	fs.StringVar(&opts.Instructions, "instructions", "", "custom review instructions")
	fs.StringVar(&opts.Profile, "profile", "", "review profile: standard, pr-readiness, strict, or repo-policy")
	fs.StringVar(&opts.PolicyFile, "policy-file", "", "repository policy file to include in profile context")
	fs.BoolVar(&opts.Structured, "structured", false, "require structured review output with subsystem coverage and limits")
	fs.BoolVar(&opts.Full, "full", false, "review the full repository even when --base is set")
	fs.BoolVar(&opts.DryRun, "dry-run", false, "print the docker command without running it")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: codex-reviewer review docker [flags]\n\n")
		fs.PrintDefaults()
	}
	_ = fs.Parse(args)
	if fs.NArg() != 0 {
		fs.Usage()
		os.Exit(2)
	}

	opts.Dir = "."

	// Apply repo config with the same flag-precedence logic as runReviewLocal so
	// that (a) --base= (explicit empty) is honored rather than overridden and
	// (b) opts.Base is resolved before defaultReviewReport uses it.
	ctx, stop := interruptContext()
	defer stop()
	repoCfg, _, err := reviewer.LoadRepoConfigForDir(ctx, opts.Dir, false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load reviewer config failed: %v\n", err)
		os.Exit(1)
	}
	if !flagWasSet(fs, "base") {
		opts.Base = repoCfg.Base
	}
	if !flagWasSet(fs, "profile") {
		opts.Profile = repoCfg.Profile
	}
	if opts.Profile == "" {
		opts.Profile = "standard"
	}
	if !flagWasSet(fs, "policy-file") {
		opts.PolicyFile = repoCfg.PolicyFile
	}
	opts.Ignore = repoCfg.Ignore
	opts.Directives = repoCfg.Directives

	opts.Report = defaultReviewReport(report, cfg.Report, opts.Base, opts.Full)
	opts.Stdout = os.Stdout
	opts.Stderr = os.Stderr
	if !opts.DryRun {
		requireGlobalSetupCurrent()
	}
	if err := reviewer.RunDocker(ctx, opts); err != nil {
		fmt.Fprintf(os.Stderr, "docker review failed: %v\n", err)
		os.Exit(1)
	}
}

func defaultDockerImageForVersion(version string) string {
	tag := dockerTagForVersion(version)
	return reviewer.DefaultDockerRepository + ":" + tag
}

func dockerTagForVersion(version string) string {
	tag := versionutil.ReleaseTag(version)
	if tag == "" || tag == "dev" {
		return "latest"
	}
	return tag
}

func runReviewLocal(args []string) {
	cfg := codexconfig.LoadReviewerConfig()
	var opts reviewer.LocalOptions
	var report string
	var recommend bool
	fs := flag.NewFlagSet("review local", flag.ExitOnError)
	fs.StringVar(&opts.Base, "base", "", "optional base branch/ref for a diff review; omit for full repository review")
	fs.StringVar(&report, "report", "", "review report path")
	fs.StringVar(&opts.Instructions, "instructions", "", "custom review instructions")
	fs.StringVar(&opts.Profile, "profile", "", "review profile: standard, pr-readiness, strict, or repo-policy")
	fs.StringVar(&opts.PolicyFile, "policy-file", "", "repository policy file to include in profile context")
	fs.BoolVar(&opts.Structured, "structured", false, "require structured review output with subsystem coverage and limits")
	fs.BoolVar(&opts.Full, "full", false, "review the full repository even when --base is set")
	fs.BoolVar(&recommend, "recommend", false, "print an advisory review mode recommendation instead of running Codex")
	fs.BoolVar(&opts.DryRun, "dry-run", false, "print the codex command without running it")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: codex-reviewer review local [flags]\n\n")
		fs.PrintDefaults()
	}
	_ = fs.Parse(args)
	if fs.NArg() != 0 {
		fs.Usage()
		os.Exit(2)
	}

	opts.Dir = "."
	repoCfg, _, err := reviewer.LoadRepoConfigForDir(context.Background(), opts.Dir, false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load reviewer config failed: %v\n", err)
		os.Exit(1)
	}
	if !flagWasSet(fs, "base") {
		opts.Base = repoCfg.Base
	}
	if !flagWasSet(fs, "profile") {
		opts.Profile = repoCfg.Profile
	}
	if opts.Profile == "" {
		opts.Profile = "standard"
	}
	if !flagWasSet(fs, "policy-file") {
		opts.PolicyFile = repoCfg.PolicyFile
	}
	if recommend {
		runRecommendationForBase(opts.Base)
		return
	}
	opts.Report = defaultReviewReport(report, cfg.Report, opts.Base, opts.Full)
	opts.Stdout = os.Stdout
	opts.Stderr = os.Stderr
	if !opts.DryRun {
		requireGlobalSetupCurrent()
	}
	ctx, stop := interruptContext()
	defer stop()
	if err := reviewer.RunLocal(ctx, opts); err != nil {
		fmt.Fprintf(os.Stderr, "local review failed: %v\n", err)
		os.Exit(1)
	}
}

func runReviewRecommend(args []string) {
	var opts reviewer.RecommendOptions
	fs := flag.NewFlagSet("review recommend", flag.ExitOnError)
	fs.StringVar(&opts.Base, "base", "", "base branch/ref to summarize; defaults to config, upstream, origin/main, origin/master, then main")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: codex-reviewer review recommend [flags]\n\n")
		fs.PrintDefaults()
	}
	_ = fs.Parse(args)
	if fs.NArg() != 0 {
		fs.Usage()
		os.Exit(2)
	}

	opts.Dir = "."
	opts.Stdout = os.Stdout
	ctx, stop := interruptContext()
	defer stop()
	if err := reviewer.RunRecommend(ctx, opts); err != nil {
		fmt.Fprintf(os.Stderr, "review recommendation failed: %v\n", err)
		os.Exit(1)
	}
}

func defaultReviewReport(flagReport, configReport, base string, full bool) string {
	if flagReport != "" {
		return flagReport
	}
	if base != "" && !full && (configReport == "" || configReport == "codex-review/full-review.md") {
		return "codex-review/branch-review.md"
	}
	return configReport
}

func requireGlobalSetupCurrent() {
	if err := installer.CheckGlobalAgentVersion("", version); err != nil {
		fmt.Fprintf(os.Stderr, "global setup check failed: %v\n", err)
		os.Exit(1)
	}
}

func runReviewPrePush(args []string) {
	var opts reviewer.PrePushOptions
	var recommend bool
	fs := flag.NewFlagSet("review pre-push", flag.ExitOnError)
	fs.StringVar(&opts.Base, "base", "", "base branch/ref to review against; defaults to config, upstream, origin/main, origin/master, then main")
	fs.StringVar(&opts.Report, "report", "", "review report path; defaults to config")
	fs.StringVar(&opts.BlockOn, "block-on", "", "when to block push: block or never; defaults to config")
	fs.BoolVar(&opts.AllowDirty, "allow-dirty", false, "allow review to run with a dirty working tree")
	fs.BoolVar(&recommend, "recommend", false, "print an advisory review mode recommendation instead of running Codex")
	fs.BoolVar(&opts.DryRun, "dry-run", false, "print the codex review command without running it")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: codex-reviewer review pre-push [flags]\n\n")
		fs.PrintDefaults()
	}
	_ = fs.Parse(args)
	if fs.NArg() != 0 {
		fs.Usage()
		os.Exit(2)
	}

	opts.Dir = "."
	opts.Version = version
	opts.Stdout = os.Stdout
	opts.Stderr = os.Stderr
	if recommend {
		base := opts.Base
		if !flagWasSet(fs, "base") {
			ctx, stop := interruptContext()
			defer stop()
			if repoCfg, _, err := reviewer.LoadRepoConfigForDir(ctx, opts.Dir, false); err == nil {
				base = firstNonEmpty(repoCfg.PrePushBase, repoCfg.Base)
			}
		}
		runRecommendationForBase(base)
		return
	}
	if !opts.DryRun {
		requireGlobalSetupCurrent()
	}
	ctx, stop := interruptContext()
	defer stop()
	if err := reviewer.RunPrePush(ctx, opts); err != nil {
		fmt.Fprintf(os.Stderr, "pre-push review failed: %v\n", err)
		os.Exit(1)
	}
}

func runRecommendationForBase(base string) {
	opts := reviewer.RecommendOptions{Dir: ".", Base: base, Stdout: os.Stdout}
	ctx, stop := interruptContext()
	defer stop()
	if err := reviewer.RunRecommend(ctx, opts); err != nil {
		fmt.Fprintf(os.Stderr, "review recommendation failed: %v\n", err)
		os.Exit(1)
	}
}

func flagWasSet(fs *flag.FlagSet, name string) bool {
	wasSet := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == name {
			wasSet = true
		}
	})
	return wasSet
}

func runInstall(args []string) {
	var opts installer.Options
	fs := flag.NewFlagSet("install", flag.ExitOnError)
	fs.BoolVar(&opts.DryRun, "dry-run", false, "print planned changes without writing files")
	fs.BoolVar(&opts.Quiet, "quiet", false, "suppress per-file install output")
	fs.StringVar(&opts.AGENTSFile, "agents-file", "AGENTS.md", "repository guidance file to create or extend")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: codex-reviewer install [flags] /path/to/project\n\n")
		fs.PrintDefaults()
	}
	_ = fs.Parse(args)

	if fs.NArg() != 1 {
		fs.Usage()
		os.Exit(2)
	}

	opts.TargetDir = fs.Arg(0)
	opts.Version = version
	result, err := installer.Install(opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "install failed: %v\n", err)
		os.Exit(1)
	}
	if opts.Quiet {
		return
	}

	if opts.DryRun {
		fmt.Println("Dry run complete.")
	} else {
		fmt.Println("Install complete.")
	}
	fmt.Println()
	printActions(result.Actions)
	printWarnings(result.Warnings)

	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Println("  git status --short")
	fmt.Println("  codex-reviewer doctor .")
	fmt.Println("  codex review --base main")
}

func runSetup(args []string) {
	var opts installer.GlobalOptions
	var yes bool
	fs := flag.NewFlagSet("setup", flag.ExitOnError)
	fs.StringVar(&opts.CodexHome, "codex-home", "", "Codex home directory; defaults to CODEX_HOME or ~/.codex")
	fs.BoolVar(&opts.DryRun, "dry-run", false, "print planned changes without writing files")
	fs.BoolVar(&yes, "yes", false, "apply setup changes without prompting")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: codex-reviewer setup [flags]\n\n")
		fs.PrintDefaults()
	}
	_ = fs.Parse(args)
	if fs.NArg() != 0 {
		fs.Usage()
		os.Exit(2)
	}
	opts.Version = version

	planOpts := opts
	planOpts.DryRun = true
	result, err := installer.InstallGlobal(planOpts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "setup failed: %v\n", err)
		os.Exit(1)
	}
	if opts.DryRun {
		fmt.Println("Dry run complete.")
		fmt.Println()
		printActions(result.Actions)
		printWarnings(result.Warnings)
		return
	}

	if allSkipped(result.Actions) {
		fmt.Println("Setup already complete.")
		fmt.Println()
		printActions(result.Actions)
		printWarnings(result.Warnings)
		validateGlobalCodexConfig(opts.CodexHome)
		return
	}

	fmt.Println("Setup will make these changes:")
	fmt.Println()
	printActions(result.Actions)
	printWarnings(result.Warnings)
	if !yes && !confirm("Apply these changes now? [y/N] ") {
		fmt.Println()
		fmt.Println("Setup canceled.")
		return
	}

	opts.DryRun = false
	result, err = installer.InstallGlobal(opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "setup failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println()
	fmt.Println("Setup complete.")
	fmt.Println()
	printActions(result.Actions)
	printWarnings(result.Warnings)
	validateGlobalCodexConfig(opts.CodexHome)
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Println("  codex-reviewer review local")
}

func validateGlobalCodexConfig(codexHome string) {
	if err := installer.ValidateGlobalCodexConfig(codexHome); err != nil {
		fmt.Println()
		fmt.Printf("Warning: Codex strict config validation failed: %v\n", err)
		fmt.Println("Run `codex doctor` for details after checking your Codex installation.")
		return
	}
	fmt.Println()
	fmt.Println("Codex strict config validation passed.")
}

func allSkipped(actions []installer.Action) bool {
	if len(actions) == 0 {
		return false
	}
	for _, action := range actions {
		if action.Status != "skip" {
			return false
		}
	}
	return true
}

func confirm(prompt string) bool {
	fmt.Print(prompt)
	reader := bufio.NewReader(os.Stdin)
	reply, err := reader.ReadString('\n')
	if err != nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(reply)) {
	case "y", "yes":
		return true
	default:
		return false
	}
}

func runDoctor(args []string) {
	var agentsFile string
	fs := flag.NewFlagSet("doctor", flag.ExitOnError)
	fs.StringVar(&agentsFile, "agents-file", "AGENTS.md", "repository guidance file to check")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: codex-reviewer doctor [flags] [path]\n\n")
		fs.PrintDefaults()
	}
	_ = fs.Parse(args)

	if fs.NArg() > 1 {
		fs.Usage()
		os.Exit(2)
	}
	target := "."
	if fs.NArg() == 1 {
		target = fs.Arg(0)
	}

	report, err := installer.Doctor(installer.DoctorOptions{
		TargetDir:  target,
		AGENTSFile: agentsFile,
		Version:    version,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "doctor failed: %v\n", err)
		os.Exit(1)
	}

	if report.OK {
		fmt.Println("Codex reviewer setup looks good.")
	} else {
		fmt.Println("Codex reviewer setup is incomplete.")
	}
	fmt.Println()
	printChecks(report.Checks)

	if !report.OK {
		fmt.Println()
		fmt.Println("Run:")
		fmt.Println("  codex-reviewer install " + target)
		os.Exit(1)
	}
}

func printActions(actions []installer.Action) {
	for _, action := range actions {
		fmt.Printf("%-8s %s\n", action.Status, action.Path)
		if action.Detail != "" {
			fmt.Printf("         %s\n", action.Detail)
		}
	}
}

func printWarnings(warnings []string) {
	if len(warnings) == 0 {
		return
	}
	fmt.Println()
	fmt.Println("Warnings:")
	for _, warning := range warnings {
		fmt.Printf("- %s\n", warning)
	}
}

func printChecks(checks []installer.Check) {
	for _, check := range checks {
		fmt.Printf("%-7s %s\n", check.Status, check.Path)
		if check.Detail != "" {
			fmt.Printf("        %s\n", check.Detail)
		}
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `codex-reviewer %s

Usage:
  codex-reviewer setup [flags]
  codex-reviewer install [flags] /path/to/project
  codex-reviewer doctor [flags] /path/to/project
  codex-reviewer review local [flags]
  codex-reviewer review docker [flags]
  codex-reviewer review pre-push [flags]
  codex-reviewer review recommend [flags]
  codex-reviewer service api [flags]
  codex-reviewer service submit [flags]
  codex-reviewer service job-manifest [flags]
  codex-reviewer service runner
  codex-reviewer service telemetry [flags]
  codex-reviewer workflow run [flags]
  codex-reviewer version

`, version)
}

func reviewUsage() {
	fmt.Fprintf(os.Stderr, `codex-reviewer review

Usage:
  codex-reviewer review local [flags]
  codex-reviewer review docker [flags]
  codex-reviewer review pre-push [flags]
  codex-reviewer review recommend [flags]

`)
}

func workflowUsage() {
	fmt.Fprintf(os.Stderr, `codex-reviewer workflow

Usage:
  codex-reviewer workflow run [flags]

`)
}

func serviceUsage() {
	fmt.Fprintf(os.Stderr, `codex-reviewer service

Usage:
  codex-reviewer service api [flags]
  codex-reviewer service submit [flags]
  codex-reviewer service job-manifest [flags]
  codex-reviewer service runner
  codex-reviewer service telemetry [flags]

`)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
