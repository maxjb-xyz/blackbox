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

Blackbox is a lightweight, self-hosted event correlation platform built for homelabbers who want to understand their infrastructure at a glance. It collects events from Docker, config file changes, selected systemd units, uptime monitors, and container update tools, correlates them into a single chronological timeline, and groups likely outages into incidents with scored cause candidates and optional local-AI analysis or AI-enhanced correlation.

When your homelab breaks, Blackbox tells you what happened. You don't need to lift a finger.

### Core Principles

- **The Forensic Timeline** — Every event (Docker, file change, systemd, webhook) is timestamped and correlated. Nothing is lost.
- **The "Discipline" Fix** — Automation handles the *what*. Your notes handle the *why*. No manual logging of events that can be detected automatically.
- **The 2 AM Rule** — High-density, low eye strain (dark mode), zero-friction navigation. Built for stressful troubleshooting.

---

## Quick Start

> This gets a single-node setup running in minutes. For multi-node, see [Multi-Node Deployment](#multi-node-deployment).
> Both images run as non-root. The server uses UID 65532 (fixed). The agent defaults to UID/GID 65532 but can be overridden with `PUID`/`PGID` to match the owner of your watched host paths. The agent entrypoint auto-detects the GIDs of any mounted resources (Docker socket, systemd journal) at startup — no manual group configuration needed.

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

**4. Optional: open Admin > Settings to configure file-diff redaction and Ollama-based incident analysis. Open Admin > Systemd to choose which units each node should watch when `WATCH_SYSTEMD=true` is enabled on that agent.**

---

## Features

### Event Ingestion
- **Docker events** — Automatic detection of container start, stop, die, create, pull, and delete events. Per-node, with a 3-second debounce to collapse rapid restarts into a single restart/stop entry.
- **Smarter service inference** — Docker and file events resolve service names from Compose labels, Swarm metadata, image/container lookups, and common homelab path layouts so correlation works with cleaner service names.
- **Crash log capture** — Collapsed Docker stop/restart events include a best-effort tail of recent container logs, which Blackbox can surface directly inside incidents.
- **Config file watching** — inotify-based watching of `.yaml`, `.yml`, `.conf`, `.env`, `.json`, and `.ini` files via configurable `WATCH_PATHS`.
- **Config diffs** — File change events include a bounded text diff in metadata for deeper timeline analysis, with optional secret redaction controlled from the Admin settings page.
- **Systemd unit watching** — Linux agents can watch selected systemd units from the journal and emit `started`, `stopped`, `restart`, and `failed` lifecycle entries.
- **OOM kill detection** — Kernel OOM events are ingested as `systemd` entries, and failed units include a recent journal log snippet for faster triage.
- **Uptime Kuma webhooks** — Ingest Down/Up state changes. Down events open confirmed incidents and score likely causes from recent Docker, file, and webhook activity.
- **Watchtower webhooks** — Ingest container image update events with version metadata and use them as incident evidence when a restart follows shortly after.
- **Custom entries** — Post arbitrary events via the API.

### Incidents & Correlation
- **Incident lifecycle** — Blackbox opens confirmed incidents from monitor-down events and suspected incidents from crash loops, unexpected container exits, watched systemd failures/OOM kills/restart loops, and update-triggered restarts.
- **Weighted cause scoring** — Likely causes are ranked from recent Docker, file, systemd, and update entries using event-specific lookback windows, same-node bonuses, and log-snippet bonuses.
- **Event chain view** — The Incidents page shows open and resolved incidents, duration, linked trigger/cause/evidence/recovery events, AI-derived causes when enabled, and the chosen root-cause entry.
- **Optional Ollama modes** — If Ollama is configured, Blackbox can run either plain AI analysis or an AI-enhanced correlation mode from the same Admin settings page.
- **AI across suspected incidents too** — Suspected incidents get the same log-backed AI treatment as confirmed incidents, including crash-log context and the selected Ollama mode.

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
- Non-root container images — server runs distroless (no shell, UID 65532), agent runs as configurable `PUID`/`PGID` (default 65532) with all capabilities dropped and a read-only filesystem
- Constant-time token comparison for all shared secrets
- Rate limiting on auth endpoints
- Security headers middleware

---

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

### Agent Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `SERVER_URL` | Yes | — | Base URL of the Blackbox server (e.g., `http://blackbox-server:8080`). |
| `AGENT_TOKEN` | Yes | — | Token matching an entry in the server's `AGENT_TOKENS`. |
| `NODE_NAME` | No | System hostname | Identifier for this node in the timeline. |
| `WATCH_PATHS` | No | — | Colon-separated list of directories to watch for file changes as seen inside the agent container (e.g., `/watch/etc:/watch/appdata`). |
| `WATCH_IGNORE` | No | — | Colon-separated glob patterns to exclude from file watching. |
| `WATCH_SYSTEMD` | No | `false` | Set to `true` on Linux agents to enable journal-based systemd monitoring for the units configured in the Admin UI. |
| `PUID` | No | `65532` | UID the agent process runs as. Set to your host user's UID (`id -u`) when you own the watched paths. |
| `PGID` | No | `65532` | GID the agent process runs as. Set to your host user's GID (`id -g`) when you own the watched paths. |

#### Granting File Watcher Access

The agent needs read access to every directory and file under your `WATCH_PATHS`. The right approach depends on who owns those files on the host.

##### You own the files (easiest)

Set `PUID` and `PGID` to your host user's IDs. The agent runs as you, so your files are already readable — no host permission changes needed.

```yaml
environment:
  PUID: "1000"   # your host UID — run: id -u
  PGID: "1000"   # your host GID — run: id -g
```

##### Root or another user owns the files

Leave `PUID`/`PGID` at their defaults (or set them to match the owning user if you have that flexibility), then grant the agent's UID explicit read access on the host.

**Recommended: ACLs** (non-destructive, works alongside existing permissions)

```bash
WATCH_ROOT=/srv/stacks
AGENT_UID="${PUID:-65532}"   # match whatever PUID you set

# Allow traversal of directories
sudo find "$WATCH_ROOT" -type d -exec setfacl -m u:${AGENT_UID}:rx {} +

# Allow reading files
sudo find "$WATCH_ROOT" -type f -exec setfacl -m u:${AGENT_UID}:r {} +

# Inherit for new files/directories
sudo setfacl -d -m u:${AGENT_UID}:rx "$WATCH_ROOT"
sudo find "$WATCH_ROOT" -type d -exec setfacl -d -m u:${AGENT_UID}:rx {} +
```

**Alternative: traditional Unix mode bits** (if ACLs are unavailable)

```bash
AGENT_GID="${PGID:-65532}"   # match whatever PGID you set

sudo chgrp -R "$AGENT_GID" /srv/stacks
sudo find /srv/stacks -type d -exec chmod 750 {} +
sudo find /srv/stacks -type f -exec chmod 640 {} +
```

Notes:

- `PUID`/`PGID` default to `65532` if not set. Numeric ownership is what matters on the host — the container has no `/etc/passwd` mapping.
- Parent directories must also be traversable (`x` bit), not just the final config file.
- If part of a tree should stay unreadable, mount only the readable subtree and point `WATCH_PATHS` at that path instead.
- After changing permissions or `PUID`/`PGID`, restart the agent and look for `files watcher: registered ... directories` in the logs.

### File Watcher Troubleshooting

- `WATCH_PATHS` must match the container-side mount target, not the host source path. Example: `- /srv/stacks:/watch/stacks:ro` pairs with `WATCH_PATHS=/watch/stacks`.
- On startup, the agent now logs a per-root registration line. If you see `failed to register root /watch/stacks`, the bind mount and `WATCH_PATHS` do not line up inside the container.
- The agent runs as `PUID`/`PGID` (default `65532`). For watched bind mounts, that user must be able to traverse every directory under each watched root and read the files you want diffed. A single unreadable subdirectory can cause the whole watched root to fail registration.
- `WATCH_IGNORE` helps skip noisy paths after they are reachable, but it cannot bypass a directory the container user cannot traverse at all. If a tree contains unreadable secrets, narrow `WATCH_PATHS` to a readable subdirectory instead of mounting the whole parent.
- Some editors save by replacing files instead of writing them in place. Blackbox now emits alerts for those `rename` and `chmod-style` config-file changes as well.
- File-change metadata now includes a small line diff when the file is UTF-8 text and under the tracking limit. Obvious secret values such as `TOKEN`, `PASSWORD`, and `CLIENT_SECRET` are redacted before upload.

### Systemd Monitoring

- Systemd monitoring is Linux-only. On non-Linux hosts the watcher is a no-op.
- Set `WATCH_SYSTEMD=true` on the agent to enable journal monitoring.
- Configure the watched units from **Admin > Systemd**. Settings are stored per node and the agent refreshes them from the server every minute.
- The watcher emits `started`, `stopped`, `restart`, and `failed` events for configured units, plus `oom_kill` events from the kernel journal.
- Failed unit entries include a recent journal snippet in entry metadata, which Blackbox also uses as a correlation scoring bonus.
- A watched unit `failed` or `oom_kill` opens a suspected incident immediately. Repeated watched `restart`/`failed` events within 5 minutes also open a suspected incident, while a lone `restart` does not.
- For containerized agents, mount the host journal read-only. The agent entrypoint auto-detects the journal GID at startup — no manual group configuration needed:

```yaml
volumes:
  - /run/log/journal:/run/log/journal:ro
  - /var/log/journal:/var/log/journal:ro
  - /etc/machine-id:/etc/machine-id:ro
```

### Incident Enrichment

- Incident detection works without extra configuration. Confirmed incidents come from Uptime Kuma Down/Up pairs; suspected incidents come from Docker crash loops and non-zero exits, watched systemd `failed` and `oom_kill` events, watched systemd restart/failure loops, and watchtower-triggered restarts.
- Systemd-sourced suspected incidents resolve immediately on matching monitor `up` events, or automatically after a watched unit emits `started` and stays stable for 2 minutes without another failure/restart.
- Ollama is optional and configured in **Admin > Settings** with an absolute base URL such as `http://192.168.1.10:11434`, a model name such as `llama3.2`, and an AI mode.
- `analysis` mode stores a concise AI-written incident summary in incident metadata.
- `enhanced` mode asks Ollama to analyze the deterministic incident chain plus recent same-node events, then writes validated `ai_cause` links and an AI summary back onto the incident.
- AI mode applies to both confirmed and suspected incidents. Suspected incidents still pull in captured crash logs and run the same selected Ollama path.
- Leaving the Ollama URL or model blank disables all optional AI behavior while keeping the incident engine and deterministic correlation active.
- The same Admin settings page also controls whether newly captured file diffs redact obvious secret-bearing keys before upload.

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
> Both images run as non-root. The server uses UID 65532 (fixed). The agent defaults to UID/GID 65532 but can be overridden with `PUID`/`PGID` to match the owner of your watched host paths. The agent entrypoint auto-detects the GIDs of any mounted resources (Docker socket, systemd journal) at startup — no manual group configuration needed.

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
      - /run/log/journal:/run/log/journal:ro
      - /var/log/journal:/var/log/journal:ro
      - /etc/machine-id:/etc/machine-id:ro
    environment:
      SERVER_URL: "http://blackbox-server:8080"
      AGENT_TOKEN: "token-for-node-01"
      NODE_NAME: "node-01"
      WATCH_SYSTEMD: "true"

volumes:
  blackbox-data:
```

**Node 02 — Any other machine on your network:**

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
    environment:
      SERVER_URL: "http://node-01.lan:8080"
      AGENT_TOKEN: "token-for-node-02"
      NODE_NAME: "node-02"
      WATCH_PATHS: "/watch/appdata"
      WATCH_SYSTEMD: "true"
```

---

## Webhook Integrations

### Uptime Kuma

In Uptime Kuma, add a notification with type **Webhook** and URL:

```
http://blackbox.example.com/api/webhooks/uptime
```

Add a header: `X-Webhook-Secret: your-webhook-secret`

When a monitor goes down, Blackbox will automatically query the 120-second window before the incident and surface correlated events (e.g., a container that died, a config file that changed). If Ollama is enabled, the resulting incident can also receive an AI summary or AI-derived cause links, depending on the selected mode.

### Watchtower

In your Watchtower config, add:

```bash
WATCHTOWER_NOTIFICATION_URL="webhook+http://blackbox.example.com/api/webhooks/watchtower"
WATCHTOWER_HTTP_API_TOKEN="your-webhook-secret"
```

Blackbox will log every image update with the container name and new image version.

---

## OIDC / SSO Setup

Blackbox supports multiple OIDC providers configured from the Admin panel under **Settings > OIDC**.

Each provider entry requires:

- **Name**
- **Issuer URL**
- **Client ID**
- **Client Secret**
- **Redirect URL**

You can configure providers such as Google, Authentik, Keycloak, and other OIDC-compliant identity providers. The redirect URL format is:

```text
https://<your-domain>/api/auth/oidc/<provider-id>/callback
```

Each provider also has an access policy:

- `open`
- `existing accounts only`
- `invite required`

Once configured, the login page shows one or more **Sign in with SSO** options alongside local auth.

---

## User Registration / Invites

Admin users can create invite codes in the Admin panel and copy the full invite link directly for sharing with new users.

Invited users register at:

```text
/register?code=<invite-code>
```

Admins can also revoke unused invite codes from the Admin panel at any time.

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
| **Server** | Central brain. Hosts the UI, stores the database, receives events from agents, handles webhook ingestion, and runs the incident/correlation engine. |
| **Agent** | Lightweight binary deployed on each node. Watches Docker, config files, and optional systemd journals, then pushes events to the server. |

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

To test systemd monitoring locally on Linux, add `WATCH_SYSTEMD=true` and make sure the agent can read the host journal. Then configure units from **Admin > Systemd** after the node registers.

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

### Incidents

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/incidents` | List incidents. Query params: `status`, `confidence`, `service`, `limit` (1-200). |
| `GET` | `/api/incidents/{id}` | Get a single incident with linked trigger/cause/evidence/recovery entries and any `ai_cause` links. |

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

### Admin Settings

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/admin/config` | Load admin-visible runtime config, including webhook secret, file-watcher settings, Ollama URL/model, and `ollama_mode`. |
| `PUT` | `/api/admin/settings/file-watcher` | Update file-diff secret redaction behavior for newly uploaded diffs. |
| `PUT` | `/api/admin/settings/ollama` | Update the Ollama URL, model, and `ollama_mode` (`analysis` or `enhanced`) used for optional incident analysis. |
| `GET` | `/api/admin/settings/systemd` | List per-node systemd unit selections. |
| `PUT` | `/api/admin/settings/systemd/{node_name}` | Replace the watched systemd unit list for a node. |

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
- [x] Incident lifecycle engine
- [x] Improved correlation engine (automatic causation for downtime events)
- [x] Optional local AI analysis via Ollama
- [x] Incidents UI + sidebar badge
- [x] Optional AI-enhanced correlation engine
- [x] Support tracking systemd services
- [ ] Timeline UI polish and interaction improvements
- [ ] Grafana data source plugin
- [ ] Dynamic notifications to Discord
- [ ] Mobile-friendly view

---

## License

[AGPL-3.0](LICENSE)

---

<p align="center">
  Built for homelabbers who want to know what broke their lab at 2 AM.
</p>
