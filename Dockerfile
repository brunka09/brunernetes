# Build the controller manager.
#
# The image contains one static binary and nothing else: no shell, no package
# manager, no writable root filesystem. The manager needs none of them.
FROM golang:1.24-bookworm AS builder

ARG TARGETOS=linux
ARG TARGETARCH=amd64
ARG VERSION=0.1.0

WORKDIR /workspace

# Download the dependencies first so the layer is reused while the sources
# change.
COPY go.mod go.sum ./
RUN go mod download

COPY cmd/ cmd/
COPY api/ api/
COPY internal/ internal/

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" \
    -o /workspace/manager ./cmd/manager

FROM gcr.io/distroless/static:nonroot

WORKDIR /
COPY --from=builder /workspace/manager /manager

# 65532 is the nonroot user of the distroless base image.
USER 65532:65532

ENTRYPOINT ["/manager"]
