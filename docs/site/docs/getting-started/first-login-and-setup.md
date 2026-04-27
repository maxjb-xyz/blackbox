---
title: First Login And Setup
---

# First Login And Setup

On first launch, Blackbox walks you through the initial admin bootstrap flow.

## Initial Tasks

1. Create the first admin account.
2. Confirm the server is healthy.
3. Verify that your first node has registered.
4. Review the default source setup in **Admin > Data Sources**.

## Admin Areas To Visit First

### Data Sources

Use **Admin > Data Sources** to:

- Inspect server-scoped and agent-scoped source configuration.
- Confirm that the node capability view matches the machine you deployed.
- Configure systemd units, file watcher settings, and webhook secrets.
- Add Docker container or Compose stack exclusions.

### System

Use **Admin > System** to:

- Configure file diff secret redaction behavior.
- Enable and configure optional Ollama-backed incident analysis.
- Enable the MCP server if you want AI assistants to query Blackbox data.

### Integrations

Use **Admin > Integrations** to:

- Add outbound notification destinations.
- Set a base URL for incident links in notifications.
- Review webhook delivery logs after you connect external systems.

## Good First Validation Checks

- Change a container state and confirm an event appears in the timeline.
- If file watching is enabled, touch a watched config file and verify that a
  file event appears.
- If systemd is enabled, confirm the node exposes that capability and has units
  configured.
