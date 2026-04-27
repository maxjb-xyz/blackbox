---
title: Quick Start
---

# Quick Start

This gets a single-node Blackbox setup running in minutes.

For multi-node layouts, see [Multi-Node Deployment](../deployment/multi-node.md).

Both images run as non-root:

- The server uses UID `65532`.
- The agent defaults to UID/GID `65532`.
- The agent entrypoint auto-detects the GIDs of mounted resources like the
  Docker socket and systemd journal at startup.
- You can override `PUID` and `PGID` when watched host paths are owned by a
  different user.

## 1. Create A Compose File

```yaml
services:
  blackbox-server:
    image: ghcr.io/maxjb-xyz/blackbox-server:latest
    container_name: blackbox-server
    restart: unless-stopped
    ports:
      - "8080:8080"
    volumes:
      - blackbox-data:/data
    environment:
      JWT_SECRET: "change-me-to-a-long-random-string"
      AGENT_TOKENS: "my-homelab=change-me-to-a-secret-agent-token"
      WEBHOOK_SECRET: "change-me-to-a-webhook-secret"
      TZ: "America/New_York"
    networks:
      - blackbox

  blackbox-agent:
    image: ghcr.io/maxjb-xyz/blackbox-agent:latest
    container_name: blackbox-agent
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
      - /etc:/watch/etc:ro
      - /run/log/journal:/run/log/journal:ro
      - /var/log/journal:/var/log/journal:ro
      - /etc/machine-id:/etc/machine-id:ro
    environment:
      SERVER_URL: "http://blackbox-server:8080"
      AGENT_TOKEN: "change-me-to-a-secret-agent-token"
      NODE_NAME: "my-homelab"
      WATCH_PATHS: "/watch/etc"
      WATCH_SYSTEMD: "true"
      TZ: "America/New_York"
    networks:
      - blackbox

volumes:
  blackbox-agent-data:
  blackbox-data:

networks:
  blackbox:
    driver: bridge
```

## 2. Start The Stack

```bash
docker compose up -d
```

## 3. Finish Setup In The UI

Open `http://your-server:8080` and complete the setup wizard.

## 4. Optional First Tweaks

- Open **Admin > Data Sources** to review per-node and server-wide sources.
- Open **Admin > System** to configure file diff redaction and AI analysis.
- If you only care about Docker at first, leave the file watcher and systemd
  settings alone until the node shows up in the UI.

## What You Should See

- A server UI that loads successfully.
- One registered node with a recent heartbeat.
- Docker events appearing in the timeline once containers change state.
- Optional file and systemd events if those capabilities are configured.
