---
title: Multi-Node Deployment
---

# Multi-Node Deployment

Multi-node deployment uses one central Blackbox server and one agent on each
machine you want to monitor.

## Deployment Model

- The server runs once and holds the timeline, incidents, users, and settings.
- Each agent points to the same server with a unique `NODE_NAME` and
  `AGENT_TOKEN`.
- Webhooks also point at the same central server.

## Token Planning

The server's `AGENT_TOKENS` environment variable maps node names to secrets.
Each agent must use a matching `NODE_NAME` and `AGENT_TOKEN`.

```bash
AGENT_TOKENS="node-01=token1,node-02=token2,nas=token3"
```

Generate tokens with:

```bash
openssl rand -hex 32
```

## Example: Primary Node

```yaml
services:
  blackbox-server:
    image: ghcr.io/maxjb-xyz/blackbox-server:latest
    restart: unless-stopped
    ports:
      - "8080:8080"
    volumes:
      - blackbox-data:/data
    environment:
      JWT_SECRET: "your-jwt-secret"
      AGENT_TOKENS: "node-01=token-for-node-01,node-02=token-for-node-02,nas=token-for-nas"
      WEBHOOK_SECRET: "your-webhook-secret"
      TZ: "America/New_York"

  blackbox-agent:
    image: ghcr.io/maxjb-xyz/blackbox-agent:latest
    restart: unless-stopped
    cap_drop:
      - ALL
    cap_add:
      - SETUID
      - SETGID
    security_opt:
      - no-new-privileges:true
    read_only: true
    volumes:
      - blackbox-agent-data:/data
      - /var/run/docker.sock:/var/run/docker.sock:ro
      - /run/log/journal:/run/log/journal:ro
      - /var/log/journal:/var/log/journal:ro
      - /etc/machine-id:/etc/machine-id:ro
    environment:
      SERVER_URL: "http://blackbox-server:8080"
      AGENT_TOKEN: "token-for-node-01"
      NODE_NAME: "node-01"
      WATCH_SYSTEMD: "true"
      TZ: "America/New_York"

volumes:
  blackbox-agent-data:
  blackbox-data:
```

## Example: Secondary Node

```yaml
services:
  blackbox-agent:
    image: ghcr.io/maxjb-xyz/blackbox-agent:latest
    restart: unless-stopped
    cap_drop:
      - ALL
    cap_add:
      - SETUID
      - SETGID
    security_opt:
      - no-new-privileges:true
    read_only: true
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
      - /opt/appdata:/watch/appdata:ro
      - /run/log/journal:/run/log/journal:ro
      - /var/log/journal:/var/log/journal:ro
      - /etc/machine-id:/etc/machine-id:ro
      - blackbox-agent-data:/data
    environment:
      SERVER_URL: "http://node-01.lan:8080"
      AGENT_TOKEN: "token-for-node-02"
      NODE_NAME: "node-02"
      WATCH_PATHS: "/watch/appdata"
      WATCH_SYSTEMD: "true"
      TZ: "America/New_York"

volumes:
  blackbox-agent-data:
```

## Recommended Rollout Order

1. Deploy the central server and one local agent.
2. Confirm node registration and timeline flow.
3. Add one remote agent at a time.
4. Review per-node capabilities in **Admin > Data Sources**.
5. Add webhook integrations only after the core node topology is stable.

## Common Mistakes

- Reusing the same token for the wrong `NODE_NAME`.
- Pointing remote agents at an internal URL they cannot resolve.
- Forgetting persistent agent storage, which drops queued events on restart.
- Assuming every node should expose every source capability.
