---
title: Nodes And Heartbeats
---

# Nodes And Heartbeats

Blackbox models each monitored machine as a node. In most deployments, a node
appears after an agent successfully authenticates and begins sending
heartbeats.

## Registration

Node registration depends on the agent reaching the server with:

- `SERVER_URL`
- `NODE_NAME`
- `AGENT_TOKEN`
- A matching `AGENT_TOKENS` entry on the server

Once that handshake succeeds, the node becomes available in the UI and can
participate in source configuration and timeline correlation.

## Metadata Blackbox Tracks

For each node, Blackbox can surface operational details such as:

- Node name
- Recent heartbeat or last-seen time
- Reported agent version
- IP address
- Reported capabilities

These fields help explain why one node may expose different source options or
behave differently from another.

## Heartbeats And Status

- A recent heartbeat is the clearest sign that the agent is connected.
- The UI uses heartbeat freshness to show node health at a glance.
- Source setup and troubleshooting are easier when heartbeat status and
  reported capabilities line up with the machine you expect.

## Why This Matters

Node status is not just informational.

- It affects confidence that agents are still sending events.
- It explains capability-aware source availability in **Admin > Data Sources**.
- It gives operators a fast way to spot version drift or stale agents.

## If Something Looks Wrong

If a node never appears, looks stale, or shows unexpected metadata, start with
[Troubleshooting Agents And Nodes](./troubleshooting-agents-and-nodes.md).
