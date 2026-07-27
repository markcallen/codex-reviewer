#!/usr/bin/env sh
set -eu

GO_MIN_VERSION="${GO_MIN_VERSION:-1.25}"
GO_INSTALL_VERSION="${GO_INSTALL_VERSION:-1.25.0}"
KIND_VERSION="${KIND_VERSION:-v0.30.0}"
KUBECTL_VERSION="${KUBECTL_VERSION:-stable}"
GOLANGCI_LINT_VERSION="${GOLANGCI_LINT_VERSION:-v2.12.2}"
ACTIONLINT_VERSION="${ACTIONLINT_VERSION:-v1.7.12}"
ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
BIN_DIR="${ROOT_DIR}/bin"
TOOLS_DIR="${ROOT_DIR}/.tools"
GO_ROOT="${TOOLS_DIR}/go"

mkdir -p "$BIN_DIR" "$TOOLS_DIR"
PATH="${BIN_DIR}:${GO_ROOT}/bin:${PATH}"
export PATH

have() {
  command -v "$1" >/dev/null 2>&1
}

os_name() {
  case "$(uname -s)" in
    Linux) printf linux ;;
    Darwin) printf darwin ;;
    *) echo "unsupported OS: $(uname -s)" >&2; exit 1 ;;
  esac
}

arch_name() {
  case "$(uname -m)" in
    x86_64|amd64) printf amd64 ;;
    arm64|aarch64) printf arm64 ;;
    *) echo "unsupported architecture: $(uname -m)" >&2; exit 1 ;;
  esac
}

go_satisfies() {
  have go || return 1
  go version | awk -v want="$GO_MIN_VERSION" '
    {
      split($3, v, "go")
      split(v[2], got, ".")
      split(want, req, ".")
      if ((got[1]+0) > (req[1]+0)) exit 0
      if ((got[1]+0) == (req[1]+0) && (got[2]+0) >= (req[2]+0)) exit 0
      exit 1
    }
  '
}

install_go() {
  if go_satisfies; then
    echo "go satisfies ${GO_MIN_VERSION}+"
    return
  fi
  if have brew; then
    brew install go || true
    if go_satisfies; then
      return
    fi
  fi
  if ! have curl || ! have tar; then
    echo "curl and tar are required to install Go locally" >&2
    exit 1
  fi
  os="$(os_name)"
  arch="$(arch_name)"
  archive="go${GO_INSTALL_VERSION}.${os}-${arch}.tar.gz"
  url="https://go.dev/dl/${archive}"
  tmp="${TOOLS_DIR}/${archive}"
  echo "installing Go ${GO_INSTALL_VERSION} into ${GO_ROOT}"
  curl -fsSL "$url" -o "$tmp"
  rm -rf "$GO_ROOT"
  tar -C "$TOOLS_DIR" -xzf "$tmp"
}

install_kind() {
  if have kind; then
    echo "kind already installed"
    return
  fi
  GOBIN="$BIN_DIR" go install "sigs.k8s.io/kind@${KIND_VERSION}"
}

install_kubectl() {
  if have kubectl; then
    echo "kubectl already installed"
    return
  fi
  if ! have curl; then
    echo "curl is required to install kubectl" >&2
    exit 1
  fi
  version="$KUBECTL_VERSION"
  if [ "$version" = "stable" ]; then
    version="$(curl -fsSL https://dl.k8s.io/release/stable.txt)"
  fi
  os="$(os_name)"
  arch="$(arch_name)"
  echo "installing kubectl ${version} into ${BIN_DIR}"
  curl -fsSL "https://dl.k8s.io/release/${version}/bin/${os}/${arch}/kubectl" -o "${BIN_DIR}/kubectl"
  chmod +x "${BIN_DIR}/kubectl"
}

install_golangci_lint() {
  if have golangci-lint; then
    echo "golangci-lint already installed"
    return
  fi
  GOBIN="$BIN_DIR" go install "github.com/golangci/golangci-lint/v2/cmd/golangci-lint@${GOLANGCI_LINT_VERSION}"
}

install_actionlint() {
  if have actionlint; then
    echo "actionlint already installed"
    return
  fi
  GOBIN="$BIN_DIR" go install "github.com/rhysd/actionlint/cmd/actionlint@${ACTIONLINT_VERSION}"
}

install_with_package_manager() {
  tool="$1"
  brew_pkg="$2"
  apt_pkg="$3"
  if have "$tool"; then
    echo "$tool already installed"
    return
  fi
  if have brew; then
    brew install "$brew_pkg" || true
    return
  fi
  if have apt-get && have sudo; then
    sudo apt-get update
    sudo apt-get install -y "$apt_pkg"
    return
  fi
  echo "missing $tool and no supported package manager found" >&2
  exit 1
}

install_go
install_golangci_lint
install_actionlint
install_kind
install_kubectl
install_with_package_manager docker docker docker.io
install_with_package_manager gh gh gh
install_with_package_manager pre-commit pre-commit pre-commit
pre-commit install --install-hooks
pre-commit install --hook-type pre-push --install-hooks
