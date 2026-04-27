---
title: Docs Home
slug: /intro
---

# Blackbox Docs

Blackbox is a self-hosted forensic event timeline for homelabs and home
servers. It collects infrastructure events from agents and webhooks, correlates
them into a single timeline, and groups likely outages into incidents with
scored cause candidates and optional AI analysis.

These docs are organized around the jobs people usually need to do:

- Get a single-node install running quickly.
- Deploy one central server with multiple agents.
- Configure and understand the built-in data sources.
- Operate Blackbox safely in production.
- Contribute code, docs, and new source types.

## Start Here

- New install: [Quick Start](./getting-started/quick-start.md)
- Multiple machines: [Multi-Node Deployment](./deployment/multi-node.md)
- Contributor onboarding: [Contributing Overview](./contributing/overview.md)
- New source work: [Adding a Data Source](./contributing/adding-a-data-source.md)

## Major Sections

- **Getting Started** covers the fastest path to a working install and the core
  concepts behind nodes, sources, and incidents.
- **Deployment** covers single-node and multi-node layouts, reverse proxies,
  and persistence.
- **Configuration** covers environment variables and operational defaults.
- **Data Sources** explains Docker, file watching, systemd, and webhooks.
- **Incidents And Timeline** explains what Blackbox stores and how correlation
  works.
- **Integrations** covers notifications, OIDC, MCP, and the API reference.
- **Operations** covers troubleshooting, observability, and security behavior.
- **Contributing** covers development setup, repo structure, testing, and how
  to add new source types.
