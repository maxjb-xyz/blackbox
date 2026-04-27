---
title: Architecture
---

# Architecture

Blackbox is split into a central server and one or more agents.

## Components

### Server

The server is the central brain. It:

- Hosts the UI.
- Stores data in SQLite.
- Receives events from agents.
- Handles webhook ingestion.
- Runs the incident and correlation engine.

### Agent

The agent runs on each monitored machine. It:

- Watches the local Docker socket.
- Watches configured file paths.
- Optionally tails the systemd journal on Linux.
- Reports node capabilities to the server.
- Queues events locally before sending them upstream.

### Webhook Sources

Webhook sources live on the server and accept inbound events from systems like
Uptime Kuma and Watchtower.

## Event Flow

1. The agent or webhook source emits an event.
2. The server normalizes and stores it as a timeline entry.
3. The incident engine evaluates whether that event triggers or resolves an
   incident.
4. The UI and API expose the resulting timeline and incident state.

## Why This Model Works Well

- One central UI can describe many machines.
- Local agents stay lightweight and mostly stateless aside from their queue DB.
- Source setup can be node-aware instead of pretending every machine is the
  same.
