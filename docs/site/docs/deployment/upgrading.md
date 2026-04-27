---
title: Upgrading
---

# Upgrading

Blackbox is still in active beta development, so upgrades should be treated
carefully.

## Recommended Upgrade Routine

1. Back up the server data volume or `blackbox.db`.
2. Record your current image tags.
3. Pull the new images.
4. Restart the stack.
5. Confirm server health, node registration, and timeline ingestion.

## What To Validate After An Upgrade

- The UI loads and auth still works.
- Agents reconnect and show recent heartbeats.
- Existing data sources still appear in **Admin > Data Sources**.
- Timeline events continue flowing.
- Webhook deliveries still succeed.

## Why This Page Exists Early

Even before formal release notes exist, operators need a stable place to look
for upgrade expectations, rollback guidance, and migration warnings.
