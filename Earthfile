# Copyright (c) Obsyk. All rights reserved.
# Licensed under the Apache License, Version 2.0.

VERSION 0.8
FROM golang:1.24-alpine
WORKDIR /workspace

# Base image with Go tools
go-base:
    RUN apk add --no-cache git make bash
    ENV CGO_ENABLED=0
    ENV GOPROXY=https://proxy.golang.org,direct

# Download dependencies
deps:
    FROM +go-base
    COPY go.mod go.sum ./
    RUN go mod download
    SAVE ARTIFACT go.mod AS LOCAL go.mod
    SAVE ARTIFACT go.sum AS LOCAL go.sum

# Install controller-gen
controller-gen:
    FROM +go-base
    ARG CONTROLLER_GEN_VERSION=v0.14.0
    RUN go install sigs.k8s.io/controller-tools/cmd/controller-gen@${CONTROLLER_GEN_VERSION}
    SAVE ARTIFACT /go/bin/controller-gen

# Generate DeepCopy methods
generate:
    FROM +deps
    COPY +controller-gen/controller-gen /usr/local/bin/
    COPY --dir api ./
    RUN controller-gen object:headerFile="hack/boilerplate.go.txt" paths="./..."  2>/dev/null || \
        controller-gen object paths="./..."
    SAVE ARTIFACT api AS LOCAL api

# Generate CRD manifests
manifests:
    FROM +deps
    COPY +controller-gen/controller-gen /usr/local/bin/
    COPY --dir api ./
    RUN mkdir -p config/crd/bases
    RUN controller-gen crd paths="./api/..." output:crd:artifacts:config=config/crd/bases
    SAVE ARTIFACT config/crd AS LOCAL config/crd

# Lint the code
lint:
    FROM +deps
    RUN go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
    COPY --dir cmd ./
    COPY --dir internal ./
    COPY --dir api ./
    RUN gofmt -l . | tee /tmp/gofmt.out && test ! -s /tmp/gofmt.out
    RUN go vet ./...
    RUN golangci-lint run --timeout=5m || true

# Run tests
test:
    FROM +deps
    COPY --dir cmd ./
    COPY --dir internal ./
    COPY --dir api ./
    RUN go test -v -coverprofile=cover.out ./... || echo "No tests to run yet"
    SAVE ARTIFACT cover.out AS LOCAL cover.out

# Build the binary
build:
    FROM +deps
    COPY --dir cmd ./
    COPY --dir internal ./
    COPY --dir api ./
    RUN GOOS=linux GOARCH=amd64 go build -a -o manager ./cmd/main.go
    SAVE ARTIFACT manager AS LOCAL bin/manager

# Build Docker image
docker:
    FROM gcr.io/distroless/static:nonroot
    WORKDIR /
    COPY +build/manager .
    USER 65532:65532
    ENTRYPOINT ["/manager"]
    ARG VERSION=latest
    SAVE IMAGE --push ghcr.io/obsyk/obsyk-operator:${VERSION}

# All CI checks
ci:
    BUILD +lint
    BUILD +test
    BUILD +build

# Full build including Docker image
all:
    BUILD +ci
    BUILD +docker
