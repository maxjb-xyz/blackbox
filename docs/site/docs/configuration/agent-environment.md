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
| `QUEUE_DB_PATH` | No | `/data/queue.db` | Local persistent event queue path. |
| `PUID` | No | `65532` | Runtime user ID for the agent. |
| `PGID` | No | `65532` | Runtime group ID for the agent. |
| `TZ` | No | Container default | IANA timezone for process logs. |

## Most Common Settings

- `SERVER_URL`, `AGENT_TOKEN`, and `NODE_NAME` are always relevant.
- `WATCH_PATHS` matters only if you want file watcher behavior.
- `WATCH_SYSTEMD` matters only on Linux nodes where journald access is mounted.
- `PUID` and `PGID` matter when the default runtime identity cannot read your
  watched paths.
