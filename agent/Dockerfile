# syntax=docker/dockerfile:1.7

# Stage 1: Build Go binary with native systemd journal support
FROM golang:1.26-bookworm AS go-builder
WORKDIR /app

ARG VERSION=dev
ARG COMMIT=unknown

# hadolint ignore=DL3008
# NOTE: Packages are intentionally unpinned for flexibility with Debian updates.
# For reproducible builds, switch to pinning versions or a package lockfile and update this comment.
RUN --mount=type=cache,target=/var/cache/apt,sharing=locked \
    --mount=type=cache,target=/var/lib/apt,sharing=locked \
    apt-get update && \
    apt-get install -y --no-install-recommends build-essential pkg-config libsystemd-dev

COPY go.work go.work.sum ./
COPY server/go.mod server/go.sum ./server/
COPY shared/go.mod* ./shared/
COPY agent/go.mod agent/go.sum ./agent/
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY shared/ ./shared/
COPY agent/ ./agent/

RUN --mount=type=cache,target=/root/.cache/go-build \
    cd agent && \
    CGO_ENABLED=1 go build -tags systemd \
    -ldflags="-s -w -X main.Version=${VERSION} -X main.Commit=${COMMIT}" \
    -o /blackbox-agent .

# Stage 2: Runtime image with libsystemd available for journal access
FROM debian:bookworm-slim
# hadolint ignore=DL3008
# NOTE: Packages are intentionally unpinned for flexibility with Debian updates.
RUN apt-get update && \
    apt-get install -y --no-install-recommends ca-certificates libsystemd0 util-linux && \
    rm -rf /var/lib/apt/lists/* && \
    useradd --no-create-home --shell /bin/false --uid 65532 nonroot

COPY --from=go-builder /blackbox-agent /blackbox-agent
COPY --chmod=755 agent/entrypoint.sh /entrypoint.sh
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 CMD ["/bin/sh", "-c", "kill -0 1"]
ENTRYPOINT ["/entrypoint.sh"]
