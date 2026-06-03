---
title: Notifications
---

# Notifications

Blackbox can send outbound notifications when notable events occur.

## Supported Targets

Examples include:

- Discord
- Slack
- ntfy

## Features

- Regular event notifications
- AI review notifications after incident analysis completes
- Incident deep-links when you configure an instance base URL

## Quiet Hours & Rate Limiting

Each notification destination can be tuned independently so a 3 AM reboot or a
flapping container does not wake you.

### Quiet hours

Set a daily window (start and end time) during which the destination is
silenced. The window is evaluated in the **server's configured timezone** and
may wrap past midnight (e.g. `22:00`–`07:00`). When a notification lands inside
the window, the destination's mode decides what happens:

- **Drop** — the notification is silently not sent. The incident still appears
  in the timeline; you simply are not pinged.
- **Defer & digest** — held notifications are rolled up and sent as a single
  digest when the window ends. A digest left pending across a server restart is
  flushed on the next startup.

### Rate limit

Cap a destination at a maximum number of notifications per **hour** or per
**day** (a sliding window). Once the cap is reached, further notifications are
**dropped**, and the next notification that *is* allowed through carries a
`(+N suppressed)` note so you know something was held back.

Both controls default to **off**; existing destinations keep firing exactly as
before until you enable them.

## Where To Configure It

Use **Admin > Integrations > Notifications**.
