---
title: Troubleshooting File Watcher
---

# Troubleshooting File Watcher

## Common Problems

### `WATCH_PATHS` Does Not Match The Mount

The watched path must match the container-visible mount target, not the host
source path.

Example:

- Mount: `/srv/stacks:/watch/stacks:ro`
- Setting: `WATCH_PATHS=/watch/stacks`

### The Agent Cannot Traverse Or Read Files

If the agent cannot traverse directories or read files, it will skip those
subtrees. Confirm permissions, `PUID`, and `PGID`.

### Expected Churn Is Too Noisy

Use `WATCH_IGNORE` to suppress known-noisy paths after basic access is working.

## Useful Log Clues

- `failed to register root ...`
- Registration counts for watched directories
- Warnings about unreadable paths
