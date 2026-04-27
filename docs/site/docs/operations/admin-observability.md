---
title: Admin Observability
---

# Admin Observability

Blackbox includes several built-in operational visibility surfaces for admins.

## Audit Log

The audit log records admin actions such as:

- User management
- Invite creation
- OIDC changes
- Notification destination changes
- Source changes

It is viewable in **Admin > Access > Audit Log**.

## Webhook Delivery Log

The webhook delivery log records inbound webhook deliveries including:

- Source
- Status
- Payload snippet
- Correlated entry ID when available

It is viewable in **Admin > Integrations > Webhook Log**.

## Why It Matters

These surfaces help separate:

- "Did Blackbox receive the event?"
- "Did Blackbox store the event?"
- "Did an admin change a config value?"
