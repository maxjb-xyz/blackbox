---
title: Komodo
---

# Komodo

Blackbox supports [Komodo](https://komo.do/) as a server-scoped webhook source. Komodo manages deployments, builds, automations, and server resources across multiple hosts. Events from Komodo appear on the timeline alongside Docker, file, and systemd events from your agents.

Unlike Watchtower and Uptime Kuma, Komodo is **non-singleton**: you can create one source per Komodo instance, so prod and staging environments can each have their own entry with separate secrets and filtering rules.

## Setup

### 1. Create the source in Blackbox

In **Admin > Data Sources**, click **Add Source** under the **Server** section. Select **Komodo**, give it a name, and fill in the config:

- **Secret** — any non-empty string; you will use it in Komodo's webhook config below.
- **Allowed Types** — the list of Komodo alert type strings you want recorded. Events with any other type are silently ignored. See [Supported Alert Types](#supported-alert-types) below.
- **Node Map** (optional) — maps Komodo server/resource names to Blackbox node names. Useful when your Komodo node names differ from the names your agents registered under. Unmapped names pass through as-is.

### 2. Configure Komodo

In Komodo, go to your alert configuration and add a webhook notification:

- **URL:** `http://your-blackbox-host/api/webhooks/komodo`
- **Header:** `X-Webhook-Secret: your-secret`

Replace `your-blackbox-host` with the address of your Blackbox server and `your-secret` with the secret you set in step 1.

:::tip Multiple Komodo instances
Create a separate source entry in Blackbox for each Komodo instance. Each entry has its own secret, so the webhook endpoint can route events to the correct instance config.
:::

## Supported Alert Types

Add the Komodo alert type strings you care about to **Allowed Types**. Any type string is accepted — Blackbox records whatever Komodo sends as long as it matches the list.

Recommended types (not redundant with the Docker socket source):

| Type | Description |
|------|-------------|
| `BuildFailed` | A build job failed |
| `BuildCompleted` | A build job completed successfully |
| `DeploymentStateChange` | A deployment changed state |
| `StackStateChange` | A compose stack changed state |
| `ProcedureCompleted` | An automation procedure completed |
| `ProcedureFailed` | An automation procedure failed |
| `ScheduledTaskCompleted` | A scheduled task completed |
| `ScheduledTaskFailed` | A scheduled task failed |
| `ResourceSyncPending` | A resource sync is pending |
| `ResourceSyncCompleted` | A resource sync completed |
| `ResourceSyncFailed` | A resource sync failed |
| `ServerCpu` | Server CPU threshold alert |
| `ServerMem` | Server memory threshold alert |
| `ServerDisk` | Server disk threshold alert |
| `ServerUnreachable` | A managed server became unreachable |

`ContainerStateChange` is intentionally omitted from the list above — your agent's Docker socket source already captures container lifecycle events, so including it would create duplicates on the timeline.

## Webhook Payload

Komodo sends a JSON envelope with a `type` field and a `data` object:

```json
{
  "type": "BuildFailed",
  "data": {
    "name": "my-service",
    "server_name": "prod-komodo"
  }
}
```

The `data` shape varies by alert type. Blackbox extracts known fields and stores the rest under `komodo.raw` in metadata.

## What Blackbox Does

For each accepted event:

- Records a timeline entry with `source: "komodo"`.
- Sets the **service** to `data.name` (falls back to `"komodo"` if absent).
- Sets the **node name** to `data.server_name`, applying the Node Map if configured (falls back to `data.name`, then `"komodo"`).
- Sets the **event** to the Komodo type string, lowercased (e.g. `"buildfailed"`).
- Sets the **content** to a human-readable summary, e.g. `"Build Failed: my-service"`.
- Stores `komodo.type`, `komodo.name`, `komodo.server_name`, and `komodo.id` in metadata. Unknown `data` fields are stored under `komodo.raw`.

Events not in the **Allowed Types** list return `204 No Content` and are not recorded.

## Node Map

If your Komodo server names differ from the node names your agents registered under, use the Node Map to keep timeline entries associated with the right node.

Example: if Komodo calls a server `prod-komodo` but your Blackbox agent registered as `prod-1`, add the mapping:

```text
prod-komodo → prod-1
```

Timeline entries from Komodo for that server will then appear under `prod-1` alongside agent events from the same host.
