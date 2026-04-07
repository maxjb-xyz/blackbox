# syntax=docker/dockerfile:1

# Stage 1: Build Go binary with native systemd journal support
FROM golang:1.26-bookworm AS go-builder
WORKDIR /app

ARG VERSION=dev
ARG COMMIT=unknown

RUN apt-get update && \
    apt-get install -y --no-install-recommends build-essential pkg-config libsystemd-dev && \
    rm -rf /var/lib/apt/lists/*

COPY go.work go.work.sum ./
COPY server/go.mod server/go.sum ./server/
COPY shared/go.mod* ./shared/
COPY agent/go.mod agent/go.sum ./agent/
RUN go mod download

COPY shared/ ./shared/
COPY agent/ ./agent/

RUN cd agent && \
    CGO_ENABLED=1 go build -tags systemd \
    -ldflags="-s -w -X main.Version=${VERSION} -X main.Commit=${COMMIT}" \
    -o /blackbox-agent .

# Stage 2: Runtime image with libsystemd available for journal access
FROM debian:bookworm-slim
RUN apt-get update && \
    apt-get install -y --no-install-recommends ca-certificates libsystemd0 && \
    rm -rf /var/lib/apt/lists/* && \
    useradd --no-create-home --shell /bin/false --uid 65532 nonroot

COPY --from=go-builder /blackbox-agent /blackbox-agent
COPY agent/entrypoint.sh /entrypoint.sh
ENTRYPOINT ["/entrypoint.sh"]
