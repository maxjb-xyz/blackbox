---
title: Uptime Kuma
---

# Uptime Kuma

Blackbox supports Uptime Kuma as a server-scoped webhook source.

## Endpoint

Configure Uptime Kuma to send webhook notifications to:

```text
http://blackbox.example.com/api/webhooks/uptime
```

## Authentication Header

Add:

```text
X-Webhook-Secret: your-webhook-secret
```

## What Blackbox Does With It

- A `down` event can open a confirmed incident.
- Blackbox inspects a recent pre-incident window for likely causes.
- Docker, file, systemd, and other evidence can be attached to the incident.
- A later recovery signal can resolve the incident.

## Source Config

The source config requires a non-empty `secret`.
