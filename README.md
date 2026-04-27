<p align="center">
  <img src="docs/assets/logo.png" alt="Blackbox" width="120" />
</p>

<h1 align="center">Blackbox</h1>
<p align="center">
  An intelligent self-hosted forensic event timeline for homelabs and home servers.<br/>
  Know <em>what</em> changed, <em>when</em> it changed, and <em>why</em> things broke.
</p>

<p align="center">
  <img src="https://img.shields.io/badge/Go-1.26-00798A?logo=go&logoColor=white" />
  <img src="https://img.shields.io/badge/React-19-149ECA?logo=react&logoColor=white" />
  <img src="https://img.shields.io/badge/SQLite-portable-002040?logo=sqlite&logoColor=white" />
  <img src="https://img.shields.io/badge/Docker-ready-0B5EA8?logo=docker&logoColor=white" />
  <img src="https://img.shields.io/badge/license-AGPL_v3-A00000" />
</p>

---

<img width="1502" height="810" alt="image" src="https://github.com/user-attachments/assets/a00c9afb-b10d-4bb5-b66b-13d12d846656" />

---

## What is Blackbox?

Blackbox is a lightweight, self-hosted event correlation platform built for homelabbers who want to understand their infrastructure at a glance. It collects events from Docker, config file changes, selected systemd units, uptime monitors, and container update tools, correlates them into a single chronological timeline, and groups likely outages into incidents with scored cause candidates and optional local-AI analysis or AI-enhanced correlation.

When your homelab breaks, Blackbox tells you what happened. You don't need to lift a finger.

Full documentation: [docs.blackboxd.dev](https://docs.blackboxd.dev)

---

> [!CAUTION]
> This is beta software. I am working towards v1.0.0, but in the meantime, it is being actively developed. Expect changes. Possibly breaking ones. During beta development, I would much appreciate if you report issues when you find them and suggest features.

---

## Quick Start

> This gets a single-node setup running in minutes. For multi-node, see the [deployment docs](https://docs.blackboxd.dev/docs/deployment/multi-node).
> Both images run as non-root. The server uses UID 65532 (fixed). The agent defaults to UID/GID 65532 but can be overridden with `PUID`/`PGID` to match the owner of your watched host paths. The agent entrypoint auto-detects the GIDs of any mounted resources (Docker socket, systemd journal) at startup - no manual group configuration needed.

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
      AGENT_TOKENS: "homelab=change-me-to-a-secret-agent-token"
      WEBHOOK_SECRET: "change-me-to-a-webhook-secret"
      TZ: "America/New_York" # optional: set to your local timezone so container logs match your clock
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
      NODE_NAME: "homelab"
      WATCH_PATHS: "/watch/etc"
      WATCH_SYSTEMD: "true"
      TZ: "America/New_York" # optional: set to your local timezone so container logs match your clock
    networks:
      - blackbox

volumes:
  blackbox-agent-data:
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

**4. Optional:**

- Open `Admin > Data Sources` to manage per-node and server-wide data sources and set up capability-aware systemd, file watcher, webhook, and Docker exclusions.
- Open `Admin > System` to configure file-diff redaction and Ollama-based incident analysis.

---

## Features

### Event Ingestion
- **Docker events** - Automatic detection of container start, stop, die, create, pull, and delete events. Per-node, with logic for collapsing events like restarts.
- **Smarter service inference** - Docker and file events resolve service names from Compose labels, Swarm metadata, image/container lookups, and common homelab path layouts so correlation works with cleaner service names.
- **Crash log capture** - Collapsed Docker stop/restart events include a best-effort tail of recent container logs, which Blackbox can surface directly inside incidents.
- **Config file watching** - inotify-based watching of `.yaml`, `.yml`, `.conf`, `.env`, `.json`, and `.ini` files via configurable `WATCH_PATHS`.
- **Config diffs** - File change events include a bounded text diff in metadata for deeper timeline analysis, with optional secret redaction controlled from the Admin settings page.
- **Systemd unit watching** - Linux agents can watch selected systemd units from the journal and emit `started`, `stopped`, `restart`, and `failed` lifecycle entries.
- **OOM kill detection** - Kernel OOM events are ingested as `systemd` entries, and failed units include a recent journal log snippet for faster triage.
- **Uptime Kuma webhooks** - Ingest Down/Up state changes. Down events open confirmed incidents and score likely causes from recent Docker, file, and webhook activity.
- **Watchtower webhooks** - Ingest container image update events with version metadata and use them as incident evidence when a restart follows shortly after.
- **Custom entries** - Post arbitrary events via the API.
- **Container & stack exclusions** - Exclude specific Docker containers or Compose stacks from event ingestion entirely. Configured in Admin > Data Sources. Excluded events are silently dropped server-side - agents do not need reconfiguring.

### Sources
- **Source catalog** - Manage supported event sources from Admin > Data Sources. The catalog is capability-aware per node, supports both server-scoped and agent-scoped sources, and preserves stored secrets when you edit an existing source.

### Notifications
- **Webhook notification support** - Send outbound notifications to Discord, Slack, ntfy, and other targets when Blackbox events occur. Configure destinations in Admin > Integrations > Notifications.
- **AI review notifications** - When Ollama completes an incident analysis, all subscribed notification destinations receive the event with the AI summary embedded in the payload.
- **Incident deep-links** - Configure a **Base URL** in Admin > Integrations > Notifications so notification payloads include a direct link to the incident in Blackbox.

### Incidents & Correlation
- **Incident lifecycle** - Blackbox opens confirmed incidents from monitor-down events and suspected incidents from crash loops, unexpected container exits, watched systemd failures/OOM kills/restart loops, and update-triggered restarts.
- **Weighted cause scoring** - Likely causes are ranked from recent Docker, file, systemd, and update entries using event-specific lookback windows, same-node bonuses, and log-snippet bonuses.
- **Event chain view** - The Incidents page shows open and resolved incidents, duration, linked trigger/cause/evidence/recovery events, AI-derived causes when enabled, and the chosen root-cause entry.
- **Optional AI modes** - If Ollama or other AI provider is configured, Blackbox can run either plain AI analysis or an AI-enhanced correlation mode from the same Admin settings page.
- **AI across suspected incidents too** - Suspected incidents get the same log-backed AI treatment as confirmed incidents, including crash-log context and the selected AI mode.
- **Postmortem PDF reports** - Download a structured PDF report from any resolved incident. The report includes the incident timeline, root cause, AI summary (if available), and a chronological event chain. Accessible via the Download Report button on resolved incident cards.

### Timeline
- Chronological, paginated event feed across all nodes
- Filter by node, event source, or full-text search
- **Full-text search** - SQLite FTS5 powered search across all entry content and service names with term highlighting
- ULID-based IDs for guaranteed sort-order correctness
- Annotate any event with notes - with user attribution

### Node Management
- Nodes auto-register on first agent heartbeat
- Per-node metadata: agent version, IP address, OS, last-seen timestamp
- Agent capability reporting powers source setup defaults and node-specific source availability in the admin UI
- Live node pulse indicator in the sidebar

### Authentication
- Local username/password with Argon2id hashing
- JWT session cookies (configurable TTL, default 24h)
- OIDC (OpenID Connect) support for Authentik, Authelia, Keycloak, etc.
- Admin bootstrap wizard on first launch
- Invite-code-based user registration

### MCP Server (optional)
- **Model Context Protocol server** - Expose your Blackbox data to AI assistants (Claude Desktop, claude.ai, and any MCP-compatible client). Toggle on/off from Admin > System > MCP Server. When enabled, serves an HTTP/SSE MCP endpoint with five tools: `list_incidents`, `get_incident`, `list_entries`, `search_entries`, and `list_nodes`.

### Admin & Observability
- **Audit log** - Every admin action (user management, invite creation, OIDC provider changes, notification destination changes) is recorded with actor identity, IP address, and timestamp. Viewable in Admin > Access > Audit Log. Last 10,000 entries retained.
- **Webhook delivery log** - Every inbound Uptime Kuma and Watchtower webhook is recorded with source, status (processed / ignored / error), payload snippet, and the correlated entry ID when applicable. Viewable in Admin > Integrations > Webhook Log. Last 1,000 entries retained.

### Security
- Non-root container images - server runs distroless (no shell, UID 65532), agent runs as configurable `PUID`/`PGID` (default 65532), drops all capabilities then adds only `SETUID`/`SETGID` for identity switching, with a read-only filesystem
- Constant-time token comparison for all shared secrets
- Rate limiting on auth endpoints
- Security headers middleware

---

## Documentation

Full docs: [docs.blackboxd.dev](https://docs.blackboxd.dev)

- [Quick Start](https://docs.blackboxd.dev/docs/getting-started/quick-start)
- [Single-Node Deployment](https://docs.blackboxd.dev/docs/deployment/single-node)
- [Multi-Node Deployment](https://docs.blackboxd.dev/docs/deployment/multi-node)
- [Server Environment](https://docs.blackboxd.dev/docs/configuration/server-environment)
- [Agent Environment](https://docs.blackboxd.dev/docs/configuration/agent-environment)
- [Data Sources Overview](https://docs.blackboxd.dev/docs/data-sources/overview)
- [Authentication](https://docs.blackboxd.dev/docs/operations/authentication)
- [MCP Server](https://docs.blackboxd.dev/docs/integrations/mcp-server)
- [API Reference](https://docs.blackboxd.dev/docs/integrations/api-reference)

---

## Building from Source

Development setup, local run instructions, and contributor docs live at:

- [Development Setup](https://docs.blackboxd.dev/docs/contributing/development-setup)
- [Testing](https://docs.blackboxd.dev/docs/contributing/testing)
- [Project Structure](https://docs.blackboxd.dev/docs/contributing/project-structure)

---

## License

[AGPL-3.0](LICENSE)

---

## AI Usage

A disclaimer - generative AI is used in the development of this repository. The agenda, features, roadmap, etc. are all set by me (a human), but a large portion of the code in this project is created by generative AI. I scan this code for issues and vulnerabilities the best I know how, but I'm not an experienced programmer.

If that makes you uncomfortable, please feel free to poke around the codebase and submit issues for anything out of place. I welcome feedback and suggestions from those more experienced than me. Please send me a private message if you find a security vulnerability that may affect other users, so I can fix it before informing everyone.

---

## Contributing

Contributions are welcome. Please review the contributing docs before doing so.

[Contributing](https://docs.blackboxd.dev/docs/contributing/overview/)

---

<p align="center">
  Built for homelabbers who want to know what broke their lab at 2 AM.
</p>
