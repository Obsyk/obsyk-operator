# Copyright (c) Obsyk. All rights reserved.
# Licensed under the Apache License, Version 2.0.

# Build stage
# Pinned digest for supply chain security (update periodically)
FROM golang:1.25-alpine@sha256:26111811bc967321e7b6f852e914d14bede324cd1accb7f81811929a6a57fea9 AS builder
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
FROM gcr.io/distroless/static:nonroot@sha256:2b7c93f6d6648c11f0e80a48558c8f77885eb0445213b8e69a6a0d7c89fc6ae4
WORKDIR /
COPY --from=builder /workspace/manager .
USER 65532:65532

ENTRYPOINT ["/manager"]
