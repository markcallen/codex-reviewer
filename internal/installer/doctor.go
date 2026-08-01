package installer

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/markcallen/codex-reviewer/internal/versionutil"
)

type DoctorOptions struct {
	TargetDir  string
	AGENTSFile string
	Version    string
}

type DoctorReport struct {
	OK     bool
	Checks []Check
}

type Check struct {
	Status string
	Path   string
	Detail string
}

func Doctor(opts DoctorOptions) (DoctorReport, error) {
	if opts.TargetDir == "" {
		return DoctorReport{}, errors.New("target directory is required")
	}
	if opts.AGENTSFile == "" {
		opts.AGENTSFile = "AGENTS.md"
	}
	if opts.Version == "" {
		opts.Version = "dev"
	}
	targetDir, err := filepath.Abs(opts.TargetDir)
	if err != nil {
		return DoctorReport{}, err
	}
	info, err := os.Stat(targetDir)
	if err != nil {
		return DoctorReport{}, fmt.Errorf("target directory: %w", err)
	}
	if !info.IsDir() {
		return DoctorReport{}, fmt.Errorf("target is not a directory: %s", targetDir)
	}

	d := doctorRun{targetDir: targetDir, opts: opts}
	d.checkReviewerConfig()
	d.checkConfig()
	d.checkAGENTS()
	d.checkArtifact(artifact{Source: "artifacts/codex/agents/code-reviewer.toml", Dest: ".codex/agents/code-reviewer.toml"})
	d.checkDoc()
	for _, a := range defaultArtifacts {
		if a.Dest == "docs/code_review.md" || a.Dest == ".codex/agents/code-reviewer.toml" {
			continue
		}
		d.checkArtifact(a)
	}
	sort.SliceStable(d.report.Checks, func(a, b int) bool {
		return d.report.Checks[a].Path < d.report.Checks[b].Path
	})
	d.report.OK = true
	for _, check := range d.report.Checks {
		if check.Status == "missing" || check.Status == "mismatch" || check.Status == "incomplete" {
			d.report.OK = false
			break
		}
	}
	return d.report, nil
}

type doctorRun struct {
	targetDir string
	opts      DoctorOptions
	report    DoctorReport
}

func (d *doctorRun) checkConfig() {
	dest := ".codex/config.toml"
	current, exists, err := readPath(filepath.Join(d.targetDir, filepath.FromSlash(dest)))
	if err != nil {
		d.add("error", dest, err.Error())
		return
	}
	if !exists {
		d.add("missing", dest, "Codex config is not installed")
		return
	}
	missing := missingConfigKeys(current)
	if len(missing) > 0 {
		d.add("incomplete", dest, "missing "+strings.Join(missing, ", "))
		return
	}
	d.add("ok", dest, "review_model, agent limits, and reviewer backend are configured")
}

func (d *doctorRun) checkReviewerConfig() {
	dest := ".codex-reviewer.toml"
	current, exists, err := readPath(filepath.Join(d.targetDir, filepath.FromSlash(dest)))
	if err != nil {
		d.add("error", dest, err.Error())
		return
	}
	if !exists {
		d.add("missing", dest, "codex-reviewer config is not installed")
		return
	}
	version, ok := readTopLevelString(current, "version")
	if !ok {
		d.add("incomplete", dest, "missing top-level version")
		return
	}
	installedVersion := versionutil.ReleaseTag(version)
	runningVersion := versionutil.ReleaseTag(d.opts.Version)
	if installedVersion != runningVersion {
		d.add("mismatch", dest, fmt.Sprintf("installed version %q does not match running version %q", installedVersion, runningVersion))
		return
	}
	d.add("ok", dest, "codex-reviewer version matches")
}

func readTopLevelString(current []byte, key string) (string, bool) {
	lines := splitLines(string(normalizeNewlines(current)))
	section := ""
	for _, line := range lines {
		if match := sectionHeaderRE.FindStringSubmatch(line); match != nil {
			section = strings.TrimSpace(match[1])
			continue
		}
		if section != "" || !keyLineMatches(line, key) {
			continue
		}
		_, value, ok := strings.Cut(line, "=")
		if !ok {
			return "", false
		}
		value = strings.TrimSpace(value)
		if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
			return strings.ReplaceAll(value[1:len(value)-1], `\"`, `"`), true
		}
		return value, true
	}
	return "", false
}

func (d *doctorRun) checkAGENTS() {
	dest := optsAGENTSPath(d.opts.AGENTSFile)
	current, exists, err := readPath(filepath.Join(d.targetDir, filepath.FromSlash(dest)))
	if err != nil {
		d.add("error", dest, err.Error())
		return
	}
	if !exists {
		d.add("missing", dest, "repository guidance file is not installed")
		return
	}
	text := string(normalizeNewlines(current))
	if strings.Contains(text, "BEGIN "+reviewBlockMarker+": agents-review-expectations") ||
		strings.Contains(text, "Follow `docs/code_review.md` for code reviews.") {
		d.add("ok", dest, "review expectations are present")
		return
	}
	d.add("incomplete", dest, "review expectations are not present")
}

func (d *doctorRun) checkDoc() {
	dest := "docs/code_review.md"
	current, exists, err := readPath(filepath.Join(d.targetDir, filepath.FromSlash(dest)))
	if err != nil {
		d.add("error", dest, err.Error())
		return
	}
	if !exists {
		d.add("missing", dest, "review checklist is not installed")
		return
	}
	text := string(normalizeNewlines(current))
	if strings.Contains(text, "Code review checklist for Codex") ||
		strings.Contains(text, "BEGIN "+reviewBlockMarker+": code-review-checklist") {
		d.add("ok", dest, "review checklist is present")
		return
	}
	d.add("incomplete", dest, "review checklist is not present")
}

func (d *doctorRun) checkArtifact(a artifact) {
	bundled, err := readArtifact(a.Source)
	if err != nil {
		d.add("error", a.Dest, err.Error())
		return
	}
	current, exists, err := readPath(filepath.Join(d.targetDir, filepath.FromSlash(a.Dest)))
	if err != nil {
		d.add("error", a.Dest, err.Error())
		return
	}
	if !exists {
		d.add("missing", a.Dest, "bundled artifact is not installed")
		return
	}
	if bytes.Equal(normalizeNewlines(current), normalizeNewlines(bundled)) {
		d.add("ok", a.Dest, "matches bundled artifact")
		return
	}
	d.add("mismatch", a.Dest, "file exists but differs from bundled artifact")
}

func (d *doctorRun) add(status, path, detail string) {
	d.report.Checks = append(d.report.Checks, Check{Status: status, Path: path, Detail: detail})
}

func readPath(path string) ([]byte, bool, error) {
	data, err := os.ReadFile(path)
	if err == nil {
		return data, true, nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	return nil, false, err
}

func missingConfigKeys(current []byte) []string {
	lines := splitLines(string(normalizeNewlines(current)))
	var missing []string
	if !hasTopLevelKey(lines, "review_model") {
		missing = append(missing, "review_model")
	}
	start, end := findSection(lines, "agents")
	if start == -1 {
		missing = append(missing, "[agents].max_threads", "[agents].max_depth")
	} else {
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
		if !hasThreads {
			missing = append(missing, "[agents].max_threads")
		}
		if !hasDepth {
			missing = append(missing, "[agents].max_depth")
		}
	}

	start, end = findSection(lines, "codex_reviewer")
	if start == -1 {
		return append(missing, "[codex_reviewer].backend", "[codex_reviewer].report", "[codex_reviewer].k8s_api_url")
	}
	if !sectionHasKey(lines[start+1:end], "backend") {
		missing = append(missing, "[codex_reviewer].backend")
	}
	if !sectionHasKey(lines[start+1:end], "report") {
		missing = append(missing, "[codex_reviewer].report")
	}
	if !sectionHasKey(lines[start+1:end], "k8s_api_url") {
		missing = append(missing, "[codex_reviewer].k8s_api_url")
	}
	return missing
}
