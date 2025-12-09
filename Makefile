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
CONTROLLER_GEN_VERSION ?= v0.14.0
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

##@ CI/CD

.PHONY: ci
ci: lint test build ## Run all CI checks (lint, test, build)

.PHONY: verify
verify: fmt ## Verify code formatting (for CI)
	@if [ -n "$$(git status --porcelain)" ]; then \
		echo "Code is not formatted. Run 'make fmt' and commit the changes."; \
		git diff; \
		exit 1; \
	fi

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

##@ Help

.PHONY: help
help: ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)
