---
title: Overview
---

# Data Sources Overview

Blackbox organizes event producers as built-in source types.

## Source Scopes

Blackbox currently has two scopes:

- **Agent-scoped** sources belong to a specific node.
- **Server-scoped** sources live on the central server.

## Built-In Source Types

| Type | Scope | Notes |
| --- | --- | --- |
| `docker` | Agent | Virtual built-in source. Shows Docker lifecycle events from a node. |
| `systemd` | Agent | Watches selected Linux systemd units through journald. |
| `filewatcher` | Agent | Watches config paths and emits file change events. |
| `webhook_uptime_kuma` | Server | Accepts Uptime Kuma monitor events. |
| `webhook_watchtower` | Server | Accepts Watchtower update events. |

## Singleton Behavior

All current built-in source types are singletons for their target:

- One `systemd` source per node.
- One `filewatcher` source per node.
- One `webhook_uptime_kuma` source per server.
- One `webhook_watchtower` source per server.

`docker` is special: it is a virtual built-in source and cannot be created as a
normal source row from the admin API.

## Capability Awareness

The server uses node capability data to make source setup sensible per node.
That means:

- Nodes without a file watcher capability should not pretend to support file
  watching.
- Source setup defaults can vary by node.
- Operators can reason about why one node exposes different source options than
  another.

## Secrets And Redaction

When webhook sources store secrets, Blackbox preserves those secrets when you
edit the source and redacts them before sending source config back to clients.
