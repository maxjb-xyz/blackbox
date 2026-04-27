---
title: Docker
---

# Docker

Docker is a built-in agent-scoped source for container lifecycle events.

## What It Captures

Blackbox captures Docker events such as:

- `start`
- `stop`
- `die`
- `create`
- `pull`
- `delete`
- Collapsed restart-related transitions

## Important Behaviors

- Events are per-node.
- Service names are inferred from Compose labels, Swarm metadata, image and
  container lookups, and common homelab path layouts.
- Collapsed stop and restart events can include recent crash log tails.

## Exclusions

Operators can exclude:

- Specific Docker containers.
- Entire Compose stacks.

These exclusions are configured in **Admin > Data Sources** and are enforced on
the server side.

## Why Docker Is Virtual

Docker is treated as a virtual built-in source rather than a normal editable
source instance. It exists for compatibility and capability modeling, but the
admin API does not create or delete it like the other source types.
