#!/usr/bin/env sh
set -eu

BIN="${1:-codex-reviewer}"
TMPDIR="${TMPDIR:-/tmp}"
WORKDIR="$(mktemp -d "${TMPDIR%/}/codex-reviewer-smoke.XXXXXX")"
trap 'rm -rf "$WORKDIR"' EXIT

export CODEX_HOME="$WORKDIR/codex-home"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

contains() {
  haystack="$1"
  needle="$2"
  case "$haystack" in
    *"$needle"*) return 0 ;;
    *) fail "output did not contain '$needle'";;
  esac
}

run_capture() {
  "$@" 2>&1
}

run_checked() {
  if ! output="$(run_capture "$@")"; then
    fail "$* failed: $output"
  fi
  printf '%s' "$output"
}

for tool in codex git rg ssh; do
  command -v "$tool" >/dev/null 2>&1 || fail "required tool missing: $tool"
  printf 'PASS: required tool found: %s\n' "$tool"
done

version_output="$(run_checked "$BIN" version)"
[ -n "$version_output" ] || fail "$BIN version produced no output"
printf 'PASS: version output: %s\n' "$version_output"

setup_output="$(run_checked "$BIN" setup --dry-run)"
contains "$setup_output" "Dry run complete."
printf 'PASS: setup dry-run completed\n'

PROJECT="$WORKDIR/project"
mkdir -p "$PROJECT"
(
  cd "$PROJECT"
  git init -b main >/dev/null 2>&1 || git init >/dev/null 2>&1
  printf '%s\n' '# Smoke project' > README.md
)

install_output="$(run_checked "$BIN" install "$PROJECT")"
contains "$install_output" "Install complete."
printf 'PASS: install completed\n'

doctor_output="$(run_checked "$BIN" doctor "$PROJECT")"
contains "$doctor_output" "Codex reviewer setup looks good."
printf 'PASS: doctor completed\n'

local_review_output="$(
  cd "$PROJECT"
  run_checked "$BIN" review local --dry-run --base origin/main --report codex-review/smoke.md
)"
contains "$local_review_output" "codex-reviewer: dry run: codex exec --output-last-message"
contains "$local_review_output" "--output-last-message"
contains "$local_review_output" "Areas checked"
contains "$local_review_output" "Areas not checked / limits"
printf 'PASS: local review dry-run completed\n'

submit_output="$(
  cd "$PROJECT"
  run_checked "$BIN" service submit \
    --dry-run \
    --repo-url https://github.com/octocat/Hello-World \
    --base origin/main \
    --head main \
    --head-sha abc123 \
    --require-clean-tree=false
)"
contains "$submit_output" '"repo_url": "https://github.com/octocat/Hello-World"'
contains "$submit_output" '"profile": "standard"'
printf 'PASS: service submit dry-run completed\n'

manifest_output="$(
  cd "$PROJECT"
  run_checked "$BIN" service job-manifest \
    --repo-url https://github.com/octocat/Hello-World \
    --base origin/main \
    --head main \
    --head-sha abc123 \
    --require-clean-tree=false \
    --review-id smoke \
    --reviewer-image reviewer:test \
    --sidecar-image sidecar:test \
    --openai-secret openai-api
)"
contains "$manifest_output" '"kind": "Job"'
contains "$manifest_output" '"name": "codex-review-smoke"'
contains "$manifest_output" '"image": "reviewer:test"'
printf 'PASS: service job manifest completed\n'

printf 'PASS: smoke checks completed\n'
