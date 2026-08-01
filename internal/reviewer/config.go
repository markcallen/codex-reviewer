package reviewer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const ConfigFile = ".codex-reviewer.toml"

type RepoConfig struct {
	Version          string
	Base             string
	PrePushBase      string
	Ignore           []string
	Directives       []string
	Profile          string
	PolicyFile       string
	BlockOn          string
	Report           string
	RequireCleanTree bool
}

func LoadRepoConfig(repoRoot string, requireVersion bool) (RepoConfig, bool, error) {
	cfg := RepoConfig{
		BlockOn:          "block",
		Report:           ".git/codex-review/pre-push-review.md",
		RequireCleanTree: true,
	}
	data, err := os.ReadFile(filepath.Join(repoRoot, ConfigFile))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if requireVersion {
				return RepoConfig{}, false, fmt.Errorf("%s is missing; run codex-reviewer install ./", ConfigFile)
			}
			return cfg, false, nil
		}
		return RepoConfig{}, false, fmt.Errorf("read %s: %w", ConfigFile, err)
	}

	section := ""
	for _, line := range logicalConfigLines(string(data)) {
		line = stripComment(strings.TrimSpace(line))
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "["), "]"))
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		switch section {
		case "":
			if key == "version" {
				cfg.Version = unquote(value)
			}
		case "review":
			switch key {
			case "base":
				cfg.Base = unquote(value)
			case "ignore", "ignore_globs":
				cfg.Ignore = parseStringList(value)
			case "directives":
				cfg.Directives = parseStringList(value)
			case "profile":
				cfg.Profile = unquote(value)
			case "policy_file":
				cfg.PolicyFile = unquote(value)
			}
		case "review.pre_push":
			switch key {
			case "base":
				cfg.PrePushBase = unquote(value)
			case "block_on":
				cfg.BlockOn = unquote(value)
			case "report":
				cfg.Report = unquote(value)
			case "require_clean_tree":
				cfg.RequireCleanTree = value != "false"
			}
		}
	}
	if requireVersion && cfg.Version == "" {
		return RepoConfig{}, true, fmt.Errorf("%s is missing required top-level version", ConfigFile)
	}
	return cfg, true, nil
}

func LoadRepoConfigForDir(ctx context.Context, dir string, requireVersion bool) (RepoConfig, bool, error) {
	if dir == "" {
		dir = "."
	}
	repoRoot, err := gitOutput(ctx, dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return RepoConfig{}, false, fmt.Errorf("find git repository root: %w", err)
	}
	return LoadRepoConfig(strings.TrimSpace(repoRoot), requireVersion)
}

func logicalConfigLines(text string) []string {
	var lines []string
	var current string
	depth := 0
	for _, raw := range strings.Split(text, "\n") {
		line := stripComment(strings.TrimSpace(raw))
		if line == "" {
			continue
		}
		if current == "" {
			current = line
		} else {
			current += " " + line
		}
		depth += bracketDelta(line)
		if depth <= 0 {
			lines = append(lines, current)
			current = ""
			depth = 0
		}
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines
}

func bracketDelta(line string) int {
	inQuote := false
	escaped := false
	delta := 0
	for _, r := range line {
		switch {
		case escaped:
			escaped = false
		case r == '\\' && inQuote:
			escaped = true
		case r == '"':
			inQuote = !inQuote
		case r == '[' && !inQuote:
			delta++
		case r == ']' && !inQuote:
			delta--
		}
	}
	return delta
}

func parseStringList(value string) []string {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, "[") || !strings.HasSuffix(value, "]") {
		if single := strings.TrimSpace(unquote(value)); single != "" {
			return []string{single}
		}
		return nil
	}
	value = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(value, "["), "]"))
	if value == "" {
		return nil
	}
	var out []string
	var part strings.Builder
	inQuote := false
	escaped := false
	for _, r := range value {
		switch {
		case escaped:
			part.WriteRune(r)
			escaped = false
		case r == '\\' && inQuote:
			escaped = true
		case r == '"':
			inQuote = !inQuote
			part.WriteRune(r)
		case r == ',' && !inQuote:
			if item := strings.TrimSpace(unquote(part.String())); item != "" {
				out = append(out, item)
			}
			part.Reset()
		default:
			part.WriteRune(r)
		}
	}
	if item := strings.TrimSpace(unquote(part.String())); item != "" {
		out = append(out, item)
	}
	return out
}

func loadConfig(path string) (RepoConfig, error) {
	cfg, _, err := LoadRepoConfig(filepath.Dir(path), true)
	return cfg, err
}

func pathspecExcludes(ignore []string) []string {
	var out []string
	for _, pattern := range ignore {
		pattern = strings.TrimSpace(filepath.ToSlash(pattern))
		if pattern == "" {
			continue
		}
		out = append(out, ":(exclude)"+pattern)
	}
	return out
}

func matchesIgnoredPath(path string, ignore []string) bool {
	path = strings.TrimPrefix(filepath.ToSlash(path), "./")
	for _, pattern := range ignore {
		pattern = strings.TrimPrefix(filepath.ToSlash(strings.TrimSpace(pattern)), "./")
		if pattern == "" {
			continue
		}
		if ok, _ := filepath.Match(pattern, path); ok {
			return true
		}
		if strings.HasSuffix(pattern, "/**") && strings.HasPrefix(path, strings.TrimSuffix(pattern, "**")) {
			return true
		}
		if strings.HasSuffix(pattern, "/") && strings.HasPrefix(path, pattern) {
			return true
		}
		if !strings.Contains(pattern, "/") {
			parts := strings.Split(path, "/")
			for _, part := range parts {
				if ok, _ := filepath.Match(pattern, part); ok {
					return true
				}
			}
		}
	}
	return false
}
