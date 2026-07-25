BINARY := bin/codex-reviewer
PKG := ./cmd/codex-reviewer
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X main.version=$(VERSION)
export PATH := $(CURDIR)/bin:$(CURDIR)/.tools/go/bin:$(PATH)
GO_MIN_VERSION ?= 1.25
GO_INSTALL_VERSION ?= 1.25.0
KIND_VERSION ?= v0.30.0
KUBECTL_VERSION ?= stable

KIND_CLUSTER ?= codex-reviewer-e2e
NAMESPACE ?= codex-reviewer-e2e
SERVICE_ACCOUNT ?= codex-reviewer
RUNNER_IMAGE ?= codex-reviewer:phase1
SIDECAR_IMAGE ?= openai-egress:phase1
OPENAI_SECRET ?= openai-api
OPENAI_SECRET_KEY ?= api-key
GITHUB_SECRET ?= github-token
GITHUB_SECRET_KEY ?= token
E2E_TEST ?= TestKindReviewsSmallAndLargePrivateRepos

.PHONY: help setup build test test-e2e lint deps deps-tools deps-go-mod check-deps check-e2e-deps setup-e2e kind-create kind-namespace kind-service-account kind-secrets docker-build-runner docker-build-sidecar kind-load-runner kind-load-sidecar kind-load-images e2e clean clean-kind

help:
	@printf '%s\n' \
		'Targets:' \
		'  make setup              Install deps, verify tools, build, and test' \
		'  make build              Build bin/codex-reviewer' \
		'  make test               Run unit tests' \
		'  make test-e2e           Compile e2e tests; skips unless RUN_KIND_E2E=1' \
		'  make lint               Run gofmt check and go vet' \
		'  make deps               Install dev tools and download Go modules' \
		'  make check-deps         Verify local dev tools and versions' \
		'  make setup-e2e          Prepare kind cluster, namespace, images, and secrets' \
		'  make e2e                Run the kind e2e review test' \
		'  make clean              Remove local build output' \
		'  make clean-kind         Delete the kind cluster' \
		'' \
		'Common variables:' \
		'  GO_MIN_VERSION=$(GO_MIN_VERSION)' \
		'  GO_INSTALL_VERSION=$(GO_INSTALL_VERSION)' \
		'  KIND_VERSION=$(KIND_VERSION)' \
		'  KUBECTL_VERSION=$(KUBECTL_VERSION)' \
		'  KIND_CLUSTER=$(KIND_CLUSTER)' \
		'  NAMESPACE=$(NAMESPACE)' \
		'  RUNNER_IMAGE=$(RUNNER_IMAGE)' \
		'  SIDECAR_IMAGE=$(SIDECAR_IMAGE)' \
		'  OPENAI_SECRET=$(OPENAI_SECRET)' \
		'  GITHUB_SECRET=$(GITHUB_SECRET)'

setup: deps check-deps build lint test test-e2e

build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) $(PKG)

test:
	go test ./...

test-e2e:
	go test -tags=e2e ./e2e

lint:
	@test -z "$$(gofmt -l cmd internal e2e 2>/dev/null)" || { echo 'gofmt needed:'; gofmt -l cmd internal e2e; exit 1; }
	go vet ./...

deps: deps-tools deps-go-mod

deps-tools:
	GO_MIN_VERSION="$(GO_MIN_VERSION)" GO_INSTALL_VERSION="$(GO_INSTALL_VERSION)" KIND_VERSION="$(KIND_VERSION)" KUBECTL_VERSION="$(KUBECTL_VERSION)" sh scripts/install-dev-deps.sh

deps-go-mod:
	go mod download

check-deps:
	@GO_MIN_VERSION="$(GO_MIN_VERSION)" KIND_VERSION="$(KIND_VERSION)" KUBECTL_VERSION="$(KUBECTL_VERSION)" sh scripts/check-dev-deps.sh

check-e2e-deps: check-deps
	@if gh auth status >/dev/null 2>&1; then \
		printf '%-10s required=%-18s found=%-24s %s\n' gh-auth authenticated authenticated OK; \
	else \
		printf '%-10s required=%-18s found=%-24s %s\n' gh-auth authenticated missing FAIL; exit 1; \
	fi
	@if docker image inspect "$(RUNNER_IMAGE)" >/dev/null 2>&1; then \
		printf '%-10s required=%-18s found=%-24s %s\n' runner-img "$(RUNNER_IMAGE)" "$(RUNNER_IMAGE)" OK; \
	else \
		printf '%-10s required=%-18s found=%-24s %s\n' runner-img "$(RUNNER_IMAGE)" missing FAIL; echo 'Run make docker-build-runner'; exit 1; \
	fi
	@if docker image inspect "$(SIDECAR_IMAGE)" >/dev/null 2>&1; then \
		printf '%-10s required=%-18s found=%-24s %s\n' sidecar-img "$(SIDECAR_IMAGE)" "$(SIDECAR_IMAGE)" OK; \
	else \
		printf '%-10s required=%-18s found=%-24s %s\n' sidecar-img "$(SIDECAR_IMAGE)" missing FAIL; echo 'Run make docker-build-sidecar'; exit 1; \
	fi

setup-e2e: check-deps deps-go-mod kind-service-account kind-load-images kind-secrets

kind-create:
	@if kind get clusters | grep -qx "$(KIND_CLUSTER)"; then \
		echo "kind cluster $(KIND_CLUSTER) already exists"; \
	else \
		kind create cluster --name "$(KIND_CLUSTER)"; \
	fi

kind-namespace: kind-create
	kubectl create namespace "$(NAMESPACE)" --dry-run=client -o yaml | kubectl apply -f -

kind-service-account: kind-namespace
	kubectl -n "$(NAMESPACE)" create serviceaccount "$(SERVICE_ACCOUNT)" --dry-run=client -o yaml | kubectl apply -f -

kind-secrets: kind-namespace
	@[ -n "$${OPENAI_API_KEY:-}" ] || { echo 'OPENAI_API_KEY is required'; exit 1; }
	@[ -n "$${GITHUB_TOKEN:-}" ] || { echo 'GITHUB_TOKEN is required'; exit 1; }
	@tmpdir="$$(mktemp -d)"; \
		trap 'rm -rf "$$tmpdir"' EXIT; \
		printf '%s' "$$OPENAI_API_KEY" > "$$tmpdir/openai"; \
		printf '%s' "$$GITHUB_TOKEN" > "$$tmpdir/github"; \
		kubectl -n "$(NAMESPACE)" create secret generic "$(OPENAI_SECRET)" \
			--from-file="$(OPENAI_SECRET_KEY)=$$tmpdir/openai" \
			--dry-run=client -o yaml | kubectl apply -f -; \
		kubectl -n "$(NAMESPACE)" create secret generic "$(GITHUB_SECRET)" \
			--from-file="$(GITHUB_SECRET_KEY)=$$tmpdir/github" \
			--dry-run=client -o yaml | kubectl apply -f -

docker-build-runner: build
	docker build -f Dockerfile.runner -t "$(RUNNER_IMAGE)" .

docker-build-sidecar:
	docker build -f Dockerfile.egress -t "$(SIDECAR_IMAGE)" .

kind-load-runner: kind-create docker-build-runner
	kind load docker-image "$(RUNNER_IMAGE)" --name "$(KIND_CLUSTER)"

kind-load-sidecar: kind-create docker-build-sidecar
	kind load docker-image "$(SIDECAR_IMAGE)" --name "$(KIND_CLUSTER)"

kind-load-images: kind-load-runner kind-load-sidecar

e2e: check-e2e-deps
	RUN_KIND_E2E=1 \
	CODEX_REVIEWER_REVIEWER_IMAGE="$(RUNNER_IMAGE)" \
	CODEX_REVIEWER_SIDECAR_IMAGE="$(SIDECAR_IMAGE)" \
	CODEX_REVIEWER_OPENAI_SECRET="$(OPENAI_SECRET)" \
	CODEX_REVIEWER_GITHUB_SECRET="$(GITHUB_SECRET)" \
	CODEX_REVIEWER_NAMESPACE="$(NAMESPACE)" \
	CODEX_REVIEWER_KIND_CLUSTER="$(KIND_CLUSTER)" \
	CODEX_REVIEWER_SERVICE_ACCOUNT="$(SERVICE_ACCOUNT)" \
	go test -tags=e2e ./e2e -run "$(E2E_TEST)" -count=1

clean:
	rm -f $(BINARY)

clean-kind:
	kind delete cluster --name "$(KIND_CLUSTER)"
