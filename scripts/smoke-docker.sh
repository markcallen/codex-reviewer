#!/usr/bin/env sh
set -eu

if docker compose version >/dev/null 2>&1; then
  docker compose build reviewer
  output="$(docker compose run --rm reviewer)"
else
  docker build -f Dockerfile.runner -t codex-reviewer:smoke .
  output="$(docker run --rm codex-reviewer:smoke codex-reviewer version)"
fi

if [ -z "$output" ]; then
  echo "FAIL: codex-reviewer version produced no output" >&2
  exit 1
fi

printf 'PASS: codex-reviewer version output: %s\n' "$output"
