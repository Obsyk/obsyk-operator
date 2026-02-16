# Copyright (c) Obsyk. All rights reserved.
# Licensed under the Apache License, Version 2.0.

# Build stage
# Pinned to manifest list digest for supply chain security (supports all platforms)
# To update: docker buildx imagetools inspect golang:1.25-alpine
FROM --platform=$BUILDPLATFORM golang:1.26-alpine@sha256:d4c4845f5d60c6a974c6000ce58ae079328d03ab7f721a0734277e69905473e5 AS builder

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
FROM gcr.io/distroless/static:nonroot@sha256:f9f84bd968430d7d35e8e6d55c40efb0b980829ec42920a49e60e65eac0d83fc
WORKDIR /
COPY --from=builder /workspace/manager .
USER 65532:65532

ENTRYPOINT ["/manager"]
