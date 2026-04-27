---
title: Watchtower
---

# Watchtower

Blackbox supports Watchtower as a server-scoped webhook source. Update events are recorded on the timeline and can be used as evidence in incident analysis when a container restart follows shortly after an image update.

## Setup

### 1. Create the source in Blackbox

In **Admin > Data Sources**, click **Add Source** under the **Server** section. Select **Watchtower**, give it a name, and enter a secret. The secret can be any non-empty string — you will use it in the Watchtower configuration below.

### 2. Configure Watchtower

Watchtower sends notifications via [shoutrrr](https://containrrr.dev/shoutrrr/). Use the `webhook+http://` service with the `X-Webhook-Secret` header embedded in the URL:

```bash
WATCHTOWER_NOTIFICATIONS=shoutrrr
WATCHTOWER_NOTIFICATION_URL="webhook+http://your-blackbox-host/api/webhooks/watchtower?headers=X-Webhook-Secret:your-secret&contenttype=application/json"
```

Replace `your-blackbox-host` with the address of your Blackbox server and `your-secret` with the secret you set in step 1.

:::tip
If your secret contains characters that need URL encoding (spaces, `&`, etc.), percent-encode them in the URL.
:::

## Webhook Payload

Blackbox expects Watchtower's standard notification JSON body:

```json
{
  "Title": "Watchtower Updates",
  "Message": "Updated containers: my-app (sha256:abc123)",
  "Level": "info"
}
```

The `Message` field is stored as the entry content. Service names are extracted from the message by parsing the comma-separated list after the colon (e.g. `my-app` from the example above). Names are lowercased and deduplicated.

## What Blackbox Does

- Records a timeline entry with event `update` and service `watchtower`.
- Stores extracted container names in `metadata.watchtower.services`.
- Stores the message title and level in metadata.
- Makes the update entry available for incident correlation — if a container restart or failure follows shortly after an update, the update timing is surfaced in the incident evidence.

## Notes

- `WATCHTOWER_HTTP_API_TOKEN` is Watchtower's own REST API token and has no effect on outgoing webhook notifications. Use the `headers=` parameter in `WATCHTOWER_NOTIFICATION_URL` to pass the secret to Blackbox.
- If no containers are updated in a run, Watchtower may not send a notification at all. This is normal behavior.
