# Build stage using Go 1.23
FROM golang:1.23 AS builder
WORKDIR /workspace

# Allow Go to automatically download required toolchains if go.mod requires a newer version
ENV GOTOOLCHAIN=auto

# Copy go.mod and go.sum
COPY go.mod go.mod
COPY go.sum go.sum
RUN go mod download

# Copy source code
COPY cmd/ cmd/
COPY api/ api/
COPY internal/ internal/

# Build Controller Manager Binary
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -a -o manager ./cmd/main.go

# Final stage
FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /workspace/manager /manager
USER 65532:65532

ENTRYPOINT ["/manager"]