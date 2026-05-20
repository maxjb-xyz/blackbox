---
title: MCP Server
---

# MCP Server

Blackbox includes an optional MCP server for AI assistants.

## What It Exposes

When enabled, the MCP server exposes tools including:

- `list_incidents`
- `get_incident`
- `list_entries`
- `search_entries`
- `list_nodes`

## Enable It

1. Open **Admin > System > MCP Server**
2. Enable the service
3. Copy or regenerate the server-wide bearer token

## Claude Desktop Example

```json
{
  "mcpServers": {
    "blackbox": {
      "url": "http://your-server/mcp",
      "headers": {
        "Authorization": "Bearer <your-token>"
      }
    }
  }
}
```

## Endpoint

Blackbox mounts MCP on the main server at `/mcp`.

- No separate MCP listener or port is required.
- Reverse proxies should forward `/mcp` to the same Blackbox server as the UI
  and API.

## Breaking Change

Older MCP setups that pointed clients at a dedicated port or `/sse` endpoint
must be updated to `/mcp` on the main Blackbox server.

## Security

- Every request requires a bearer token.
- The token is server-wide and can be regenerated from **Admin > System > MCP
  Server**.
- You can enable or disable MCP from **Admin > System > MCP Server**.
