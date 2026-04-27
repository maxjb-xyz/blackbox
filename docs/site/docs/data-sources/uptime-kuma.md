---
title: Uptime Kuma
---

# Uptime Kuma

Blackbox supports Uptime Kuma as a server-scoped webhook source. When a monitor goes down, Blackbox opens a confirmed incident and correlates it with recent agent events to identify a likely cause.

## Setup

### 1. Create the source in Blackbox

In **Admin > Data Sources**, click **Add Source** under the **Server** section. Select **Uptime Kuma**, give it a name, and enter a secret. The secret can be any non-empty string — you will use it in the Uptime Kuma configuration below.

### 2. Configure Uptime Kuma

In Uptime Kuma, go to **Settings > Notifications** and add a new notification:

- **Notification Type:** Webhook
- **Webhook URL:** `http://your-blackbox-host/api/webhooks/uptime`
- **Request Header:** add a custom header:
  - Name: `X-Webhook-Secret`
  - Value: the secret you set in step 1

Apply the notification to the monitors you want Blackbox to track.

## Webhook Payload

Blackbox expects Uptime Kuma's standard JSON webhook body:

```json
{
  "heartbeat": {
    "status": 0,
    "time": "2026-01-01T02:00:00Z",
    "msg": "Connection refused"
  },
  "monitor": {
    "name": "my-app"
  }
}
```

`status` is `0` for down and `1` for up. The `monitor.name` field becomes the service name on the timeline entry (lowercased).

## What Blackbox Does

**On a `down` event:**

- Records a timeline entry with event `down`.
- Looks back through recent Docker, file, and systemd events on all nodes for a likely cause. The best-scoring cause is attached as `metadata.possible_cause` and the entry is cross-linked via `correlated_id`.
- Opens a confirmed incident.

**On an `up` event:**

- Records a timeline entry with event `up`.
- If a prior `down` entry exists for the same monitor, calculates and stores the outage duration in `metadata.duration_seconds`.
- Can auto-resolve the associated incident.

## Correlation

Correlation looks backward from the moment the down signal arrived. The monitor name is used to match against service names in agent events. Entries are scored by recency and relevance. If a single strong candidate is found, it is attached to the incident as the suspected cause.

Correlation is skipped if Uptime Kuma did not send a valid RFC 3339 timestamp in `heartbeat.time`. In that case, Blackbox falls back to the current server time for the entry timestamp and logs a warning.

## Service Name

The service name on every Uptime Kuma entry is the `monitor.name` value, lowercased. This is what Blackbox uses to correlate against agent events and to group entries on the timeline. Name your Uptime Kuma monitors to match the service names used in your Docker Compose stacks or container labels for the best correlation results.
