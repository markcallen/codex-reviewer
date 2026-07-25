BINARY := bin/codex-reviewer
PKG := ./cmd/codex-reviewer
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X main.version=$(VERSION)

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

.PHONY: help build test test-e2e deps check-deps setup-e2e kind-create kind-namespace kind-service-account kind-secrets docker-build-runner kind-load-runner kind-load-sidecar kind-load-images e2e clean clean-kind

help:
	@printf '%s\n' \
		'Targets:' \
		'  make build              Build bin/codex-reviewer' \
		'  make test               Run unit tests' \
		'  make test-e2e           Compile e2e tests; skips unless RUN_KIND_E2E=1' \
		'  make deps               Download Go modules' \
		'  make check-deps         Verify local tools for kind e2e' \
		'  make setup-e2e          Prepare kind cluster, namespace, images, and secrets' \
		'  make e2e                Run the kind e2e review test' \
		'  make clean              Remove local build output' \
		'  make clean-kind         Delete the kind cluster' \
		'' \
		'Common variables:' \
		'  KIND_CLUSTER=$(KIND_CLUSTER)' \
		'  NAMESPACE=$(NAMESPACE)' \
		'  RUNNER_IMAGE=$(RUNNER_IMAGE)' \
		'  SIDECAR_IMAGE=$(SIDECAR_IMAGE)' \
		'  OPENAI_SECRET=$(OPENAI_SECRET)' \
		'  GITHUB_SECRET=$(GITHUB_SECRET)'

build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) $(PKG)

test:
	go test ./...

test-e2e:
	go test -tags=e2e ./e2e

deps:
	go mod download

check-deps:
	@command -v go >/dev/null || { echo 'missing dependency: go'; exit 1; }
	@command -v docker >/dev/null || { echo 'missing dependency: docker'; exit 1; }
	@command -v kind >/dev/null || { echo 'missing dependency: kind'; exit 1; }
	@command -v kubectl >/dev/null || { echo 'missing dependency: kubectl'; exit 1; }
	@command -v gh >/dev/null || { echo 'missing dependency: gh'; exit 1; }
	@gh auth status >/dev/null
	@docker image inspect "$(SIDECAR_IMAGE)" >/dev/null || { echo 'missing sidecar image: $(SIDECAR_IMAGE)'; exit 1; }

setup-e2e: check-deps deps kind-service-account kind-load-images kind-secrets

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

kind-load-runner: kind-create docker-build-runner
	kind load docker-image "$(RUNNER_IMAGE)" --name "$(KIND_CLUSTER)"

kind-load-sidecar: kind-create check-deps
	kind load docker-image "$(SIDECAR_IMAGE)" --name "$(KIND_CLUSTER)"

kind-load-images: kind-load-runner kind-load-sidecar

e2e:
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
