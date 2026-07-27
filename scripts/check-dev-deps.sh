#!/usr/bin/env sh
set -eu

GO_MIN_VERSION="${GO_MIN_VERSION:-1.25}"
KIND_VERSION="${KIND_VERSION:-v0.30.0}"
KUBECTL_VERSION="${KUBECTL_VERSION:-stable}"
KIND_MIN_VERSION="${KIND_VERSION#v}"

failed=0

version_ge() {
  got="$1"
  want="$2"
  awk -v got="$got" -v want="$want" '
    BEGIN {
      split(got, g, ".")
      split(want, w, ".")
      for (i = 1; i <= 3; i++) {
        gv = g[i] + 0
        wv = w[i] + 0
        if (gv > wv) exit 0
        if (gv < wv) exit 1
      }
      exit 0
    }
  '
}

print_check() {
  name="$1"
  found="$2"
  required="$3"
  status="$4"
  printf '%-10s required=%-18s found=%-24s %s\n' "$name" "$required" "$found" "$status"
}

missing() {
  print_check "$1" "missing" "$2" "FAIL"
  failed=1
}

check_go() {
  if ! command -v go >/dev/null 2>&1; then
    missing go "${GO_MIN_VERSION}+"
    return
  fi
  raw="$(go version | awk '{print $3}')"
  found="${raw#go}"
  if version_ge "$found" "$GO_MIN_VERSION"; then
    print_check go "$found" "${GO_MIN_VERSION}+" OK
  else
    print_check go "$found" "${GO_MIN_VERSION}+" FAIL
    failed=1
  fi
}

check_gofmt() {
  if ! command -v gofmt >/dev/null 2>&1; then
    missing gofmt "bundled with Go"
    return
  fi
  print_check gofmt "$(command -v gofmt)" "bundled with Go" OK
}

check_golangci_lint() {
  if ! command -v golangci-lint >/dev/null 2>&1; then
    missing golangci-lint installed
    return
  fi
  found="$(golangci-lint version | awk '{print $4}')"
  print_check golangci-lint "$found" installed OK
}

check_actionlint() {
  if ! command -v actionlint >/dev/null 2>&1; then
    missing actionlint installed
    return
  fi
  found="$(actionlint -version 2>/dev/null | head -1)"
  print_check actionlint "$found" installed OK
}

check_docker() {
  if ! command -v docker >/dev/null 2>&1; then
    missing docker installed
    return
  fi
  found="$(docker --version | sed 's/^Docker version //; s/,.*//')"
  print_check docker "$found" installed OK
}

check_kind() {
  if ! command -v kind >/dev/null 2>&1; then
    missing kind "$KIND_VERSION"
    return
  fi
  found="$(kind version | awk '{print $2}')"
  found_num="${found#v}"
  if version_ge "$found_num" "$KIND_MIN_VERSION"; then
    print_check kind "$found" "${KIND_VERSION}+" OK
  else
    print_check kind "$found" "${KIND_VERSION}+" FAIL
    failed=1
  fi
}

check_kubectl() {
  if ! command -v kubectl >/dev/null 2>&1; then
    missing kubectl "$KUBECTL_VERSION"
    return
  fi
  found="$(kubectl version --client=true --output=yaml 2>/dev/null | awk '/gitVersion:/ {print $2; exit}')"
  if [ -z "$found" ]; then
    found="$(kubectl version --client=true 2>/dev/null | head -1)"
  fi
  print_check kubectl "$found" "$KUBECTL_VERSION" OK
}

check_gh() {
  if ! command -v gh >/dev/null 2>&1; then
    missing gh installed
    return
  fi
  found="$(gh --version | head -1 | awk '{print $3}')"
  print_check gh "$found" installed OK
}

check_go
check_gofmt
check_golangci_lint
check_actionlint
check_docker
check_kind
check_kubectl
check_gh

exit "$failed"
