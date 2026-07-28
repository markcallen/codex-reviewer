#!/usr/bin/env sh
set -eu

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"

cd "$ROOT_DIR"
mkdir -p .cache/go-build .cache/go-mod
export GOCACHE="$ROOT_DIR/.cache/go-build"
export GOMODCACHE="$ROOT_DIR/.cache/go-mod"
go build -ldflags "-X main.version=${VERSION:-dev}" -o bin/codex-reviewer ./cmd/codex-reviewer
sh scripts/smoke-checks.sh "$ROOT_DIR/bin/codex-reviewer"
