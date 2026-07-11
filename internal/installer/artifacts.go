package installer

import "embed"

//go:embed artifacts/* artifacts/**/*
var artifactFS embed.FS
