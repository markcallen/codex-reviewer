#!/usr/bin/env bash
set -euo pipefail

SOURCE_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DEST_DIR="${CODEX_HOME:-$HOME/.codex}"

mkdir -p "$DEST_DIR/agents"
cp "$SOURCE_DIR/.codex/agents/code-reviewer.toml" "$DEST_DIR/agents/code-reviewer.toml"

cat <<MSG
Installed global reviewer agent:
  $DEST_DIR/agents/code-reviewer.toml

Add this block to:
  $DEST_DIR/config.toml

model = "gpt-5.5"
model_reasoning_effort = "medium"
model_verbosity = "medium"
review_model = "gpt-5.5"
approval_policy = "on-request"

[agents]
max_threads = 4
max_depth = 1

Then run:
  codex

MSG
