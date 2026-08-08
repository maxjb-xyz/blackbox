---
title: PM2
---

# PM2

PM2 is an agent-scoped source that polls `pm2 jlist` and turns Node.js process
state changes into normalized timeline entries. It is intentionally small: the
MVP reports lifecycle changes and restarts, not PM2 logs or CPU/memory samples.

## Requirements

- `WATCH_PM2=true` on the agent.
- The `pm2` executable (or the path configured by `PM2_BIN`) must be available
  to the agent process.
- The agent must run as the same user, or use the same `PM2_HOME`, as the PM2
  daemon whose processes should be monitored.

The stock Blackbox agent image does not include PM2 and cannot see a host's PM2
daemon through a normal Docker socket mount. Run the agent natively alongside
PM2, or build an agent image with PM2 and the required process environment.

## Agent configuration

```yaml
environment:
  - WATCH_PM2=true
  # Optional when pm2 is not on PATH:
  - PM2_BIN=/usr/local/bin/pm2
```

When PM2 is enabled and the executable is present, the agent advertises the
`pm2` capability. The source is then available under **Admin > Data Sources**.

## Source setup

1. Enable `WATCH_PM2=true` and restart the agent if its environment changed.
2. In **Admin > Data Sources**, open the node and choose **Add Source**.
3. Select **PM2** and save the source.
4. Optionally add exact PM2 process names, such as `api` or `worker`. Leave the
   list empty to watch every process returned by `pm2 jlist`.

The source is disabled until it is created. Enabling or changing it in the
catalog is picked up by the agent's normal configuration refresh; the PM2
poller runs every 15 seconds.

## Events emitted

| Event | Trigger |
| --- | --- |
| `started` | A watched process becomes `online`, or a new online process appears. |
| `stopped` | A watched process becomes `stopped` or `stopping`. |
| `failed` | A watched process becomes `errored`. |
| `restart` | PM2's `restart_time` increases while the process remains online. |

The first successful poll establishes a baseline and emits no entries for
already-running processes. Entries use `source: "pm2"`, the PM2 process name as
`service`, and include `pm_id`, `pid`, `status`, `restart_time`,
`unstable_restarts`, and (when applicable) `previous_status` in metadata.

## Troubleshooting

Run the same command as the agent user and confirm it returns JSON:

```sh
pm2 jlist
```

If the source is unavailable in the catalog, check that `WATCH_PM2=true` is
set and that `PM2_BIN` is executable in the agent environment. If it is
available but produces no events, verify the PM2 user/`PM2_HOME`, confirm the
source is enabled for that node, and inspect the agent logs for `pm2 watcher`.
