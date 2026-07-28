package versionutil

import "strings"

// ReleaseTag returns the stable release tag portion of a build version.
// It turns git-describe values like v0.1.2-8-gabcdef-dirty into v0.1.2,
// while preserving dev, raw SHAs, and prerelease tags such as v0.1.2-rc.1.
func ReleaseTag(version string) string {
	version = strings.TrimSpace(version)
	if version == "" {
		return "dev"
	}
	version = strings.TrimSuffix(version, "-dirty")
	parts := strings.Split(version, "-")
	if len(parts) >= 3 && isDecimal(parts[len(parts)-2]) && strings.HasPrefix(parts[len(parts)-1], "g") {
		return strings.Join(parts[:len(parts)-2], "-")
	}
	return version
}

func isDecimal(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
