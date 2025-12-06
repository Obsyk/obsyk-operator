# Copyright (c) Obsyk. All rights reserved.
# Licensed under the Apache License, Version 2.0.

# Build stage
# Pinned digest for supply chain security (update periodically)
# To update: docker manifest inspect golang:1.23-alpine | jq '.manifests[] | select(.platform.architecture=="amd64")'
FROM golang:1.23-alpine@sha256:a7ecaac5efda22510d8c903bdc6b19026543f1eac3317d47363680df22161bd8 AS builder
ARG TARGETOS=linux
ARG TARGETARCH=amd64

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

# Build
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -a -o manager ./cmd/main.go

# Runtime stage
# Pinned digest for supply chain security (update periodically)
# To update: docker manifest inspect gcr.io/distroless/static:nonroot | jq '.manifests[] | select(.platform.architecture=="amd64")'
FROM gcr.io/distroless/static:nonroot@sha256:cc50b1934f8352245c873c23b06fda77935f99e1f94166f366ee7397141d273c
WORKDIR /
COPY --from=builder /workspace/manager .
USER 65532:65532

ENTRYPOINT ["/manager"]
