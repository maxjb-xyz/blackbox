# syntax=docker/dockerfile:1

# ─── Stage 1: Build React ────────────────────────────────────────────────────
FROM node:22-alpine AS web-builder
WORKDIR /web
COPY web/package*.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# ─── Stage 2: Build Go binary ────────────────────────────────────────────────
FROM golang:1.26-alpine AS go-builder
WORKDIR /app

ARG VERSION=dev
ARG COMMIT=unknown

COPY go.work go.work.sum ./
COPY shared/go.mod shared/go.sum ./shared/
COPY server/go.mod server/go.sum ./server/
RUN cd server && go mod download

COPY shared/ ./shared/
COPY server/ ./server/
COPY --from=web-builder /web/dist ./server/web/dist

RUN CGO_ENABLED=0 cd server && go build     -ldflags="-s -w -X main.Version=${VERSION} -X main.Commit=${COMMIT}"     -o /blackbox-server .

# ─── Stage 3: Minimal runtime image ──────────────────────────────────────────
FROM gcr.io/distroless/static:nonroot
COPY --from=go-builder /blackbox-server /blackbox-server
VOLUME /data
EXPOSE 8080
ENTRYPOINT ["/blackbox-server"]
