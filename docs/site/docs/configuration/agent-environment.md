---
title: Agent Environment
---

# Agent Environment

These environment variables configure the Blackbox agent.

| Variable | Required | Default | Description |
| --- | --- | --- | --- |
| `SERVER_URL` | Yes | None | Base URL of the Blackbox server. |
| `AGENT_TOKEN` | Yes | None | Secret matching the configured node token. |
| `NODE_NAME` | No | System hostname | Displayed node identifier. |
| `WATCH_PATHS` | No | None | Colon-separated list of container-visible paths to watch. |
| `WATCH_IGNORE` | No | None | Colon-separated glob patterns to exclude from file watching. |
| `WATCH_SYSTEMD` | No | `false` | Enables journal-based systemd monitoring on Linux. |
| `WATCH_PM2` | No | `false` | Enables PM2 lifecycle polling when `pm2` is available. |
| `PM2_BIN` | No | `pm2` on `PATH` | Optional PM2 executable path. |
| `QUEUE_DB_PATH` | No | `/data/queue.db` | Local persistent event queue path. |
| `PUID` | No | `65532` | Runtime user ID for the agent. |
| `PGID` | No | `65532` | Runtime group ID for the agent. |
| `TZ` | No | Container default | IANA timezone for process logs. |

## Most Common Settings

- `SERVER_URL`, `AGENT_TOKEN`, and `NODE_NAME` are always relevant.
- `WATCH_PATHS` matters only if you want file watcher behavior.
- `WATCH_SYSTEMD` matters only on Linux nodes where journald access is mounted.
- `WATCH_PM2` matters only when the agent can execute `pm2 jlist` as the PM2
  process owner; see [PM2](../data-sources/pm2.md).
- `PUID` and `PGID` matter when the default runtime identity cannot read your
  watched paths.
