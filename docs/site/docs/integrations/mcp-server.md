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
3. Choose the port
4. Copy the generated token

## Claude Desktop Example

```json
{
  "mcpServers": {
    "blackbox": {
      "url": "http://your-server:3001/sse",
      "headers": {
        "Authorization": "Bearer <your-token>"
      }
    }
  }
}
```

## Security

- Every request requires a bearer token.
- The full token is not shown again after generation.
- The MCP port is separate from the main app port.
