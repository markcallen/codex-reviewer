package versionutil

import "testing"

func TestReleaseTag(t *testing.T) {
	tests := map[string]string{
		"":                             "dev",
		"dev":                          "dev",
		"abc1234":                      "abc1234",
		"v0.1.2":                       "v0.1.2",
		"v0.1.2-dirty":                 "v0.1.2",
		"v0.1.2-8-g7511702":            "v0.1.2",
		"v0.1.2-8-g7511702-dirty":      "v0.1.2",
		"v0.1.2-rc.1":                  "v0.1.2-rc.1",
		"v0.1.2-rc.1-3-g7511702-dirty": "v0.1.2-rc.1",
	}
	for input, want := range tests {
		if got := ReleaseTag(input); got != want {
			t.Fatalf("ReleaseTag(%q) = %q, want %q", input, got, want)
		}
	}
}
