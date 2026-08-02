package service

import (
	"fmt"
	"sort"
	"strings"
)

type Profile struct {
	Name            string `json:"name"`
	Agent           string `json:"agent"`
	Model           string `json:"model"`
	ReasoningEffort string `json:"reasoning_effort"`
	Prompt          string `json:"prompt"`
	Timeout         string `json:"timeout"`
}

var builtinProfiles = map[string]Profile{
	"standard": {
		Name:            "standard",
		Agent:           "code_reviewer",
		Model:           "gpt-5.5",
		ReasoningEffort: "high",
		Prompt:          "review-branch",
		Timeout:         "30m",
	},
	"pr-readiness": {
		Name:            "pr-readiness",
		Agent:           "code_reviewer",
		Model:           "gpt-5.5",
		ReasoningEffort: "high",
		Prompt:          "review-pr-readiness",
		Timeout:         "30m",
	},
	"strict": {
		Name:            "strict",
		Agent:           "code_reviewer",
		Model:           "gpt-5.5",
		ReasoningEffort: "high",
		Prompt:          "review-branch-strict",
		Timeout:         "45m",
	},
	"repo-policy": {
		Name:            "repo-policy",
		Agent:           "code_reviewer",
		Model:           "gpt-5.5",
		ReasoningEffort: "high",
		Prompt:          "review-repo-policy",
		Timeout:         "30m",
	},
	"deep": {
		Name:            "deep",
		Agent:           "code_reviewer",
		Model:           "gpt-5.5",
		ReasoningEffort: "high",
		Prompt:          "review-branch-deep",
		Timeout:         "60m",
	},
	"security": {
		Name:            "security",
		Agent:           "security_reviewer",
		Model:           "gpt-5.5",
		ReasoningEffort: "high",
		Prompt:          "review-security",
		Timeout:         "45m",
	},
	"fast": {
		Name:            "fast",
		Agent:           "code_reviewer_fast",
		Model:           "gpt-5.5",
		ReasoningEffort: "medium",
		Prompt:          "review-branch-fast",
		Timeout:         "15m",
	},
	"docs": {
		Name:            "docs",
		Agent:           "docs_reviewer",
		Model:           "gpt-5.5",
		ReasoningEffort: "medium",
		Prompt:          "review-docs",
		Timeout:         "20m",
	},
}

func ResolveProfile(name string) (Profile, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "standard"
	}
	profile, ok := builtinProfiles[name]
	if !ok {
		return Profile{}, fmt.Errorf("unknown review profile %q; available profiles: %s", name, strings.Join(ProfileNames(), ", "))
	}
	return profile, nil
}

func ProfileNames() []string {
	names := make([]string, 0, len(builtinProfiles))
	for name := range builtinProfiles {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
