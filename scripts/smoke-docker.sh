#!/usr/bin/env sh
set -eu

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
IMAGE="${SMOKE_DOCKER_IMAGE:-codex-reviewer:smoke}"

cd "$ROOT_DIR"
docker build -f Dockerfile.runner -t "$IMAGE" .
docker run --rm \
  -v "$ROOT_DIR:/workspace" \
  -w /workspace \
  "$IMAGE" \
  sh scripts/smoke-checks.sh codex-reviewer
