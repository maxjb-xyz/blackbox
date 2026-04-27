---
title: File Watcher
---

# File Watcher

The file watcher is an agent-scoped source that watches configuration paths for
changes.

## What It Watches

Blackbox focuses on common homelab config formats:

- `.yaml`
- `.yml`
- `.conf`
- `.env`
- `.json`
- `.ini`

## Core Agent Settings

- `WATCH_PATHS` controls which mounted container-visible paths are watched.
- `WATCH_IGNORE` excludes specific patterns inside those paths.

## Source Config

The file watcher source currently requires a `redact_secrets` boolean in its
stored config.

## Permissions

The agent needs:

- Traverse access to directories.
- Read access to files it should diff.

If you own the files, set `PUID` and `PGID` to your host user IDs.

If another user owns them, either:

- Grant ACL-based read and traverse access.
- Or align group ownership and mode bits.

## Notes

- `WATCH_PATHS` must match the container-side mount target, not the original
  host path.
- Inaccessible subtrees are skipped instead of breaking the whole root.
- File diffs are bounded and optionally redacted before upload.

For troubleshooting details, see
[Troubleshooting File Watcher](../operations/troubleshooting-file-watcher.md).
