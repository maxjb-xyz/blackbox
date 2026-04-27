---
title: Systemd
---

# Systemd

The systemd source is an agent-scoped Linux-only source that watches selected units through the systemd journal (journald). OOM kills from the kernel are also captured regardless of which units are configured.

## Requirements

- Linux host
- Agent built with the `linux`, `cgo`, and `systemd` build tags (the standard Linux agent image includes this)
- `WATCH_SYSTEMD=true` set on the agent
- Read-only access to the host journal (see mounts below)

## Agent Environment Variable

```yaml
environment:
  - WATCH_SYSTEMD=true
```

Without this variable, the agent does not start the systemd watcher and will not advertise the `systemd` capability, so the source appears as "unavailable" in the catalog.

## Required Volume Mounts

```yaml
volumes:
  - /run/log/journal:/run/log/journal:ro
  - /var/log/journal:/var/log/journal:ro
  - /etc/machine-id:/etc/machine-id:ro
```

Both journal paths are needed because persistent journal storage uses `/var/log/journal` while runtime storage uses `/run/log/journal`. The `machine-id` mount is required for the journal library to identify the host.

## Setup

1. Set `WATCH_SYSTEMD=true` and add the volume mounts to the agent container.
2. In **Admin > Data Sources**, open the node and click **Add Source**.
3. Select **Systemd**.
4. Enter the unit names to watch, one per line. Names are normalized automatically — missing `.service` suffixes are appended, duplicates and blank entries are removed.
5. Save. The agent picks up the new unit list live without restarting.

## Adding or Changing Watched Units

Unit changes take effect immediately. The agent polls for config updates and rebuilds its journal filter on the next poll cycle. No agent restart is required.

## Unit Naming

Enter units as `nginx.service`, `postgresql.service`, etc. If you omit the suffix, Blackbox appends `.service` automatically. Names are validated against a safe character set before being stored.

## Events Emitted

| Event | Trigger |
|-------|---------|
| `started` | Unit transitions to `active` state |
| `stopped` | Unit transitions to `deactivating` or `inactive` from `active` |
| `restart` | Journal message contains "Scheduled restart job" |
| `failed` | Journal message contains "Failed with result" or "Failed to start" |
| `oom_kill` | Kernel OOM kill detected (facility 0, message contains "Out of memory") |

Events are derived from journal messages, not from DBus state polling. Only manager-originated lifecycle messages are processed — regular log output from the service itself does not produce events.

## Log Capture on Failure

When a `failed` event is detected, the agent captures the last 50 journal lines for that unit and stores them in `metadata.log_snippet`. These lines appear in the timeline detail view.

## OOM Kill Detection

OOM kills are detected independently of the configured unit list. Any kernel message at syslog facility 0 containing "Out of memory" produces an `oom_kill` entry attributed to the `kernel` service. The killed process name and PID are extracted from the message and stored in metadata.

## Incident Behavior

- `failed` and `oom_kill` events can open suspected incidents immediately.
- Repeated `restart` or `failed` events can escalate an incident.
- A `started` event followed by a stability window can auto-resolve a suspected incident.

For troubleshooting details, see [Troubleshooting Systemd](../operations/troubleshooting-systemd.md).
