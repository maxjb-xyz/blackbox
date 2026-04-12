# syntax=docker/dockerfile:1.7

# ─── Stage 1: Build React ────────────────────────────────────────────────────
FROM --platform=$BUILDPLATFORM node:22-alpine AS web-builder
WORKDIR /web
COPY web/package*.json web/.npmrc ./
RUN --mount=type=cache,target=/root/.npm npm ci
COPY web/ ./
RUN npm run build

# ─── Stage 2: Build Go binary ────────────────────────────────────────────────
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS go-builder
WORKDIR /app

ARG VERSION=dev
ARG COMMIT=unknown
ARG TARGETOS=linux
ARG TARGETARCH=amd64

COPY go.work go.work.sum ./
COPY agent/go.mod agent/go.sum ./agent/
COPY shared/go.mod* ./shared/
COPY server/go.mod server/go.sum ./server/
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY shared/ ./shared/
COPY server/ ./server/
COPY --from=web-builder /web/dist ./server/web/dist

RUN --mount=type=cache,target=/root/.cache/go-build cd server && \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
    -ldflags="-s -w -X main.Version=${VERSION} -X main.Commit=${COMMIT}" \
    -o /blackbox-server .

# Prepare the persistent data directory with the same UID/GID as the
# distroless nonroot user so fresh named volumes inherit writable ownership.
RUN mkdir -p /data && \
    chown 65532:65532 /data && \
    chmod 0750 /data

# ─── Stage 3: Minimal runtime image ──────────────────────────────────────────
FROM gcr.io/distroless/static:nonroot
COPY --from=go-builder /blackbox-server /blackbox-server
COPY --from=go-builder --chown=65532:65532 /data /data
VOLUME /data
EXPOSE 8080
ENTRYPOINT ["/blackbox-server"]
