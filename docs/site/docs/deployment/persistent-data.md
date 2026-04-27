---
title: Persistent Data
---

# Persistent Data

Blackbox stores persistent data in two places.

## Server Database

The server stores all application data in a single SQLite file at
`/data/blackbox.db` unless `DB_PATH` is overridden.

Persist this directory with either:

- A named volume.
- A bind mount to a host path.

## Agent Queue

The agent stores queued outbound events in `/data/queue.db` unless
`QUEUE_DB_PATH` is overridden.

This queue is important because it lets events survive:

- Agent restarts.
- Temporary server downtime.
- Short network interruptions.

## Example Volume Setup

```yaml
volumes:
  - blackbox-data:/data
  - blackbox-agent-data:/data
```

## Host Path Ownership

If you bind-mount a host directory for the server database, make sure it is
writable by UID/GID `65532`, for example:

```bash
sudo chown 65532:65532 /opt/appdata/blackbox
```

## Migrations

The server migrates its database schema automatically on startup.
