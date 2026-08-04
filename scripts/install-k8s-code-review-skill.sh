#!/usr/bin/env sh
set -eu

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
SOURCE_DIR="$ROOT_DIR/skills/k8s-code-review"
TARGET="${1:-both}"

install_skill() {
  dest_root="$1"
  dest="$dest_root/k8s-code-review"
  mkdir -p "$dest_root"
  rm -rf "$dest"
  cp -R "$SOURCE_DIR" "$dest"
  printf 'installed %s\n' "$dest"
}

case "$TARGET" in
  codex)
    CODEX_HOME="${CODEX_HOME:-$HOME/.codex}"
    install_skill "$CODEX_HOME/skills"
    ;;
  claude)
    CLAUDE_SKILLS_HOME="${CLAUDE_SKILLS_HOME:-$HOME/.claude/skills}"
    install_skill "$CLAUDE_SKILLS_HOME"
    ;;
  both)
    CODEX_HOME="${CODEX_HOME:-$HOME/.codex}"
    CLAUDE_SKILLS_HOME="${CLAUDE_SKILLS_HOME:-$HOME/.claude/skills}"
    install_skill "$CODEX_HOME/skills"
    install_skill "$CLAUDE_SKILLS_HOME"
    ;;
  *)
    printf 'usage: %s [codex|claude|both]\n' "$0" >&2
    exit 2
    ;;
esac
