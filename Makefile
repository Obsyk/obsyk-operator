# Copyright (c) Obsyk. All rights reserved.
# Licensed under the Apache License, Version 2.0.

# Image configuration
REGISTRY ?= ghcr.io
IMAGE_NAME ?= obsyk/obsyk-operator
VERSION ?= latest
IMG ?= $(REGISTRY)/$(IMAGE_NAME):$(VERSION)

# Go configuration
GOOS ?= linux
GOARCH ?= amd64
CGO_ENABLED ?= 0

# Version is set from git tag or defaults to dev
GIT_VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -X main.version=$(GIT_VERSION)

# Tool versions
CONTROLLER_GEN_VERSION ?= v0.19.0
GOLANGCI_LINT_VERSION ?= v1.62.2

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

.PHONY: all
all: ci

##@ Development

.PHONY: fmt
fmt: ## Run go fmt against code
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code
	go vet ./...

.PHONY: lint
lint: fmt vet golangci-lint ## Run all linters
	$(GOLANGCI_LINT) run --timeout=5m

.PHONY: test
test: ## Run tests
	go test -v -race -coverprofile=cover.out ./...

.PHONY: test-short
test-short: ## Run tests (short mode)
	go test -v -short ./...

##@ Build

.PHONY: build
build: ## Build manager binary
	CGO_ENABLED=$(CGO_ENABLED) GOOS=$(GOOS) GOARCH=$(GOARCH) go build -ldflags="$(LDFLAGS)" -a -o bin/manager ./cmd/main.go

.PHONY: run
run: ## Run the controller locally
	go run -ldflags="$(LDFLAGS)" ./cmd/main.go

.PHONY: docker-build
docker-build: ## Build docker image
	docker build --build-arg VERSION=$(GIT_VERSION) -t $(IMG) .

.PHONY: docker-push
docker-push: ## Push docker image
	docker push $(IMG)

.PHONY: docker-buildx
docker-buildx: ## Build and push docker image for cross-platform support
	docker buildx build --build-arg VERSION=$(GIT_VERSION) --platform linux/amd64,linux/arm64 -t $(IMG) --push .

##@ Code Generation

.PHONY: generate
generate: controller-gen ## Generate code (DeepCopy methods)
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..." 2>/dev/null || \
		$(CONTROLLER_GEN) object paths="./..."

.PHONY: manifests
manifests: controller-gen ## Generate CRD manifests
	$(CONTROLLER_GEN) crd paths="./api/..." output:crd:artifacts:config=config/crd/bases
	@cp config/crd/bases/*.yaml charts/obsyk-operator/crds/

##@ CI/CD

.PHONY: ci
ci: lint test build verify-manifests ## Run all CI checks (lint, test, build, verify-manifests)

.PHONY: verify
verify: fmt ## Verify code formatting (for CI)
	@if [ -n "$$(git status --porcelain)" ]; then \
		echo "Code is not formatted. Run 'make fmt' and commit the changes."; \
		git diff; \
		exit 1; \
	fi

.PHONY: verify-manifests
verify-manifests: manifests ## Verify CRD manifests are up to date
	@if [ -n "$$(git diff --name-only config/crd/ charts/obsyk-operator/crds/)" ]; then \
		echo "CRD manifests are out of date. Run 'make manifests' and copy to charts/obsyk-operator/crds/"; \
		git diff config/crd/ charts/obsyk-operator/crds/; \
		exit 1; \
	fi
	@echo "CRD manifests are up to date"

##@ Tools

CONTROLLER_GEN = $(GOBIN)/controller-gen
.PHONY: controller-gen
controller-gen: ## Download controller-gen locally if necessary
	@test -s $(CONTROLLER_GEN) || \
		GOBIN=$(GOBIN) go install sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_GEN_VERSION)

GOLANGCI_LINT = $(GOBIN)/golangci-lint
.PHONY: golangci-lint
golangci-lint: ## Download golangci-lint locally if necessary
	@test -s $(GOLANGCI_LINT) || \
		GOBIN=$(GOBIN) go install github.com/golangci/golangci-lint/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

##@ Multi-Cloud Testing

# Kind node images for K8s version matrix
KIND_IMAGE_1_28 ?= kindest/node:v1.28.15
KIND_IMAGE_1_30 ?= kindest/node:v1.30.8
KIND_IMAGE_1_32 ?= kindest/node:v1.32.2

.PHONY: e2e-kind-matrix
e2e-kind-matrix: ## Run E2E on K8s 1.28, 1.30, 1.32 (local testing)
	@echo "=== Running E2E tests on multiple Kubernetes versions ==="
	@for version in 1.28.15 1.30.8 1.32.2; do \
		echo ""; \
		echo "=== Testing K8s v$$version ==="; \
		kind create cluster --name obsyk-$$version --image kindest/node:v$$version || exit 1; \
		$(MAKE) docker-build IMG=obsyk-operator:e2e; \
		kind load docker-image obsyk-operator:e2e --name obsyk-$$version; \
		helm install obsyk-operator ./charts/obsyk-operator -n obsyk-system --create-namespace \
			--set image.repository=obsyk-operator --set image.tag=e2e --set image.pullPolicy=Never \
			--set agent.enabled=false || (kind delete cluster --name obsyk-$$version && exit 1); \
		kubectl wait --for=condition=Available deployment/obsyk-operator -n obsyk-system --timeout=120s || \
			(kubectl logs -n obsyk-system deployment/obsyk-operator --tail=50; kind delete cluster --name obsyk-$$version; exit 1); \
		echo "K8s v$$version: PASSED"; \
		kind delete cluster --name obsyk-$$version; \
	done
	@echo ""
	@echo "=== All Kubernetes versions passed ==="

.PHONY: e2e-pss
e2e-pss: ## Test with PSS restricted (validates OpenShift/ROSA compatibility)
	@echo "=== Testing with Pod Security Standards: restricted ==="
	-kind delete cluster --name obsyk-pss 2>/dev/null
	kind create cluster --name obsyk-pss
	kubectl create namespace obsyk-system
	kubectl label namespace obsyk-system \
		pod-security.kubernetes.io/enforce=restricted \
		pod-security.kubernetes.io/enforce-version=latest \
		pod-security.kubernetes.io/warn=restricted \
		pod-security.kubernetes.io/warn-version=latest
	$(MAKE) docker-build IMG=obsyk-operator:e2e
	kind load docker-image obsyk-operator:e2e --name obsyk-pss
	helm install obsyk-operator ./charts/obsyk-operator -n obsyk-system \
		--set image.repository=obsyk-operator --set image.tag=e2e --set image.pullPolicy=Never \
		--set agent.enabled=false || (kind delete cluster --name obsyk-pss && exit 1)
	kubectl wait --for=condition=Available deployment/obsyk-operator -n obsyk-system --timeout=120s || \
		(kubectl logs -n obsyk-system deployment/obsyk-operator --tail=50; kind delete cluster --name obsyk-pss; exit 1)
	@echo ""
	@echo "=== PSS Restricted test PASSED ==="
	@echo "Operator runs successfully with Pod Security Standards: restricted"
	@echo "This validates compatibility with OpenShift/ROSA Security Context Constraints"
	kind delete cluster --name obsyk-pss

.PHONY: e2e-kind
e2e-kind: ## Run basic E2E test on local Kind cluster
	@echo "=== Running basic E2E test ==="
	-kind delete cluster --name obsyk-e2e 2>/dev/null
	kind create cluster --name obsyk-e2e
	$(MAKE) docker-build IMG=obsyk-operator:e2e
	kind load docker-image obsyk-operator:e2e --name obsyk-e2e
	helm install obsyk-operator ./charts/obsyk-operator -n obsyk-system --create-namespace \
		--set image.repository=obsyk-operator --set image.tag=e2e --set image.pullPolicy=Never \
		--set agent.enabled=false || (kind delete cluster --name obsyk-e2e && exit 1)
	kubectl wait --for=condition=Available deployment/obsyk-operator -n obsyk-system --timeout=120s || \
		(kubectl logs -n obsyk-system deployment/obsyk-operator --tail=50; kind delete cluster --name obsyk-e2e; exit 1)
	@echo ""
	@echo "=== E2E test PASSED ==="
	kubectl get pods -n obsyk-system
	kind delete cluster --name obsyk-e2e

##@ Help

.PHONY: help
help: ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)
