# syntax=docker/dockerfile:1

# ─── Stage 1: Build Go binary ────────────────────────────────────────────────
FROM golang:1.26-alpine AS go-builder
WORKDIR /app

ARG VERSION=dev
ARG COMMIT=unknown

COPY go.work ./
COPY shared/go.mod* ./shared/
COPY agent/go.mod agent/go.sum ./agent/
RUN go mod download

COPY shared/ ./shared/
COPY agent/ ./agent/

RUN cd agent && \
    CGO_ENABLED=0 go build \
    -ldflags="-s -w -X main.Version=${VERSION} -X main.Commit=${COMMIT}" \
    -o /blackbox-agent .

# ─── Stage 2: Minimal runtime image ──────────────────────────────────────────
FROM gcr.io/distroless/static:nonroot
COPY --from=go-builder /blackbox-agent /blackbox-agent
ENTRYPOINT ["/blackbox-agent"]
