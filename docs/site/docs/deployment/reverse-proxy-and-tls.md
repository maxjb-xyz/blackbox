---
title: Reverse Proxy And TLS
---

# Reverse Proxy And TLS

Blackbox works well behind a reverse proxy for HTTPS termination and stable
hostnames.

## Nginx

```nginx
server {
    listen 443 ssl;
    server_name blackbox.example.com;

    location / {
        proxy_pass http://localhost:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

## Caddy

```caddyfile
blackbox.example.com {
    reverse_proxy localhost:8080
}
```

## Trusted Proxy IPs

Blackbox trusts `X-Forwarded-For` only from loopback by default. If your proxy
is on another machine, set `TRUSTED_PROXY_IP` on the server container so audit
logs record the real client IP.

## MCP Endpoint

If you enable the MCP server, it is mounted at `/mcp` on the main Blackbox
server.

- No separate MCP listener or port needs to be exposed through your reverse
  proxy.
- Proxy `/mcp` the same way you proxy the rest of the UI and API.
- MCP still requires the server-wide bearer token from **Admin > System > MCP
  Server**.

This is a breaking change for older deployments that previously routed MCP to a
dedicated port or `/sse` path.
