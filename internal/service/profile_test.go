package service

import (
	"strings"
	"testing"
)

func TestResolveProfileDefaultsToStandard(t *testing.T) {
	profile, err := ResolveProfile("")
	if err != nil {
		t.Fatalf("ResolveProfile() error = %v", err)
	}
	if profile.Name != "standard" {
		t.Fatalf("profile.Name = %q, want standard", profile.Name)
	}
	if profile.Agent == "" || profile.Model == "" || profile.Prompt == "" {
		t.Fatalf("profile is incomplete: %#v", profile)
	}
}

func TestResolveProfileSupportsDifferentAgents(t *testing.T) {
	standard, err := ResolveProfile("standard")
	if err != nil {
		t.Fatalf("ResolveProfile(standard) error = %v", err)
	}
	security, err := ResolveProfile("security")
	if err != nil {
		t.Fatalf("ResolveProfile(security) error = %v", err)
	}
	if standard.Agent == security.Agent {
		t.Fatalf("standard and security use the same agent %q", standard.Agent)
	}
}

func TestResolveProfileRejectsUnknownProfile(t *testing.T) {
	_, err := ResolveProfile("missing")
	if err == nil {
		t.Fatal("ResolveProfile() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "available profiles") {
		t.Fatalf("ResolveProfile() error = %v, want available profiles", err)
	}
}

func TestProfileNamesSorted(t *testing.T) {
	names := ProfileNames()
	if len(names) == 0 {
		t.Fatal("ProfileNames() returned no names")
	}
	for i := 1; i < len(names); i++ {
		if names[i-1] > names[i] {
			t.Fatalf("ProfileNames() not sorted: %v", names)
		}
	}
}
