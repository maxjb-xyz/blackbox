---
title: Server Environment
---

# Server Environment

These environment variables configure the Blackbox server.

| Variable | Required | Default | Description |
| --- | --- | --- | --- |
| `JWT_SECRET` | Yes | None | Secret used to sign JWT session tokens. Use a long random value. |
| `AGENT_TOKENS` | Yes | None | Comma-separated or newline-separated `node=token` pairs. |
| `WEBHOOK_SECRET` | Yes | None | Shared secret for validating webhook requests. |
| `DB_PATH` | No | `/data/blackbox.db` | SQLite database path. |
| `LISTEN_ADDR` | No | `:8080` | TCP bind address for the server. |
| `JWT_TTL` | No | `24h` | Session lifetime using Go duration syntax. |
| `TZ` | No | Container default | IANA timezone for process logs. |
| `TRUSTED_PROXY_IP` | No | None | Separate-host proxy IP trusted for forwarded client IPs. |

## Minimum Required Set

At minimum, most deployments need:

- `JWT_SECRET`
- `AGENT_TOKENS`
- `WEBHOOK_SECRET`

## Related Docs

- [Agent Tokens](./agent-tokens.md)
- [Reverse Proxy And TLS](../deployment/reverse-proxy-and-tls.md)
