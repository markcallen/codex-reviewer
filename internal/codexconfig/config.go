package codexconfig

import (
	"os"
	"path/filepath"
	"strings"
)

type ReviewerConfig struct {
	Backend   string
	Report    string
	K8sAPIURL string
}

func LoadReviewerConfig() ReviewerConfig {
	cfg := ReviewerConfig{
		Backend: "local",
		Report:  "codex-review/full-review.md",
	}
	data, err := os.ReadFile(filepath.Join(codexHome(), "config.toml"))
	if err != nil {
		return cfg
	}
	section := ""
	for _, line := range strings.Split(normalizeNewlines(string(data)), "\n") {
		line = stripComment(strings.TrimSpace(line))
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "["), "]"))
			continue
		}
		if section != "codex_reviewer" {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch strings.TrimSpace(key) {
		case "backend":
			cfg.Backend = unquote(strings.TrimSpace(value))
		case "report":
			cfg.Report = unquote(strings.TrimSpace(value))
		case "k8s_api_url":
			cfg.K8sAPIURL = unquote(strings.TrimSpace(value))
		}
	}
	if cfg.Backend == "" {
		cfg.Backend = "local"
	}
	if cfg.Report == "" {
		cfg.Report = "codex-review/full-review.md"
	}
	return cfg
}

func codexHome() string {
	if home := os.Getenv("CODEX_HOME"); home != "" {
		return home
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".codex")
	}
	return ".codex"
}

func stripComment(line string) string {
	inQuote := false
	escaped := false
	for idx, r := range line {
		if escaped {
			escaped = false
			continue
		}
		if r == '\\' && inQuote {
			escaped = true
			continue
		}
		if r == '"' {
			inQuote = !inQuote
			continue
		}
		if r == '#' && !inQuote {
			return strings.TrimSpace(line[:idx])
		}
	}
	return line
}

func unquote(value string) string {
	if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
		value = value[1 : len(value)-1]
		value = strings.ReplaceAll(value, `\"`, `"`)
		value = strings.ReplaceAll(value, `\\`, `\`)
	}
	return value
}

func normalizeNewlines(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	return strings.ReplaceAll(text, "\r", "\n")
}
