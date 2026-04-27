---
title: Troubleshooting Systemd
---

# Troubleshooting Systemd

## Linux Only

Systemd monitoring is a no-op on non-Linux hosts.

## Check The Basics

- `WATCH_SYSTEMD=true`
- Journal mounts are present
- `/etc/machine-id` is mounted read-only
- The node has a configured `systemd` source in **Admin > Data Sources**

## If Units Never Appear

- Confirm the node reports the `systemd` capability.
- Confirm the configured unit names are normalized as expected.
- Confirm the journal is readable by the agent runtime identity.
