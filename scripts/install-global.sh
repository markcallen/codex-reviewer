#!/usr/bin/env bash
set -euo pipefail

SOURCE_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DEST_DIR="${CODEX_HOME:-$HOME/.codex}"
CONFIG_FILE="$DEST_DIR/config.toml"

mkdir -p "$DEST_DIR/agents"
cp "$SOURCE_DIR/.codex/agents/code-reviewer.toml" "$DEST_DIR/agents/code-reviewer.toml"

top_level_missing() {
  local key
  for key in model model_reasoning_effort model_verbosity review_model approval_policy; do
    if ! awk -v key="$key" '
      /^[[:space:]]*\[/ { exit }
      $0 ~ "^[[:space:]]*" key "[[:space:]]*=" { found = 1; exit }
      END { exit found ? 0 : 1 }
    ' "$CONFIG_FILE" 2>/dev/null; then
      case "$key" in
        model) printf '%s\n' 'model = "gpt-5.5"' ;;
        model_reasoning_effort) printf '%s\n' 'model_reasoning_effort = "medium"' ;;
        model_verbosity) printf '%s\n' 'model_verbosity = "medium"' ;;
        review_model) printf '%s\n' 'review_model = "gpt-5.5"' ;;
        approval_policy) printf '%s\n' 'approval_policy = "on-request"' ;;
      esac
    fi
  done
}

agent_key_missing() {
  local key="$1"
  ! awk -v key="$key" '
    /^[[:space:]]*\[/ {
      in_agents = ($0 ~ /^[[:space:]]*\[agents\][[:space:]]*$/)
      next
    }
    in_agents && $0 ~ "^[[:space:]]*" key "[[:space:]]*=" { found = 1; exit }
    END { exit found ? 0 : 1 }
  ' "$CONFIG_FILE" 2>/dev/null
}

has_agents_section() {
  grep -Eq '^[[:space:]]*\[agents\][[:space:]]*$' "$CONFIG_FILE" 2>/dev/null
}

append_or_merge_config() {
  local top_missing="$1"
  local agents_missing="$2"
  local tmp
  tmp="$(mktemp)"

  if [ ! -s "$CONFIG_FILE" ]; then
    {
      [ -z "$top_missing" ] || printf '%s\n' "$top_missing"
      if [ -n "$agents_missing" ]; then
        printf '\n[agents]\n%s\n' "$agents_missing"
      fi
    } > "$tmp"
    mv "$tmp" "$CONFIG_FILE"
    return
  fi

  if [ -n "$top_missing" ]; then
    awk -v block="$top_missing" '
      BEGIN { inserted = 0 }
      !inserted && /^[[:space:]]*\[/ {
        print block
        print ""
        inserted = 1
      }
      { print }
      END {
        if (!inserted) {
          print ""
          print block
        }
      }
    ' "$CONFIG_FILE" > "$tmp"
    mv "$tmp" "$CONFIG_FILE"
  fi

  if [ -n "$agents_missing" ]; then
    tmp="$(mktemp)"
    if has_agents_section; then
      awk -v block="$agents_missing" '
        BEGIN { inserted = 0; in_agents = 0 }
        /^[[:space:]]*\[/ {
          if (in_agents && !inserted) {
            print block
            inserted = 1
          }
          in_agents = ($0 ~ /^[[:space:]]*\[agents\][[:space:]]*$/)
          print
          next
        }
        { print }
        END {
          if (in_agents && !inserted) {
            print block
          }
        }
      ' "$CONFIG_FILE" > "$tmp"
    else
      {
        cat "$CONFIG_FILE"
        printf '\n[agents]\n%s\n' "$agents_missing"
      } > "$tmp"
    fi
    mv "$tmp" "$CONFIG_FILE"
  fi
}

missing_top="$(top_level_missing)"
missing_agents=""
if agent_key_missing max_threads; then
  missing_agents="${missing_agents}max_threads = 4"$'\n'
fi
if agent_key_missing max_depth; then
  missing_agents="${missing_agents}max_depth = 1"$'\n'
fi
missing_agents="${missing_agents%$'\n'}"

cat <<MSG
Installed global reviewer agent:
  $DEST_DIR/agents/code-reviewer.toml
MSG

if [ -z "$missing_top" ] && [ -z "$missing_agents" ]; then
  cat <<MSG
Reviewer config already appears to be present:
  $CONFIG_FILE

Then run:
  codex

MSG
  exit 0
fi

cat <<MSG
Missing reviewer config in:
  $CONFIG_FILE

The installer can update it with the missing settings:

MSG

[ -z "$missing_top" ] || printf '%s\n' "$missing_top"
if [ -n "$missing_agents" ]; then
  if has_agents_section; then
    printf '\n[agents]\n%s\n' "$missing_agents"
  else
    printf '\n[agents]\n%s\n' "$missing_agents"
  fi
fi

printf '\nUpdate %s now? [y/N] ' "$CONFIG_FILE"
read -r reply
case "$reply" in
  y|Y|yes|YES)
    mkdir -p "$(dirname "$CONFIG_FILE")"
    touch "$CONFIG_FILE"
    append_or_merge_config "$missing_top" "$missing_agents"
    cat <<MSG

Updated:
  $CONFIG_FILE

Then run:
  codex

MSG
    ;;
  *)
    cat <<MSG

Skipped config update. Re-run this script when you are ready to update:
  $CONFIG_FILE

MSG
    ;;
esac
