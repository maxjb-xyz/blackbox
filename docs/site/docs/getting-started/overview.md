---
title: Overview
---

# Overview

Blackbox is a lightweight event correlation platform for operators who want to
know what changed, when it changed, and what likely caused a breakage.

At a high level:

- The **server** hosts the UI, stores data in SQLite, receives events, and
  runs the incident engine.
- One **agent** runs on each monitored node and pushes Docker, file, and
  optional systemd events to the server.
- **Webhook sources** let tools like Uptime Kuma and Watchtower send events
  directly to the server.
- Blackbox merges everything into a chronological **timeline** and groups
  related failures into **incidents**.

## What Blackbox Is Good At

- Showing Docker lifecycle activity across one or more nodes.
- Tracking config drift through file change events and bounded text diffs.
- Watching selected systemd units on Linux nodes.
- Linking monitor-down events to recent Docker, file, systemd, and webhook
  activity.
- Keeping operators out of the "which machine changed what?" rabbit hole.

## Key Concepts

### Node

A node is a monitored machine identified by its `NODE_NAME`. Nodes usually have
an agent and can expose different capabilities depending on what the agent can
watch.

### Source

A source is a built-in event producer. Some are **agent-scoped** and tied to a
node, while others are **server-scoped** and run centrally on the server.

### Timeline Entry

A timeline entry is a normalized event with a timestamp, source, service name,
node name, event type, content, and metadata.

### Incident

An incident is a grouped failure story built from trigger, cause, evidence, and
recovery events. Incidents can be confirmed by monitor signals or suspected
from local failure patterns.

## Recommended Reading Order

1. [Quick Start](./quick-start.md)
2. [How Blackbox Thinks](./how-blackbox-thinks.md)
3. [Multi-Node Deployment](../deployment/multi-node.md)
4. [Data Sources Overview](../data-sources/overview.md)
