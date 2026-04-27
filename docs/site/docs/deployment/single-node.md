---
title: Single-Node Deployment
---

# Single-Node Deployment

Single-node deployment means running the server and one agent together on the
same host.

This is the easiest way to get started and is the deployment model used in the
[Quick Start](../getting-started/quick-start.md).

## When To Use It

- You want the fastest path to a working install.
- You are monitoring one homelab machine first.
- You want to validate Docker, file watcher, and systemd behavior before
  expanding to more nodes.

## Recommended Layout

- One `blackbox-server` container.
- One `blackbox-agent` container.
- One persistent volume for server data.
- One persistent volume for the agent queue.

## Important Notes

- Keep the server's `/data` directory persistent so `blackbox.db` survives
  restarts.
- Keep the agent's `/data` directory persistent so queued events survive network
  or server downtime.
- If you mount host config trees, make sure the agent runtime UID can traverse
  them.
