---
title: Incidents
---

# Incidents

Incidents are Blackbox's higher-level view of operational failures.

## Two Incident Modes

- **Confirmed incidents** are usually driven by explicit monitor signals such as
  Uptime Kuma down and up events.
- **Suspected incidents** are inferred from local failure patterns like Docker
  crash loops, systemd failures, and update-related restarts.

## Event Roles

Blackbox reasons about incidents using roles such as:

- Trigger
- Cause
- Evidence
- Recovery

## What The UI Shows

- Open and resolved state
- Duration
- Linked event chain
- Root-cause candidate selection
- Optional AI summary and AI-linked causes
