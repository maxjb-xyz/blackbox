---
title: Timezones And Logging
---

# Timezones And Logging

Blackbox stores events with explicit timestamps, but process logs are easier to
reason about when container timezones match your local expectations.

## `TZ`

Set `TZ` on both the server and agent containers if you want their process logs
to line up with your local wall clock, for example:

```yaml
environment:
  TZ: "America/New_York"
```

## Why It Matters

- Container logs are easier to compare with the UI and your other services.
- Troubleshooting startup or file watcher issues becomes less confusing.
- Multi-node deployments still preserve event timestamps, but consistent log
  presentation makes operator life easier.
