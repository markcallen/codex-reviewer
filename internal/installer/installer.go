package installer

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/everydaydevops/codex-code-reviewer/internal/versionutil"
)

const (
	reviewBlockMarker        = "CODEX REVIEWER INSTALLER"
	globalAgentVersionPrefix = "# codex-reviewer-version = "
)

var defaultArtifacts = []artifact{
	{Source: "artifacts/codex/agents/code-reviewer.toml", Dest: ".codex/agents/code-reviewer.toml", Merge: mergeNone},
	{Source: "artifacts/docs/code_review.md", Dest: "docs/code_review.md", Merge: mergeCodeReviewDoc},
	{Source: "artifacts/prompts/review-branch.md", Dest: "prompts/review-branch.md", Merge: mergeNone},
	{Source: "artifacts/prompts/review-commit.md", Dest: "prompts/review-commit.md", Merge: mergeNone},
	{Source: "artifacts/prompts/review-pr.md", Dest: "prompts/review-pr.md", Merge: mergeNone},
	{Source: "artifacts/prompts/review-uncommitted.md", Dest: "prompts/review-uncommitted.md", Merge: mergeNone},
}

type mergeMode int

const (
	mergeNone mergeMode = iota
	mergeCodeReviewDoc
)

type artifact struct {
	Source string
	Dest   string
	Merge  mergeMode
}

type Options struct {
	TargetDir  string
	AGENTSFile string
	Version    string
	DryRun     bool
	Quiet      bool
}

type Result struct {
	Actions  []Action
	Warnings []string
}

type Action struct {
	Status string
	Path   string
	Detail string
}

type GlobalOptions struct {
	CodexHome string
	Version   string
	DryRun    bool
}

func Install(opts Options) (Result, error) {
	if opts.TargetDir == "" {
		return Result{}, errors.New("target directory is required")
	}
	if opts.AGENTSFile == "" {
		opts.AGENTSFile = "AGENTS.md"
	}
	if cleaned := filepath.ToSlash(filepath.Clean(opts.AGENTSFile)); filepath.IsAbs(opts.AGENTSFile) || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return Result{}, fmt.Errorf("--agents-file must be a relative path within the target directory, got %q", opts.AGENTSFile)
	}
	if opts.Version == "" {
		opts.Version = "dev"
	}
	targetDir, err := filepath.Abs(opts.TargetDir)
	if err != nil {
		return Result{}, err
	}
	info, err := os.Stat(targetDir)
	if err != nil {
		return Result{}, fmt.Errorf("target directory: %w", err)
	}
	if !info.IsDir() {
		return Result{}, fmt.Errorf("target is not a directory: %s", targetDir)
	}

	i := installRun{opts: opts, targetDir: targetDir}
	if err := i.installReviewerConfig(); err != nil {
		return i.result, err
	}
	if err := i.installConfig(); err != nil {
		return i.result, err
	}
	if err := i.installAGENTS(); err != nil {
		return i.result, err
	}
	for _, a := range defaultArtifacts {
		if err := i.installArtifact(a); err != nil {
			return i.result, err
		}
	}
	sort.SliceStable(i.result.Actions, func(a, b int) bool {
		return i.result.Actions[a].Path < i.result.Actions[b].Path
	})
	return i.result, nil
}

func InstallGlobal(opts GlobalOptions) (Result, error) {
	if opts.Version == "" {
		opts.Version = "dev"
	}
	codexHome := opts.CodexHome
	if codexHome == "" {
		if env := os.Getenv("CODEX_HOME"); env != "" {
			codexHome = env
		} else {
			home, err := os.UserHomeDir()
			if err != nil {
				return Result{}, fmt.Errorf("discover home directory: %w", err)
			}
			codexHome = filepath.Join(home, ".codex")
		}
	}
	codexHome, err := filepath.Abs(codexHome)
	if err != nil {
		return Result{}, err
	}

	i := installRun{opts: Options{DryRun: opts.DryRun, Version: opts.Version}, targetDir: codexHome}
	if err := i.installGlobalAgent(); err != nil {
		return i.result, err
	}
	if err := i.installGlobalConfig(); err != nil {
		return i.result, err
	}
	sort.SliceStable(i.result.Actions, func(a, b int) bool {
		return i.result.Actions[a].Path < i.result.Actions[b].Path
	})
	return i.result, nil
}

type installRun struct {
	opts      Options
	targetDir string
	result    Result
}

func (i *installRun) installReviewerConfig() error {
	dest := ".codex-reviewer.toml"
	current, exists, err := i.readTarget(dest)
	if err != nil {
		return err
	}
	if !exists {
		return i.writeTarget(dest, []byte(defaultReviewerConfig(i.opts.Version)), "create", "installed codex-reviewer config")
	}
	next := mergeReviewerConfigVersion(current, i.opts.Version)
	if bytes.Equal(current, next) {
		i.add("skip", dest, "codex-reviewer config already current")
		return nil
	}
	return i.writeTarget(dest, next, "merge", "updated codex-reviewer version")
}

func (i *installRun) installConfig() error {
	dest := ".codex/config.toml"
	bundled, err := readArtifact("artifacts/codex/config.toml")
	if err != nil {
		return err
	}
	current, exists, err := i.readTarget(dest)
	if err != nil {
		return err
	}
	if !exists {
		return i.writeTarget(dest, bundled, "create", "installed bundled Codex config")
	}
	next := mergeConfig(current)
	if bytes.Equal(current, next) {
		i.add("skip", dest, "review config already present")
		return nil
	}
	return i.writeTarget(dest, next, "merge", "added review_model and missing [agents] limits")
}

func (i *installRun) installGlobalAgent() error {
	dest := "agents/code-reviewer.toml"
	bundled, err := readArtifact("artifacts/codex/agents/code-reviewer.toml")
	if err != nil {
		return err
	}
	versioned := addGlobalAgentVersion(bundled, i.opts.Version)
	current, exists, err := i.readTarget(dest)
	if err != nil {
		return err
	}
	if !exists {
		return i.writeTarget(dest, versioned, "create", "installed bundled artifact")
	}
	if bytes.Equal(normalizeNewlines(current), normalizeNewlines(versioned)) {
		i.add("skip", dest, "already installed")
		return nil
	}
	if bytes.Equal(removeGlobalAgentVersion(current), normalizeNewlines(bundled)) {
		return i.writeTarget(dest, versioned, "merge", "updated global reviewer agent version")
	}
	versioned = addGlobalAgentVersion(current, i.opts.Version)
	i.result.Warnings = append(i.result.Warnings, fmt.Sprintf("%s differs from the bundled artifact; updated version marker only, body left unchanged", dest))
	return i.writeTarget(dest, versioned, "merge", "updated version marker on custom global reviewer agent")
}

func (i *installRun) installGlobalConfig() error {
	dest := "config.toml"
	current, exists, err := i.readTarget(dest)
	if err != nil {
		return err
	}
	var next []byte
	if exists {
		next = mergeGlobalConfig(current)
		if bytes.Equal(current, next) {
			i.add("skip", dest, "global Codex reviewer settings already present")
			return nil
		}
		return i.writeTarget(dest, next, "merge", "added missing global Codex reviewer settings")
	}
	next = mergeGlobalConfig(nil)
	return i.writeTarget(dest, next, "create", "installed global Codex reviewer settings")
}

func defaultReviewerConfig(version string) string {
	version = versionutil.ReleaseTag(version)
	return fmt.Sprintf(`version = %q

[review.pre_push]
base = ""
block_on = "block"
report = ".git/codex-review/pre-push-review.md"
require_clean_tree = true
`, version)
}

func mergeReviewerConfigVersion(current []byte, version string) []byte {
	version = versionutil.ReleaseTag(version)
	lines := splitLines(string(normalizeNewlines(current)))
	for idx, line := range lines {
		if sectionHeaderRE.MatchString(line) {
			break
		}
		if keyLineMatches(line, "version") {
			lines[idx] = fmt.Sprintf("version = %q", version)
			return []byte(strings.Join(lines, "\n") + "\n")
		}
	}
	lines = appendTopLevelKey(lines, fmt.Sprintf("version = %q", version))
	return []byte(strings.Join(lines, "\n") + "\n")
}

func (i *installRun) installAGENTS() error {
	dest := optsAGENTSPath(i.opts.AGENTSFile)
	bundled, err := readArtifact("artifacts/AGENTS.md")
	if err != nil {
		return err
	}
	current, exists, err := i.readTarget(dest)
	if err != nil {
		return err
	}
	if !exists {
		return i.writeTarget(dest, bundled, "create", "installed repository review guidance")
	}
	text := string(normalizeNewlines(current))
	if strings.Contains(text, "BEGIN "+reviewBlockMarker+": agents-review-expectations") ||
		strings.Contains(text, "Follow `docs/code_review.md` for code reviews.") {
		i.add("skip", dest, "review guidance already present")
		return nil
	}
	next := appendManagedBlock(current, "agents-review-expectations", agentsReviewBlock())
	if bytes.Equal(current, next) {
		i.add("skip", dest, "review guidance already present")
		return nil
	}
	return i.writeTarget(dest, next, "append", "added Codex code review expectations")
}

func (i *installRun) installArtifact(a artifact) error {
	bundled, err := readArtifact(a.Source)
	if err != nil {
		return err
	}
	current, exists, err := i.readTarget(a.Dest)
	if err != nil {
		return err
	}
	if !exists {
		return i.writeTarget(a.Dest, bundled, "create", "installed bundled artifact")
	}
	if bytes.Equal(normalizeNewlines(current), normalizeNewlines(bundled)) {
		i.add("skip", a.Dest, "already installed")
		return nil
	}

	switch a.Merge {
	case mergeCodeReviewDoc:
		next := appendManagedBlock(current, "code-review-checklist", string(bundled))
		if bytes.Equal(current, next) {
			i.add("skip", a.Dest, "review checklist already present")
			return nil
		}
		return i.writeTarget(a.Dest, next, "append", "added bundled review checklist")
	default:
		i.add("keep", a.Dest, "existing file differs; left unchanged")
		i.result.Warnings = append(i.result.Warnings, fmt.Sprintf("%s already exists and differs from the bundled artifact", a.Dest))
		return nil
	}
}

func (i *installRun) readTarget(rel string) ([]byte, bool, error) {
	path := filepath.Join(i.targetDir, filepath.FromSlash(rel))
	data, err := os.ReadFile(path)
	if err == nil {
		return data, true, nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	return nil, false, fmt.Errorf("read %s: %w", rel, err)
}

func (i *installRun) writeTarget(rel string, data []byte, status, detail string) error {
	path := filepath.Join(i.targetDir, filepath.FromSlash(rel))
	if i.opts.DryRun {
		i.add(status, rel, detail)
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create parent directory for %s: %w", rel, err)
	}
	if err := os.WriteFile(path, ensureTrailingNewline(data), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", rel, err)
	}
	i.add(status, rel, detail)
	return nil
}

func (i *installRun) add(status, path, detail string) {
	i.result.Actions = append(i.result.Actions, Action{Status: status, Path: path, Detail: detail})
}

func readArtifact(path string) ([]byte, error) {
	data, err := artifactFS.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read embedded artifact %s: %w", path, err)
	}
	return data, nil
}

func ValidateGlobalCodexConfig(codexHome string) error {
	if codexHome == "" {
		if env := os.Getenv("CODEX_HOME"); env != "" {
			codexHome = env
		} else {
			home, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("discover home directory: %w", err)
			}
			codexHome = filepath.Join(home, ".codex")
		}
	}
	cmd := exec.Command("codex", "--strict-config", "doctor", "--summary", "--no-color", "--ascii")
	cmd.Env = envWith("CODEX_HOME", codexHome)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(strings.Join([]string{stdout.String(), stderr.String()}, "\n"))
		if detail == "" {
			detail = err.Error()
		}
		return fmt.Errorf("codex rejected global config: %s", detail)
	}
	return nil
}

func envWith(key, value string) []string {
	prefix := key + "="
	env := os.Environ()
	next := make([]string, 0, len(env)+1)
	replaced := false
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			if !replaced {
				next = append(next, prefix+value)
				replaced = true
			}
			continue
		}
		next = append(next, entry)
	}
	if !replaced {
		next = append(next, prefix+value)
	}
	return next
}

func CheckGlobalAgentVersion(codexHome, runningVersion string) error {
	runningVersion = versionutil.ReleaseTag(runningVersion)
	if codexHome == "" {
		if env := os.Getenv("CODEX_HOME"); env != "" {
			codexHome = env
		} else {
			home, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("discover home directory: %w", err)
			}
			codexHome = filepath.Join(home, ".codex")
		}
	}
	path := filepath.Join(codexHome, "agents", "code-reviewer.toml")
	current, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("%s is missing; run codex-reviewer setup with this binary", path)
		}
		return fmt.Errorf("read %s: %w", path, err)
	}
	installed, ok := readGlobalAgentVersion(current)
	if !ok {
		return fmt.Errorf("%s has no codex-reviewer version marker; run codex-reviewer setup with this binary", path)
	}
	installed = versionutil.ReleaseTag(installed)
	if installed != runningVersion {
		return fmt.Errorf("%s version mismatch: installed by codex-reviewer %q, running %q; run codex-reviewer setup with this binary", path, installed, runningVersion)
	}
	return nil
}

func addGlobalAgentVersion(agent []byte, version string) []byte {
	version = versionutil.ReleaseTag(version)
	body := strings.TrimLeft(string(removeGlobalAgentVersion(agent)), "\n")
	return []byte(fmt.Sprintf("%s%q\n%s", globalAgentVersionPrefix, version, body))
}

func readGlobalAgentVersion(agent []byte) (string, bool) {
	for _, line := range splitLines(string(normalizeNewlines(agent))) {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, globalAgentVersionPrefix) {
			return "", false
		}
		return unquote(strings.TrimSpace(strings.TrimPrefix(line, globalAgentVersionPrefix))), true
	}
	return "", false
}

func removeGlobalAgentVersion(agent []byte) []byte {
	lines := splitLines(string(normalizeNewlines(agent)))
	for idx, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, globalAgentVersionPrefix) {
			lines = append(lines[:idx], lines[idx+1:]...)
		}
		break
	}
	return []byte(strings.Join(lines, "\n") + "\n")
}

func optsAGENTSPath(path string) string {
	path = filepath.ToSlash(filepath.Clean(path))
	if path == "." {
		return "AGENTS.md"
	}
	return path
}

func appendManagedBlock(current []byte, id, block string) []byte {
	text := string(normalizeNewlines(current))
	start := fmt.Sprintf("<!-- BEGIN %s: %s -->", reviewBlockMarker, id)
	if strings.Contains(text, start) || strings.Contains(text, block) {
		return current
	}
	var b strings.Builder
	b.WriteString(strings.TrimRight(text, "\n"))
	if b.Len() > 0 {
		b.WriteString("\n\n")
	}
	b.WriteString(start)
	b.WriteString("\n")
	b.WriteString(strings.TrimRight(block, "\n"))
	b.WriteString("\n")
	fmt.Fprintf(&b, "<!-- END %s: %s -->", reviewBlockMarker, id)
	b.WriteString("\n")
	return []byte(b.String())
}

func agentsReviewBlock() string {
	return `## Codex code review expectations

- Follow ` + "`docs/code_review.md`" + ` for code reviews.
- Reviews should prioritize correctness, security, regressions, missing tests, and maintainability.
- Avoid style-only feedback unless it hides a defect or contradicts this repository's formatter/linter rules.
- For implementation tasks, run the smallest relevant test, lint, or type-check command before reporting completion.
- Never include secrets in prompts, commits, logs, or generated documentation.

### Review severity guidelines

- Flag P0/P1 issues only when there is a concrete failure mode or credible risk.
- Treat missing tests as P1 when the change affects behavior, auth, billing, persistence, migrations, concurrency, permissions, or user-visible output.
- Treat documentation gaps as P1 only when the change alters setup, public APIs, release/deploy steps, or user-visible behavior.
- Use P3/Nit only for polish, and never block merge on personal preference.
`
}

var sectionHeaderRE = regexp.MustCompile(`^\s*\[([^\]]+)\]\s*(?:#.*)?$`)

func mergeConfig(current []byte) []byte {
	lines := splitLines(string(normalizeNewlines(current)))
	if !hasTopLevelKey(lines, "review_model") {
		lines = appendTopLevelKey(lines, `review_model = "gpt-5.5"`)
	}
	lines = ensureAgentsKeys(lines)
	lines = ensureCodexReviewerKeys(lines)
	return []byte(strings.Join(lines, "\n") + "\n")
}

func mergeGlobalConfig(current []byte) []byte {
	lines := splitLines(string(normalizeNewlines(current)))
	var topLevelAdditions []string
	for _, entry := range []struct {
		key  string
		line string
	}{
		{"model", `model = "gpt-5.5"`},
		{"model_reasoning_effort", `model_reasoning_effort = "medium"`},
		{"model_verbosity", `model_verbosity = "medium"`},
		{"review_model", `review_model = "gpt-5.5"`},
		{"approval_policy", `approval_policy = "on-request"`},
	} {
		if !hasTopLevelKey(lines, entry.key) {
			topLevelAdditions = append(topLevelAdditions, entry.line)
		}
	}
	lines = appendTopLevelKeys(lines, topLevelAdditions)
	lines = ensureAgentsKeys(lines)
	lines = ensureCodexReviewerKeys(lines)
	return []byte(strings.Join(lines, "\n") + "\n")
}

func hasTopLevelKey(lines []string, key string) bool {
	section := ""
	for _, line := range lines {
		if match := sectionHeaderRE.FindStringSubmatch(line); match != nil {
			section = strings.TrimSpace(match[1])
			continue
		}
		if section == "" && keyLineMatches(line, key) {
			return true
		}
	}
	return false
}

func appendTopLevelKey(lines []string, line string) []string {
	return appendTopLevelKeys(lines, []string{line})
}

func appendTopLevelKeys(lines []string, additions []string) []string {
	if len(additions) == 0 {
		return lines
	}
	insert := len(lines)
	for idx, candidate := range lines {
		if sectionHeaderRE.MatchString(candidate) {
			insert = idx
			break
		}
	}
	next := make([]string, 0, len(lines)+len(additions)+2)
	next = append(next, lines[:insert]...)
	if insert > 0 && strings.TrimSpace(next[len(next)-1]) != "" {
		next = append(next, "")
	}
	next = append(next, additions...)
	if insert < len(lines) && strings.TrimSpace(lines[insert]) != "" {
		next = append(next, "")
	}
	next = append(next, lines[insert:]...)
	return next
}

func ensureAgentsKeys(lines []string) []string {
	start, end := findSection(lines, "agents")
	if start == -1 {
		if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) != "" {
			lines = append(lines, "")
		}
		return append(lines, "[agents]", "max_threads = 4", "max_depth = 1")
	}

	hasThreads := false
	hasDepth := false
	for _, line := range lines[start+1 : end] {
		if keyLineMatches(line, "max_threads") {
			hasThreads = true
		}
		if keyLineMatches(line, "max_depth") {
			hasDepth = true
		}
	}
	if hasThreads && hasDepth {
		return lines
	}
	insert := end
	additions := make([]string, 0, 2)
	if !hasThreads {
		additions = append(additions, "max_threads = 4")
	}
	if !hasDepth {
		additions = append(additions, "max_depth = 1")
	}
	next := make([]string, 0, len(lines)+len(additions))
	next = append(next, lines[:insert]...)
	next = append(next, additions...)
	next = append(next, lines[insert:]...)
	return next
}

func ensureCodexReviewerKeys(lines []string) []string {
	start, end := findSection(lines, "codex_reviewer")
	if start == -1 {
		if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) != "" {
			lines = append(lines, "")
		}
		return append(lines,
			"[codex_reviewer]",
			`backend = "local"`,
			`report = "codex-review/full-review.md"`,
			`k8s_api_url = ""`,
		)
	}

	additions := make([]string, 0, 3)
	if !sectionHasKey(lines[start+1:end], "backend") {
		additions = append(additions, `backend = "local"`)
	}
	if !sectionHasKey(lines[start+1:end], "report") {
		additions = append(additions, `report = "codex-review/full-review.md"`)
	}
	if !sectionHasKey(lines[start+1:end], "k8s_api_url") {
		additions = append(additions, `k8s_api_url = ""`)
	}
	if len(additions) == 0 {
		return lines
	}
	next := make([]string, 0, len(lines)+len(additions))
	next = append(next, lines[:end]...)
	next = append(next, additions...)
	next = append(next, lines[end:]...)
	return next
}

func sectionHasKey(lines []string, key string) bool {
	for _, line := range lines {
		if keyLineMatches(line, key) {
			return true
		}
	}
	return false
}

func findSection(lines []string, name string) (int, int) {
	for idx, line := range lines {
		match := sectionHeaderRE.FindStringSubmatch(line)
		if match == nil || strings.TrimSpace(match[1]) != name {
			continue
		}
		end := len(lines)
		for scan := idx + 1; scan < len(lines); scan++ {
			if sectionHeaderRE.MatchString(lines[scan]) {
				end = scan
				break
			}
		}
		return idx, end
	}
	return -1, -1
}

func keyLineMatches(line, key string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return false
	}
	rest := strings.TrimPrefix(trimmed, key)
	if rest == trimmed {
		return false
	}
	rest = strings.TrimLeft(rest, " \t")
	return strings.HasPrefix(rest, "=")
}

func unquote(value string) string {
	if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
		value = value[1 : len(value)-1]
		value = strings.ReplaceAll(value, `\"`, `"`)
		value = strings.ReplaceAll(value, `\\`, `\`)
	}
	return value
}

func splitLines(text string) []string {
	text = strings.TrimRight(text, "\n")
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}

func normalizeNewlines(data []byte) []byte {
	data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	return bytes.ReplaceAll(data, []byte("\r"), []byte("\n"))
}

func ensureTrailingNewline(data []byte) []byte {
	data = normalizeNewlines(data)
	if len(data) == 0 || data[len(data)-1] == '\n' {
		return data
	}
	return append(data, '\n')
}
