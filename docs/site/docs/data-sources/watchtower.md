---
title: Watchtower
---

# Watchtower

Blackbox supports Watchtower as a server-scoped webhook source.

## Watchtower Configuration

```bash
WATCHTOWER_NOTIFICATION_URL="webhook+http://blackbox.example.com/api/webhooks/watchtower"
WATCHTOWER_HTTP_API_TOKEN="your-webhook-secret"
```

## What Blackbox Does With It

- Records image update events with version metadata.
- Uses update evidence when a restart follows shortly after.
- Surfaces update timing as part of incident analysis.

## Source Config

The source config requires a non-empty `secret`.
