---
title: Security Model
---

# Security Model

Blackbox is designed to keep self-hosted deployments reasonably constrained by
default.

## Container Posture

- The server runs as a non-root distroless image.
- The agent runs as configurable `PUID` and `PGID`.
- The agent drops all capabilities and adds back only `SETUID` and `SETGID`.
- The agent filesystem is read-only except for its data mount.

## Secret Handling

- Shared secrets are compared in constant time.
- Webhook secrets are redacted before config is returned to clients.
- MCP access requires a bearer token.

## Request Controls

- Auth endpoints are rate limited.
- Security headers middleware is enabled.
- Trusted proxy handling is explicit.
