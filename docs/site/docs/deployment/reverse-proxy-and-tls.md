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

## MCP Port

If you enable the MCP server, remember that it runs on a separate port from the
main UI and API. Proxy it separately if your AI client needs external access.
