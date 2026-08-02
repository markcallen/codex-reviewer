BINARY := bin/codex-reviewer
PKG := ./cmd/codex-reviewer
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X main.version=$(VERSION)
export PATH := $(CURDIR)/bin:$(CURDIR)/.tools/go/bin:$(PATH)
GO_MIN_VERSION ?= 1.25
GO_INSTALL_VERSION ?= 1.25.0
KIND_VERSION ?= v0.30.0
KUBECTL_VERSION ?= stable
ACTIONLINT_VERSION ?= v1.7.12
COVERAGE_DIR ?= coverage
COVERAGE_PROFILE ?= $(COVERAGE_DIR)/coverage.out
COVERAGE_HTML ?= $(COVERAGE_DIR)/coverage.html
COVERAGE_THRESHOLD ?= 75.0

KIND_CLUSTER ?= codex-reviewer-e2e
KUBE_CONTEXT ?= kind-$(KIND_CLUSTER)
NAMESPACE ?= codex-reviewer-e2e
SERVICE_ACCOUNT ?= codex-reviewer
RUNNER_IMAGE ?= codex-reviewer:phase1
SIDECAR_IMAGE ?= openai-egress:phase1
HELM_RELEASE ?= codex-reviewer
HELM_CHART ?= deploy/helm/codex-reviewer
AUTH_MODE ?= $(if $(CODEX_AUTH),codex,openai)
GHCR_IMAGE ?= ghcr.io/markcallen/codex-reviewer
GHCR_TAG ?= $(VERSION)
GHCR_RUNNER_IMAGE ?= $(GHCR_IMAGE):$(GHCR_TAG)
GHCR_PULL_TAG ?= latest
GHCR_PULL_RUNNER_IMAGE ?= $(GHCR_IMAGE):$(GHCR_PULL_TAG)
REVIEW_ARGS ?= review --base origin/main --output-last-message codex-review/branch-review.md
OPENAI_SECRET ?= openai-api
OPENAI_SECRET_KEY ?= api-key
CODEX_AUTH_SECRET ?= codex-auth
CODEX_AUTH_SECRET_KEY ?= auth.json
GITHUB_SECRET ?= github-token
GITHUB_SECRET_KEY ?= token
E2E_TEST ?= TestKindReviewsSmallAndLargePrivateRepos
E2E_GO_TEST_FLAGS ?= -v
E2E_REPOS ?=
E2E_SMALL_REPO ?= octocat/Hello-World

.PHONY: help setup build test coverage-check coverage-func coverage-html test-e2e lint lint-actions deps deps-tools deps-go-mod check-deps check-e2e-deps setup-e2e k8s-namespace k8s-service-account helm-lint deploy-k8s install-k8s-skill smoke smoke-local smoke-docker smoke-k8s docker-build-runner docker-build-sidecar docker-tag-runner docker-push-runner docker-pull-runner docker-run-ghcr kind-load-runner kind-load-sidecar kind-load-images e2e e2e-small clean clean-kind

help:
	@printf '%s\n' \
		'Targets:' \
		'  make setup              Install deps, verify tools, build, and test' \
		'  make build              Build bin/codex-reviewer' \
		'  make test               Run unit tests with coverage' \
		'  make coverage-check     Fail if total coverage is below threshold' \
		'  make coverage-func      Print function-level coverage' \
		'  make coverage-html      Generate HTML coverage report' \
		'  make test-e2e           Compile e2e tests; skips unless RUN_KIND_E2E=1' \
		'  make lint               Run gofmt, workflow lint, golangci-lint, and go vet' \
		'  make lint-actions       Run GitHub Actions workflow lint' \
		'  make smoke              Run local, Docker, and kind smoke checks' \
		'  make smoke-local        Build local CLI and run smoke checks' \
		'  make smoke-docker       Build container and run smoke checks' \
		'  make smoke-k8s          Load image into kind and run smoke checks' \
		'  make deps               Install dev tools and download Go modules' \
		'  make check-deps         Verify local dev tools and versions' \
		'  make setup-e2e          Prepare kind cluster, namespace, images, and secrets' \
		'  make helm-lint          Lint and render the Kubernetes Helm chart' \
		'  make deploy-k8s         Deploy the review API Helm chart to Kubernetes' \
		'  make install-k8s-skill  Install the Kubernetes review skill for Codex and Claude' \
		'  make docker-build-runner Build the local reviewer container image' \
		'  make docker-tag-runner   Tag the reviewer image for GHCR' \
		'  make docker-push-runner  Push the reviewer image to GHCR' \
		'  make docker-pull-runner  Pull the published reviewer image from GHCR' \
		'  make docker-run-ghcr     Run a local review with the published GHCR image' \
		'  make e2e                Run the kind e2e review test' \
		'  make e2e-small          Run the kind e2e against one small repo' \
		'  make clean              Remove local build output' \
		'  make clean-kind         Delete the kind cluster' \
		'' \
		'Common variables:' \
		'  GO_MIN_VERSION=$(GO_MIN_VERSION)' \
		'  GO_INSTALL_VERSION=$(GO_INSTALL_VERSION)' \
		'  KIND_VERSION=$(KIND_VERSION)' \
		'  KUBECTL_VERSION=$(KUBECTL_VERSION)' \
		'  COVERAGE_PROFILE=$(COVERAGE_PROFILE)' \
		'  COVERAGE_THRESHOLD=$(COVERAGE_THRESHOLD)' \
		'  E2E_REPOS=$(E2E_REPOS)' \
		'  E2E_SMALL_REPO=$(E2E_SMALL_REPO)' \
		'  KIND_CLUSTER=$(KIND_CLUSTER)' \
		'  KUBE_CONTEXT=$(KUBE_CONTEXT)' \
		'  NAMESPACE=$(NAMESPACE)' \
		'  RUNNER_IMAGE=$(RUNNER_IMAGE)' \
		'  SIDECAR_IMAGE=$(SIDECAR_IMAGE)' \
		'  HELM_RELEASE=$(HELM_RELEASE)' \
		'  HELM_CHART=$(HELM_CHART)' \
		'  AUTH_MODE=$(AUTH_MODE)' \
		'  GHCR_RUNNER_IMAGE=$(GHCR_RUNNER_IMAGE)' \
		'  GHCR_PULL_RUNNER_IMAGE=$(GHCR_PULL_RUNNER_IMAGE)' \
		'  REVIEW_ARGS=$(REVIEW_ARGS)' \
		'  OPENAI_SECRET=$(OPENAI_SECRET)' \
		'  CODEX_AUTH_SECRET=$(CODEX_AUTH_SECRET)' \
		'  GITHUB_SECRET=$(GITHUB_SECRET)'

setup: deps check-deps build lint test test-e2e

build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) $(PKG)

test:
	@mkdir -p "$(COVERAGE_DIR)"
	go test -covermode=atomic -coverprofile="$(COVERAGE_PROFILE)" ./...
	@go tool cover -func="$(COVERAGE_PROFILE)" | tail -1

coverage-check: test
	@total="$$(go tool cover -func="$(COVERAGE_PROFILE)" | awk '/^total:/ { sub(/%/,"",$$3); print $$3 }')"; \
	awk -v total="$$total" -v threshold="$(COVERAGE_THRESHOLD)" 'BEGIN { \
		if (total + 0 < threshold + 0) { \
			printf "coverage %.1f%% is below required %.1f%%\n", total, threshold; exit 1 \
		} \
		printf "coverage %.1f%% meets required %.1f%%\n", total, threshold \
	}'

coverage-func: test
	go tool cover -func="$(COVERAGE_PROFILE)"

coverage-html: test
	go tool cover -html="$(COVERAGE_PROFILE)" -o "$(COVERAGE_HTML)"
	@echo "Wrote $(COVERAGE_HTML)"

test-e2e:
	go test -tags=e2e ./e2e

lint: lint-actions
	@test -z "$$(gofmt -l cmd internal e2e 2>/dev/null)" || { echo 'gofmt needed:'; gofmt -l cmd internal e2e; exit 1; }
	golangci-lint run
	go vet ./...

lint-actions:
	@if ! command -v actionlint >/dev/null 2>&1; then \
		mkdir -p bin; \
		GOBIN="$(CURDIR)/bin" go install "github.com/rhysd/actionlint/cmd/actionlint@$(ACTIONLINT_VERSION)"; \
	fi
	actionlint .github/workflows/*.yml

deps: deps-tools deps-go-mod

deps-tools:
	GO_MIN_VERSION="$(GO_MIN_VERSION)" GO_INSTALL_VERSION="$(GO_INSTALL_VERSION)" KIND_VERSION="$(KIND_VERSION)" KUBECTL_VERSION="$(KUBECTL_VERSION)" sh scripts/install-dev-deps.sh

deps-go-mod:
	go mod download

check-deps:
	@GO_MIN_VERSION="$(GO_MIN_VERSION)" KIND_VERSION="$(KIND_VERSION)" KUBECTL_VERSION="$(KUBECTL_VERSION)" sh scripts/check-dev-deps.sh

smoke: smoke-local smoke-docker smoke-k8s

smoke-local:
	sh scripts/smoke-local.sh

smoke-docker:
	sh scripts/smoke-docker.sh

smoke-k8s: kind-load-runner
	sh scripts/smoke-k8s.sh

check-e2e-deps: check-deps
	@if [ -n "$${CODEX_AUTH:-}" ]; then \
		printf '%-10s required=%-18s found=%-24s %s\n' codex-auth set set OK; \
	elif [ -n "$${OPENAI_API_KEY:-}" ]; then \
		printf '%-10s required=%-18s found=%-24s %s\n' openai-key set set OK; \
	else \
		printf '%-10s required=%-18s found=%-24s %s\n' codex-auth-or-openai set missing FAIL; exit 1; \
	fi
	@if [ -n "$${GITHUB_TOKEN:-}" ]; then \
		printf '%-10s required=%-18s found=%-24s %s\n' github-tok set set OK; \
	else \
		printf '%-10s required=%-18s found=%-24s %s\n' github-tok set missing FAIL; exit 1; \
	fi
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
	$(MAKE) k8s-namespace

k8s-namespace:
	kubectl --context "$(KUBE_CONTEXT)" create namespace "$(NAMESPACE)" --dry-run=client -o yaml | kubectl --context "$(KUBE_CONTEXT)" apply -f -

kind-service-account: kind-namespace
	$(MAKE) k8s-service-account

k8s-service-account: k8s-namespace
	kubectl --context "$(KUBE_CONTEXT)" -n "$(NAMESPACE)" create serviceaccount "$(SERVICE_ACCOUNT)" --dry-run=client -o yaml | kubectl --context "$(KUBE_CONTEXT)" apply -f -

kind-secrets: kind-namespace
	@[ -n "$${CODEX_AUTH:-}" ] || [ -n "$${OPENAI_API_KEY:-}" ] || { echo 'CODEX_AUTH or OPENAI_API_KEY is required'; exit 1; }
	@[ -n "$${GITHUB_TOKEN:-}" ] || { echo 'GITHUB_TOKEN is required'; exit 1; }
	@tmpdir="$$(mktemp -d)"; \
		trap 'rm -rf "$$tmpdir"' EXIT; \
		if [ -n "$${CODEX_AUTH:-}" ]; then \
			printf '%s' "$$CODEX_AUTH" > "$$tmpdir/codex-auth"; \
			kubectl --context "$(KUBE_CONTEXT)" -n "$(NAMESPACE)" create secret generic "$(CODEX_AUTH_SECRET)" \
				--from-file="$(CODEX_AUTH_SECRET_KEY)=$$tmpdir/codex-auth" \
				--dry-run=client -o yaml | kubectl --context "$(KUBE_CONTEXT)" apply -f -; \
		else \
			printf '%s' "$$OPENAI_API_KEY" > "$$tmpdir/openai"; \
			kubectl --context "$(KUBE_CONTEXT)" -n "$(NAMESPACE)" create secret generic "$(OPENAI_SECRET)" \
				--from-file="$(OPENAI_SECRET_KEY)=$$tmpdir/openai" \
				--dry-run=client -o yaml | kubectl --context "$(KUBE_CONTEXT)" apply -f -; \
		fi; \
		printf '%s' "$$GITHUB_TOKEN" > "$$tmpdir/github"; \
		kubectl --context "$(KUBE_CONTEXT)" -n "$(NAMESPACE)" create secret generic "$(GITHUB_SECRET)" \
			--from-file="$(GITHUB_SECRET_KEY)=$$tmpdir/github" \
			--dry-run=client -o yaml | kubectl --context "$(KUBE_CONTEXT)" apply -f -

helm-lint:
	helm lint "$(HELM_CHART)"
	helm template "$(HELM_RELEASE)" "$(HELM_CHART)" \
		--namespace "$(NAMESPACE)" \
		--set-string image.fullOverride="$(RUNNER_IMAGE)" \
		--set-string reviewerJob.image.fullOverride="$(RUNNER_IMAGE)" \
		--set-string reviewerJob.sidecarImage.fullOverride="$(SIDECAR_IMAGE)" \
		--set-string serviceAccount.name="$(SERVICE_ACCOUNT)" \
		--set-string auth.mode="$(AUTH_MODE)" \
		--set-string auth.openaiSecret.name="$(OPENAI_SECRET)" \
		--set-string auth.openaiSecret.key="$(OPENAI_SECRET_KEY)" \
		--set-string auth.codexAuthSecret.name="$(CODEX_AUTH_SECRET)" \
		--set-string auth.codexAuthSecret.key="$(CODEX_AUTH_SECRET_KEY)" \
		--set-string github.secret.name="$(GITHUB_SECRET)" \
		--set-string github.secret.key="$(GITHUB_SECRET_KEY)" >/dev/null

deploy-k8s: build
	helm upgrade --install "$(HELM_RELEASE)" "$(HELM_CHART)" \
		--kube-context "$(KUBE_CONTEXT)" \
		--namespace "$(NAMESPACE)" \
		--create-namespace \
		--set-string image.fullOverride="$(RUNNER_IMAGE)" \
		--set-string reviewerJob.image.fullOverride="$(RUNNER_IMAGE)" \
		--set-string reviewerJob.sidecarImage.fullOverride="$(SIDECAR_IMAGE)" \
		--set-string serviceAccount.name="$(SERVICE_ACCOUNT)" \
		--set-string auth.mode="$(AUTH_MODE)" \
		--set-string auth.openaiSecret.name="$(OPENAI_SECRET)" \
		--set-string auth.openaiSecret.key="$(OPENAI_SECRET_KEY)" \
		--set-string auth.codexAuthSecret.name="$(CODEX_AUTH_SECRET)" \
		--set-string auth.codexAuthSecret.key="$(CODEX_AUTH_SECRET_KEY)" \
		--set-string github.secret.name="$(GITHUB_SECRET)" \
		--set-string github.secret.key="$(GITHUB_SECRET_KEY)"

install-k8s-skill:
	scripts/install-k8s-code-review-skill.sh both

docker-build-runner: build
	docker build --build-arg VERSION="$(VERSION)" -f Dockerfile.runner -t "$(RUNNER_IMAGE)" .

docker-build-sidecar:
	docker build -f Dockerfile.egress -t "$(SIDECAR_IMAGE)" .

docker-tag-runner: VERSION := $(GHCR_TAG)
docker-tag-runner: docker-build-runner
	docker tag "$(RUNNER_IMAGE)" "$(GHCR_RUNNER_IMAGE)"

docker-push-runner: docker-tag-runner
	docker push "$(GHCR_RUNNER_IMAGE)"

docker-pull-runner:
	docker pull "$(GHCR_PULL_RUNNER_IMAGE)"

docker-run-ghcr: docker-pull-runner
	docker run --rm \
		--user "$$(id -u):$$(id -g)" \
		-e CODEX_API_KEY \
		-e GITHUB_TOKEN \
		-v "$$PWD:/workspace" \
		-w /workspace \
		"$(GHCR_PULL_RUNNER_IMAGE)" \
		codex exec --sandbox danger-full-access $(REVIEW_ARGS)

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
	CODEX_REVIEWER_CODEX_AUTH_SECRET="$(CODEX_AUTH_SECRET)" \
	CODEX_REVIEWER_GITHUB_SECRET="$(GITHUB_SECRET)" \
	CODEX_REVIEWER_NAMESPACE="$(NAMESPACE)" \
	CODEX_REVIEWER_KIND_CLUSTER="$(KIND_CLUSTER)" \
	CODEX_REVIEWER_KUBE_CONTEXT="$(KUBE_CONTEXT)" \
	CODEX_REVIEWER_SERVICE_ACCOUNT="$(SERVICE_ACCOUNT)" \
	CODEX_REVIEWER_E2E_REPOS="$(E2E_REPOS)" \
	go test $(E2E_GO_TEST_FLAGS) -tags=e2e ./e2e -run "$(E2E_TEST)" -count=1

e2e-small: check-e2e-deps
	RUN_KIND_E2E=1 \
	CODEX_REVIEWER_E2E_SMALL_ONLY=1 \
	CODEX_REVIEWER_REVIEWER_IMAGE="$(RUNNER_IMAGE)" \
	CODEX_REVIEWER_SIDECAR_IMAGE="$(SIDECAR_IMAGE)" \
	CODEX_REVIEWER_OPENAI_SECRET="$(OPENAI_SECRET)" \
	CODEX_REVIEWER_CODEX_AUTH_SECRET="$(CODEX_AUTH_SECRET)" \
	CODEX_REVIEWER_GITHUB_SECRET="$(GITHUB_SECRET)" \
	CODEX_REVIEWER_NAMESPACE="$(NAMESPACE)" \
	CODEX_REVIEWER_KIND_CLUSTER="$(KIND_CLUSTER)" \
	CODEX_REVIEWER_KUBE_CONTEXT="$(KUBE_CONTEXT)" \
	CODEX_REVIEWER_SERVICE_ACCOUNT="$(SERVICE_ACCOUNT)" \
	CODEX_REVIEWER_E2E_REPOS="$(if $(E2E_REPOS),$(E2E_REPOS),$(E2E_SMALL_REPO))" \
	go test $(E2E_GO_TEST_FLAGS) -tags=e2e ./e2e -run "$(E2E_TEST)" -count=1

clean:
	rm -f $(BINARY)
	rm -rf "$(COVERAGE_DIR)"

clean-kind:
	kind delete cluster --name "$(KIND_CLUSTER)"
