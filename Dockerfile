# Copyright (c) Obsyk. All rights reserved.
# Licensed under the Apache License, Version 2.0.

# Build stage
# Pinned to manifest list digest for supply chain security (supports all platforms)
# To update: docker buildx imagetools inspect golang:1.25-alpine
FROM --platform=$BUILDPLATFORM golang:1.25-alpine@sha256:ac09a5f469f307e5da71e766b0bd59c9c49ea460a528cc3e6686513d64a6f1fb AS builder

# These are automatically set by docker buildx
ARG TARGETOS
ARG TARGETARCH

# Version is passed at build time, defaults to "dev"
ARG VERSION=dev

WORKDIR /workspace

# Install build dependencies
RUN apk add --no-cache git

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY cmd/ cmd/
COPY internal/ internal/
COPY api/ api/

# Build for target platform with version injected
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -ldflags="-X main.version=${VERSION}" -a -o manager ./cmd/main.go

# Runtime stage
# Pinned to manifest list digest for supply chain security (supports all platforms)
# To update: docker buildx imagetools inspect gcr.io/distroless/static:nonroot
FROM gcr.io/distroless/static:nonroot@sha256:2b7c93f6d6648c11f0e80a48558c8f77885eb0445213b8e69a6a0d7c89fc6ae4
WORKDIR /
COPY --from=builder /workspace/manager .
USER 65532:65532

ENTRYPOINT ["/manager"]
