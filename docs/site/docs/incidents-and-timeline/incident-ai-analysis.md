---
title: Incident AI Analysis
---

# Incident AI Analysis

AI analysis is optional and configured from **Admin > System**.

## Supported Modes

- `analysis` writes a concise AI summary into incident metadata.
- `enhanced` asks the model to analyze the deterministic chain plus nearby
  evidence and write validated AI cause links and summary data back onto the
  incident.

## Operational Notes

- AI mode applies to both confirmed and suspected incidents.
- Crash-log context is still useful in suspected incidents.
- Leaving the AI base URL or model blank disables the feature without affecting
  deterministic incident handling.

## Notifications

When AI analysis completes, Blackbox can emit an `AI Review Generated`
notification event to subscribed destinations.
