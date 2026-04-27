---
title: Systemd
---

# Systemd

The systemd source is an agent-scoped Linux-only source that watches selected
units through journald.

## Requirements

- Linux host.
- `WATCH_SYSTEMD=true` on the agent.
- Read-only access to the host journal.
- Configured units in **Admin > Data Sources**.

## Events Emitted

Blackbox emits systemd entries such as:

- `started`
- `stopped`
- `restart`
- `failed`
- `oom_kill`

## Mounts For Containerized Agents

```yaml
volumes:
  - /run/log/journal:/run/log/journal:ro
  - /var/log/journal:/var/log/journal:ro
  - /etc/machine-id:/etc/machine-id:ro
```

## Unit Naming

Systemd unit names are normalized:

- Empty values are ignored.
- Missing suffixes are expanded to `.service`.
- Duplicates are removed.

## Incident Behavior

- `failed` and `oom_kill` can open suspected incidents immediately.
- Repeated `restart` and `failed` events can also open suspected incidents.
- A matching `started` signal plus a stability window can resolve some
  incidents automatically.

For troubleshooting details, see
[Troubleshooting Systemd](../operations/troubleshooting-systemd.md).
