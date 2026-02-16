# Copyright (c) Obsyk. All rights reserved.
# Licensed under the Apache License, Version 2.0.

# Build stage
# Pinned to manifest list digest for supply chain security (supports all platforms)
# To update: docker buildx imagetools inspect golang:1.25-alpine
FROM --platform=$BUILDPLATFORM golang:1.25-alpine@sha256:f6751d823c26342f9506c03797d2527668d095b0a15f1862cddb4d927a7a4ced AS builder

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
FROM gcr.io/distroless/static:nonroot@sha256:01e550fdb7ab79ee7be5ff440a563a58f1fd000ad9e0c532e65c3d23f917f1c5
WORKDIR /
COPY --from=builder /workspace/manager .
USER 65532:65532

ENTRYPOINT ["/manager"]
