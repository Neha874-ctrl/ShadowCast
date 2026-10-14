ARG BASE_IMAGE=golang:1.26
FROM ${BASE_IMAGE} AS builder
ARG TARGETOS
ARG TARGETARCH

WORKDIR /workspace

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Build Controller Manager Binary from cmd/main.go
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -a -o shadowcast-controller ./cmd

# Build Aggregated API Server Binary from cmd/apiserver/main.go
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -a -o shadowcast-api-server ./cmd/apiserver

# Use distroless static base
FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /workspace/shadowcast-controller .
COPY --from=builder /workspace/shadowcast-api-server .
USER 65532:65532