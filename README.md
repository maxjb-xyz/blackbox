<p align="center">
  <img src="docs/assets/logo.png" alt="Blackbox" width="120" />
</p>

<h1 align="center">Blackbox</h1>
<p align="center">
  A self-hosted forensic event timeline for homelabs and home servers.<br/>
  Know <em>what</em> changed, <em>when</em> it changed, and <em>why</em> things broke.
</p>

<p align="center">
  <img src="https://img.shields.io/badge/Go-1.26-00ADD8?style=flat-square&logo=go" />
  <img src="https://img.shields.io/badge/React-19-61DAFB?style=flat-square&logo=react" />
  <img src="https://img.shields.io/badge/SQLite-portable-003B57?style=flat-square&logo=sqlite" />
  <img src="https://img.shields.io/badge/Docker-ready-2496ED?style=flat-square&logo=docker" />
  <img src="https://img.shields.io/badge/license-AGPL_v3-red?style=flat-square" />
</p>

---

## What is Blackbox?

Blackbox is a lightweight, self-hosted event correlation platform built for homelabbers who want to understand their infrastructure at a glance. It collects events from Docker, config file changes, uptime monitors, and container update tools — correlates them into a single chronological timeline — and gives you a dark, dense UI designed for quick diagnosis at 2 AM.

When your homelab breaks, Blackbox tells you what happened. You just write why.

### Core Principles

- **The Forensic Timeline** — Every event (Docker, file change, webhook) is timestamped and correlated. Nothing is lost.
- **The "Discipline" Fix** — Automation handles the *what*. Your notes handle the *why*. No manual logging of events that can be detected automatically.
- **The 2 AM Rule** — High-density, low eye strain (dark mode), zero-friction navigation. Built for stressful troubleshooting.

---

## Quick Start

> This gets a single-node setup running in minutes. For multi-node, see [Multi-Node Deployment](#multi-node-deployment).
> The server image runs as distroless `nonroot` and can write a named volume mounted at `/data` without `user: "0:0"`. The agent example still uses `root` because read-only access to `/var/run/docker.sock` commonly requires it unless you map the host Docker socket group into the container.

**1. Create a `docker-compose.yml`:**

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
    networks:
      - blackbox

  blackbox-agent:
    image: ghcr.io/maxjb-xyz/blackbox-agent:latest
    user: "0:0"
    container_name: blackbox-agent
    restart: unless-stopped
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
      - /etc:/watch/etc:ro
    environment:
      SERVER_URL: "http://blackbox-server:8080"
      AGENT_TOKEN: "change-me-to-a-secret-agent-token"
      NODE_NAME: "my-homelab"
      WATCH_PATHS: "/watch/etc"
    networks:
      - blackbox

volumes:
  blackbox-data:

networks:
  blackbox:
    driver: bridge
```

**2. Start it:**

```bash
docker compose up -d
```

**3. Open `http://your-server:8080` and complete the setup wizard.**

---

## Features

### Event Ingestion
- **Docker events** — Automatic detection of container start, stop, die, create, pull, and delete events. Per-node, with a 3-second debounce to collapse rapid restarts.
- **Config file watching** — inotify-based watching of `.yaml`, `.yml`, `.conf`, `.env`, `.json`, and `.ini` files via configurable `WATCH_PATHS`.
- **Uptime Kuma webhooks** — Ingest Down/Up state changes. Down events trigger automatic correlation: Blackbox queries the 120-second window before the incident for likely causes.
- **Watchtower webhooks** — Ingest container image update events with version metadata.
- **Manual entries** — Post arbitrary events from the UI or via the API.

### Timeline
- Chronological, paginated event feed across all nodes
- Filter by node, event source, or full-text search (content + service name)
- ULID-based IDs for guaranteed sort-order correctness
- Annotate any event with notes — with user attribution

### Node Management
- Nodes auto-register on first agent heartbeat
- Per-node metadata: agent version, IP address, OS, last-seen timestamp
- Live node pulse indicator in the sidebar

### Authentication
- Local username/password with Argon2id hashing
- JWT session cookies (configurable TTL, default 24h)
- OIDC (OpenID Connect) support for Authentik, Authelia, Keycloak, etc.
- Admin bootstrap wizard on first launch
- Invite-code-based user registration

### Security
- Distroless container images (no shell, non-root user)
- Constant-time token comparison for all shared secrets
- Rate limiting on auth endpoints
- Security headers middleware

## Configuration Reference

### Server Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `JWT_SECRET` | Yes | — | Secret key for signing JWT session tokens. Use a long random string. |
| `AGENT_TOKENS` | Yes | — | Comma or newline-separated `node-name=token` pairs. See [Agent Tokens](#agent-tokens). |
| `WEBHOOK_SECRET` | Yes | — | Shared secret for validating webhook requests. |
| `DB_PATH` | No | `/data/blackbox.db` | Path to the SQLite database file. |
| `LISTEN_ADDR` | No | `:8080` | TCP address the server binds to. |
| `JWT_TTL` | No | `24h` | JWT cookie lifetime. Accepts Go duration strings (e.g., `12h`, `7d`). |
| `OIDC_ENABLED` | No | `false` | Set to `true` to enable OpenID Connect login. |
| `OIDC_ISSUER` | If OIDC | — | OIDC issuer URL (e.g., `https://auth.example.com`). |
| `OIDC_CLIENT_ID` | If OIDC | — | OIDC application client ID. |
| `OIDC_CLIENT_SECRET` | If OIDC | — | OIDC application client secret. |
| `OIDC_REDIRECT_URL` | If OIDC | — | Callback URL (e.g., `https://blackbox.example.com/api/auth/oidc/callback`). |

### Agent Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `SERVER_URL` | Yes | — | Base URL of the Blackbox server (e.g., `http://blackbox-server:8080`). |
| `AGENT_TOKEN` | Yes | — | Token matching an entry in the server's `AGENT_TOKENS`. |
| `NODE_NAME` | No | System hostname | Identifier for this node in the timeline. |
| `WATCH_PATHS` | No | — | Colon-separated list of directories to watch for file changes (e.g., `/etc:/opt/appdata`). |
| `WATCH_IGNORE` | No | — | Colon-separated glob patterns to exclude from file watching. |

### Agent Tokens

The `AGENT_TOKENS` variable maps node names to secrets. Each agent authenticates using its name and token pair via `X-Blackbox-Node-Name` and `X-Blackbox-Agent-Key` headers.

```bash
# Single node
AGENT_TOKENS="my-homelab=supersecrettoken"

# Multiple nodes
AGENT_TOKENS="node-01=token1,node-02=token2,nas=token3"
```

Generate strong tokens with:

```bash
openssl rand -hex 32
```

---

## Multi-Node Deployment

Deploy an agent on each machine you want to monitor. All agents point at the same central server.
> The server should run with its default distroless `nonroot` user. The agent example keeps `user: "0:0"` because `/var/run/docker.sock` often requires elevated access unless you align the container group with the host Docker socket group.

**Node 01 — Primary server (also runs an agent):**

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

  blackbox-agent:
    image: ghcr.io/maxjb-xyz/blackbox-agent:latest
    user: "0:0"
    restart: unless-stopped
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
    environment:
      SERVER_URL: "http://blackbox-server:8080"
      AGENT_TOKEN: "token-for-node-01"
      NODE_NAME: "node-01"

volumes:
  blackbox-data:
```

**Node 02 — Any other machine on your network:**

```yaml
services:
  blackbox-agent:
    image: ghcr.io/maxjb-xyz/blackbox-agent:latest
    user: "0:0"
    restart: unless-stopped
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
      - /opt/appdata:/watch/appdata:ro
    environment:
      SERVER_URL: "http://node-01.lan:8080"
      AGENT_TOKEN: "token-for-node-02"
      NODE_NAME: "node-02"
      WATCH_PATHS: "/watch/appdata"
```

---

## Webhook Integrations

### Uptime Kuma

In Uptime Kuma, add a notification with type **Webhook** and URL:

```
http://blackbox.example.com/api/webhooks/uptime
```

Add a header: `X-Webhook-Secret: your-webhook-secret`

When a monitor goes down, Blackbox will automatically query the 120-second window before the incident and surface correlated events (e.g., a container that died, a config file that changed).

### Watchtower

In your Watchtower config, add:

```bash
WATCHTOWER_NOTIFICATION_URL="webhook+http://blackbox.example.com/api/webhooks/watchtower"
WATCHTOWER_HTTP_API_TOKEN="your-webhook-secret"
```

Blackbox will log every image update with the container name and new image version.

---

## OIDC / SSO Setup

Blackbox supports any OIDC-compliant provider (Authentik, Authelia, Keycloak, etc.).

**Example — Authentik:**

1. Create a new **OAuth2/OIDC Provider** in Authentik.
2. Set the redirect URI to `https://blackbox.example.com/api/auth/oidc/callback`.
3. Configure the server:

```yaml
environment:
  OIDC_ENABLED: "true"
  OIDC_ISSUER: "https://authentik.example.com"
  OIDC_CLIENT_ID: "your-client-id"
  OIDC_CLIENT_SECRET: "your-client-secret"
  OIDC_REDIRECT_URL: "https://blackbox.example.com/api/auth/oidc/callback"
```

When OIDC is enabled, a **Sign in with SSO** button appears on the login page alongside local auth.

---

## Reverse Proxy

### Nginx

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

### Caddy

```caddyfile
blackbox.example.com {
    reverse_proxy localhost:8080
}
```

---

## Architecture

Blackbox is split into two components designed to run across multiple nodes.

```
┌─────────────────────────────────────────────┐
│               Blackbox Server               │
│  ┌──────────┐  ┌──────────┐  ┌───────────┐  │
│  │ React UI │  │ REST API │  │  SQLite   │  │
│  │(embedded)│  │ + Auth   │  │  /data/   │  │
│  └──────────┘  └──────────┘  └───────────┘  │
│         ▲              ▲              ▲     │
└─────────┼──────────────┼──────────────┼─────┘
          │              │              │
    Browser           Agents        Webhooks
                         │
         ┌───────────────┼───────────────┐
         │               │               │
  ┌──────┴─────┐  ┌──────┴─────┐  ┌──────┴─────┐
  │  Agent     │  │  Agent     │  │  Agent     │
  │  node-01   │  │  node-02   │  │  node-03   │
  │            │  │            │  │            │
  │ Docker sock│  │ Docker sock│  │ Watch paths│
  └────────────┘  └────────────┘  └────────────┘
```

| Component | Role |
|-----------|------|
| **Server** | Central brain. Hosts the UI, stores the database, receives events from agents, and handles webhook ingestion. |
| **Agent** | Lightweight binary deployed on each node. Watches Docker and config files, pushes events to the server. |

---

## Building from Source

**Requirements:**
- Go 1.22+
- Node.js 20+
- Docker (optional, for building images)

```bash
git clone https://github.com/maxjb-xyz/blackbox.git
cd blackbox

# Build the frontend and stage it for the embedded server build
cd web && npm install && npm run build:server && cd ..

# Build the server (embeds the built frontend)
cd server && go build -o blackbox-server ./... && cd ..

# Build the agent
cd agent && go build -o blackbox-agent ./... && cd ..
```

**Run locally:**

```bash
# Server
JWT_SECRET=dev AGENT_TOKENS="local=devtoken" WEBHOOK_SECRET=dev ./server/blackbox-server

# Agent (separate terminal)
SERVER_URL=http://localhost:8080 AGENT_TOKEN=devtoken NODE_NAME=local ./agent/blackbox-agent
```

**Build Docker images:**

```bash
# Server
docker build -f server/Dockerfile -t blackbox-server .

# Agent
docker build -f agent/Dockerfile -t blackbox-agent .
```

---

## API Reference

All protected endpoints require an authenticated session cookie (obtained via login). Agent endpoints require `X-Blackbox-Agent-Key` and `X-Blackbox-Node-Name` headers. Webhook endpoints require an `X-Webhook-Secret` header.

### Timeline

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/entries` | List entries. Query params: `cursor`, `limit` (1-200), `node`, `source`, `q` (search). |
| `POST` | `/api/entries` | Create a manual entry. |
| `GET` | `/api/entries/{id}` | Get a single entry. |
| `GET` | `/api/entries/{id}/notes` | Get notes for an entry. |
| `POST` | `/api/entries/{id}/notes` | Add a note to an entry. |
| `DELETE` | `/api/notes/{id}` | Delete a note. |

### Nodes

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/nodes` | List all registered nodes with metadata. |

### Services

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/services/aliases` | List service name aliases. |
| `POST` | `/api/services/aliases` | Create a service alias. |
| `DELETE` | `/api/services/aliases/{alias}` | Remove a service alias. |

### Agent Ingestion

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/agent/push` | Push events from an agent. Requires agent auth headers. |

### Webhooks

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/webhooks/uptime` | Uptime Kuma Down/Up events. |
| `POST` | `/api/webhooks/watchtower` | Watchtower image update events. |

---

## Persistent Data

The server stores all data in a single SQLite file at `/data/blackbox.db` (configurable via `DB_PATH`). Mount a named volume or host path to persist it across container restarts.

```yaml
volumes:
  - blackbox-data:/data        # named volume (recommended)
  - /opt/appdata/blackbox:/data  # or host path
```

If you bind-mount a host directory instead of a named volume, make sure it is writable by the server container's runtime user (`uid=65532`, `gid=65532`), for example `sudo chown 65532:65532 /opt/appdata/blackbox`.

The database is automatically migrated on startup — no manual schema management required.

---

## Roadmap

- [x] Monorepo scaffold (Go workspace + Docker builds)
- [x] Agent Docker event watching
- [x] Agent file watching (fsnotify)
- [x] Server REST API + SQLite
- [x] JWT + OIDC authentication
- [x] Uptime Kuma + Watchtower webhook ingestion
- [x] React timeline UI (Zerobyte dark theme)
- [x] Node management + pulse indicator
- [x] Manual entry creation + notes
- [x] Service aliases
- [ ] Improved correlation engine (automatic causation for downtime events)
- [ ] Option to integrate local AI into correlation engine
- [ ] Timeline UI polish and interaction improvements
- [ ] Grafana data source plugin
- [ ] Mobile-friendly view

---

## License

[AGPL-3.0](LICENSE)

---

<p align="center">
  Built for homelabbers who want to know what broke their lab at 2 AM.
</p>
