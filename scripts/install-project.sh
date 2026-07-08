#!/usr/bin/env bash
set -euo pipefail

TARGET="${1:-}"

if [[ -z "$TARGET" ]]; then
  echo "Usage: $0 /path/to/target/repo" >&2
  exit 2
fi

if [[ ! -d "$TARGET" ]]; then
  echo "Target directory does not exist: $TARGET" >&2
  exit 2
fi

SOURCE_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

mkdir -p "$TARGET/.codex/agents" "$TARGET/docs" "$TARGET/prompts"

copy_file() {
  local src="$1"
  local dest="$2"
  if [[ -e "$dest" ]]; then
    echo "Keeping existing file: $dest"
    echo "  Compare with: $src"
  else
    cp "$src" "$dest"
    echo "Installed: $dest"
  fi
}

copy_file "$SOURCE_DIR/.codex/config.toml" "$TARGET/.codex/config.toml"
copy_file "$SOURCE_DIR/.codex/agents/code-reviewer.toml" "$TARGET/.codex/agents/code-reviewer.toml"
copy_file "$SOURCE_DIR/AGENTS.md" "$TARGET/AGENTS.md"
copy_file "$SOURCE_DIR/docs/code_review.md" "$TARGET/docs/code_review.md"

for prompt in "$SOURCE_DIR"/prompts/*.md; do
  copy_file "$prompt" "$TARGET/prompts/$(basename "$prompt")"
done

cat <<MSG

Done.

Next steps:
  cd "$TARGET"
  git add .codex AGENTS.md docs/code_review.md prompts/
  git commit -m "Add Codex code reviewer subagent"
  codex

MSG
