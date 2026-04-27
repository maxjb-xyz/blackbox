---
title: Code Style And Principles
---

# Code Style And Principles

Blackbox benefits from operational clarity more than cleverness.

## Recommended Principles

- Prefer readable event and incident behavior over abstraction for its own sake.
- Keep source configuration validation explicit.
- Preserve backwards compatibility where possible, especially for persisted
  data.
- Treat docs as part of the feature, not cleanup work.
- Keep tests close to the behavior being changed.

## When Working Across Components

If a change spans server, agent, and UI:

- Keep naming consistent across all three layers.
- Make sure the timeline shape remains predictable.
- Update docs so operators and contributors can follow the new behavior.
