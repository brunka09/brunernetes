# Brunernetes makefile.
#
# The common entry points are:
#   make build      compile brunerctl and the controller manager
#   make test       run every unit test
#   make install    install a single-node factory on this host (needs root)
#   make uninstall  remove it again (needs root)
#   make demo       deploy the Factory Showroom demo application

SHELL := /bin/bash
.DEFAULT_GOAL := help

# Versions used by the build. Keep them pinned: a reproducible build is part of
# the product.
VERSION           ?= 0.1.0
CONTROLLER_GEN_VERSION ?= v0.17.2
ENVTEST_VERSION   ?= release-0.20
ENVTEST_K8S_VERSION ?= 1.32.0
GOLANGCI_LINT_VERSION ?= v1.62.2
KIND_VERSION      ?= v0.26.0

IMAGE_REGISTRY ?= ghcr.io/brunka09
IMAGE_NAME     ?= brunernetes
IMAGE_TAG      ?= $(VERSION)
IMAGE          ?= $(IMAGE_REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG)

DEMO_IMAGE ?= $(IMAGE_REGISTRY)/factory-showroom:$(VERSION)

BIN_DIR   := $(CURDIR)/bin
TOOLS_DIR := $(CURDIR)/hack/tools/bin
GOBIN     := $(TOOLS_DIR)

CONTROLLER_GEN := $(TOOLS_DIR)/controller-gen
ENVTEST        := $(TOOLS_DIR)/setup-envtest
GOLANGCI_LINT  := $(TOOLS_DIR)/golangci-lint

LDFLAGS := -X main.version=$(VERSION)

CHART      := charts/brunernetes
NAMESPACE  ?= brunernetes-system
DEMO_NAMESPACE ?= brunernetes-demo
PRESET     ?= garage
KIND_CLUSTER ?= brunernetes-e2e

##@ General

.PHONY: help
help: ## Print this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nBrunernetes $(VERSION)\n\nUsage:\n  make <target>\n"} \
		/^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2 } \
		/^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) }' $(MAKEFILE_LIST)

##@ Development

.PHONY: generate
generate: controller-gen ## Regenerate the deepcopy functions.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./api/..."

.PHONY: manifests
manifests: controller-gen ## Regenerate the CRDs and the RBAC role.
	$(CONTROLLER_GEN) rbac:roleName=brunernetes-manager crd \
		paths="./api/..." paths="./internal/controller/..." \
		output:crd:artifacts:config=config/crd/bases
	cp config/crd/bases/*.yaml $(CHART)/crds/

.PHONY: fmt
fmt: ## Format the Go sources.
	go fmt ./...

.PHONY: fmt-check
fmt-check: ## Fail when a Go source is not formatted.
	@unformatted=$$(gofmt -l . | grep -v '^vendor/' || true); \
	if [ -n "$$unformatted" ]; then \
		echo "These files are not formatted. Run: make fmt"; \
		echo "$$unformatted"; \
		exit 1; \
	fi
	@echo "Every Go source is formatted."

.PHONY: vet
vet: ## Run go vet.
	go vet ./...

.PHONY: lint
lint: golangci-lint ## Run the Go linter.
	$(GOLANGCI_LINT) run --timeout 5m

.PHONY: tidy
tidy: ## Tidy the module dependencies.
	go mod tidy

##@ Test

.PHONY: test
test: fmt-check vet ## Run the unit tests.
	go test ./... -coverprofile=cover.out

.PHONY: test-envtest
test-envtest: envtest ## Run the controller tests against a real API server.
	KUBEBUILDER_ASSETS="$$($(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(TOOLS_DIR) -p path)" \
		go test ./internal/controller/... -v

.PHONY: coverage
coverage: test ## Print the test coverage per package.
	go tool cover -func=cover.out

##@ Build

.PHONY: build
build: build-cli build-manager ## Build every binary into ./bin.

.PHONY: build-cli
build-cli: ## Build brunerctl.
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/brunerctl ./cmd/brunerctl

.PHONY: build-manager
build-manager: ## Build the controller manager.
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/manager ./cmd/manager

.PHONY: docker-build
docker-build: ## Build the controller image.
	docker build -t $(IMAGE) .

.PHONY: docker-push
docker-push: ## Push the controller image.
	docker push $(IMAGE)

.PHONY: demo-image
demo-image: ## Build the Factory Showroom demo image.
	docker build -t $(DEMO_IMAGE) demo/factory-showroom

.PHONY: clean
clean: ## Remove the build output.
	rm -rf $(BIN_DIR) cover.out

##@ Chart

.PHONY: helm-lint
helm-lint: ## Lint the Helm chart.
	helm lint $(CHART)

.PHONY: helm-template
helm-template: ## Render the chart without installing it.
	helm template brunernetes $(CHART) --namespace $(NAMESPACE)

##@ Cluster

.PHONY: install
install: build-cli ## Install a single-node factory on this host. Needs root.
	sudo $(BIN_DIR)/brunerctl install --preset $(PRESET) --chart $(CHART)

.PHONY: install-dry-run
install-dry-run: build-cli ## Print every change the installation would make.
	$(BIN_DIR)/brunerctl install --preset $(PRESET) --chart $(CHART) --dry-run

.PHONY: uninstall
uninstall: build-cli ## Remove the factory from this host. Needs root.
	sudo $(BIN_DIR)/brunerctl uninstall --yes

.PHONY: status
status: build-cli ## Show the state of the factory.
	$(BIN_DIR)/brunerctl status

.PHONY: demo
demo: build-cli ## Deploy the Factory Showroom demo application.
	kubectl apply -f demo/manifests/namespace.yaml
	$(BIN_DIR)/brunerctl app deploy -f demo/manifests/brunerapp.yaml
	@echo "The demo application is published on http://127.0.0.1:30080/"

.PHONY: demo-clean
demo-clean: ## Remove the demo application.
	kubectl delete -f demo/manifests/brunerapp.yaml --ignore-not-found
	kubectl delete namespace $(DEMO_NAMESPACE) --ignore-not-found

##@ End to end

.PHONY: kind-e2e
kind-e2e: ## Run the kind smoke test. Needs docker, kind and helm.
	KIND_CLUSTER=$(KIND_CLUSTER) IMAGE=$(IMAGE) CHART=$(CHART) ./test/e2e/kind-smoke.sh

##@ Tools

.PHONY: controller-gen
controller-gen: ## Install controller-gen into ./hack/tools/bin.
	@test -x $(CONTROLLER_GEN) || GOBIN=$(GOBIN) go install sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_GEN_VERSION)

.PHONY: envtest
envtest: ## Install setup-envtest and the API server binaries.
	@test -x $(ENVTEST) || GOBIN=$(GOBIN) go install sigs.k8s.io/controller-runtime/tools/setup-envtest@$(ENVTEST_VERSION)
	$(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(TOOLS_DIR)

.PHONY: golangci-lint
golangci-lint: ## Install golangci-lint into ./hack/tools/bin.
	@test -x $(GOLANGCI_LINT) || GOBIN=$(GOBIN) go install github.com/golangci/golangci-lint/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
