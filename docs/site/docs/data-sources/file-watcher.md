---
title: File Watcher
---

# File Watcher

The file watcher is an agent-scoped source that watches configuration directories for file changes and records diffs on the timeline.

## Setup

1. Configure `WATCH_PATHS` on the agent (see below).
2. In **Admin > Data Sources**, open the node and click **Add Source**.
3. Select **File Watcher**. The only config option is **Redact Secrets** — enable it unless you specifically need raw values in diffs.
4. Save. The agent polls for its updated config and activates within one heartbeat cycle.

## Agent Environment Variables

### `WATCH_PATHS`

Colon-separated list of **container-side** paths to watch. These must be the mount targets inside the agent container, not the original host paths.

```yaml
environment:
  - WATCH_PATHS=/config:/opt/stacks
volumes:
  - /home/user/config:/config:ro
  - /home/user/stacks:/opt/stacks:ro
```

Each path must be a directory. Inaccessible subtrees are skipped individually — they do not stop other roots from being watched.

### `WATCH_IGNORE`

Colon-separated list of glob patterns to exclude. Patterns are matched against both the file's basename and its full path.

```yaml
environment:
  - WATCH_IGNORE=*.tmp:*.log:.DS_Store
```

Default excludes that always apply regardless of `WATCH_IGNORE`: `.git`, `node_modules`, `cache`, `logs`.

## Tracked File Types

Only files with these extensions are tracked:

`.yaml` `.yml` `.conf` `.env` `.json` `.ini`

Files larger than 64 KB, binary files, and files containing null bytes are skipped. Their presence is noted in the entry metadata with a `after_state` field (`too_large`, `binary`).

## How Diffs Work

When a tracked file changes, the agent:

1. Debounces the event for 500 ms (rapid sequential writes produce one entry).
2. Reads the new content and compares it to the stored snapshot.
3. Computes a unified diff with 2 lines of context.
4. Stores the diff in `metadata.diff` alongside SHA-256 hashes of the before and after states.

If the file has no stored baseline (first time seen), the entry records `diff_status: no_baseline`. If content is identical after reading, it records `diff_status: unchanged` and is not stored.

Diffs are capped at 32 KB and 1600 total lines. Larger diffs are truncated.

## Secret Redaction

When **Redact Secrets** is enabled, any line whose key matches a sensitive pattern is replaced with `[REDACTED]` before the diff is stored. Matched key substrings include:

`password`, `passwd`, `secret`, `token`, `api_key`, `apikey`, `access_key`, `secret_key`, `client_secret`, `private_key`, `auth_token`, `bearer_token`

Keys are normalized (lowercased, hyphens and dots replaced with underscores) before matching. Comment lines and blank lines are never redacted.

Redaction applies to both the `before` and `after` sides of the diff, so secrets do not appear in either direction.

## Service Name Inference

The service name in a timeline entry is derived from the file's path:

- `/opt/stacks/myapp/config.yml` → `myapp`
- `/home/user/docker/myapp/compose.yml` → `myapp`
- `/etc/nginx/nginx.conf` → `nginx`
- A file directly inside a collection directory (e.g. `/opt/stacks/.env`) → falls back to the watch root path

Collection directories that are stripped as prefixes: `stacks`, `docker`, `containers`, `compose`, `apps`, `services`.

## Permissions

The agent needs read access to the files and traverse access to the directories. If you own the files, set `PUID` and `PGID` to your host user IDs. If another user owns them, use ACLs or group ownership with appropriate mode bits.

## Notes

- The file watcher capability is only advertised when `WATCH_PATHS` is non-empty. If the agent has no paths configured, the source appears as "unavailable" in the catalog.
- New subdirectories created after agent startup are automatically added to the watch list.
- Symbolic links to files are followed; symbolic links to directories are not recursed.

For troubleshooting details, see [Troubleshooting File Watcher](../operations/troubleshooting-file-watcher.md).
