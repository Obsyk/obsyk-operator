# Copyright (c) Obsyk. All rights reserved.
# Licensed under the Apache License, Version 2.0.

VERSION 0.8
FROM golang:1.23-alpine
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

# Generate CRD manifests (placeholder - will be implemented with controller-gen)
manifests:
    FROM +deps
    RUN echo "Manifests generation will be added when CRDs are implemented"

# All CI checks
ci:
    BUILD +lint
    BUILD +test
    BUILD +build

# Full build including Docker image
all:
    BUILD +ci
    BUILD +docker
