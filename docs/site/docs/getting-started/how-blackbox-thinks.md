---
title: How Blackbox Thinks
---

# How Blackbox Thinks

Blackbox is designed around operational causality rather than raw log storage.

## Timeline First

Everything starts as a normalized timeline entry. Blackbox tries to keep event
shape consistent across sources so you can filter by node, source, service, and
text search without needing to remember which subsystem produced the event.

## Sources Are Capability-Aware

Some sources only make sense on certain nodes. Agents report capabilities, and
the server uses that capability set to decide which source types should be
available or meaningful for each node.

## Incidents Are Built From Patterns

Blackbox opens incidents from:

- Uptime Kuma down events paired with later recovery signals.
- Docker crash loops and unexpected exits.
- Selected systemd failures, OOM kills, and restart loops.
- Update-related restart patterns when Watchtower activity lines up with a
  failure.

## Correlation Is Deterministic First

The core engine ranks likely causes using source-specific timing windows,
same-node preference, service inference, and other operational hints. Optional
AI analysis adds a second layer, but the deterministic engine is always the
base.

## AI Is Optional

If AI is configured, Blackbox can:

- Write a concise summary of an incident.
- Analyze the deterministic event chain and recent evidence.
- Suggest or validate likely cause links.

If AI is not configured, the incident engine still works normally.
