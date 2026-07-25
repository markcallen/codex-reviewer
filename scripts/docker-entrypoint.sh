#!/usr/bin/env sh
set -eu

export CODEX_HOME="${CODEX_HOME:-/tmp/codex-home}"

if [ ! -d "$CODEX_HOME" ]; then
  mkdir -p "$CODEX_HOME"
fi

if [ -d /opt/codex-reviewer/codex ]; then
  mkdir -p "$CODEX_HOME/agents"
  if [ ! -f "$CODEX_HOME/config.toml" ]; then
    cp /opt/codex-reviewer/codex/config.toml "$CODEX_HOME/config.toml"
  fi
  if [ -d /opt/codex-reviewer/codex/agents ]; then
    cp -n /opt/codex-reviewer/codex/agents/* "$CODEX_HOME/agents/" 2>/dev/null || true
  fi
fi

if [ "$#" -eq 0 ]; then
  set -- codex-reviewer
fi

exec "$@"
