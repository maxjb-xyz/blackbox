---
title: Docker
---

# Docker

Docker is a virtual built-in agent-scoped source. It requires no setup in the admin UI — every agent with access to the Docker socket produces Docker events automatically.

## Requirements

The agent container must have access to the Docker socket:

```yaml
volumes:
  - /var/run/docker.sock:/var/run/docker.sock:ro
```

## What It Captures

Blackbox watches container and image events from the Docker API:

| Event | Description |
|-------|-------------|
| `start` | Container started |
| `stop` | Container stopped (clean or with non-zero exit code) |
| `die` | Container exited (may be collapsed into `stop` or `restart`) |
| `restart` | Stop + die + start sequence detected within 15 seconds |
| `create` | Container created |
| `pull` | Image pulled |
| `delete` | Image deleted |

## Restart Collapsing

When a container stops and starts again within 15 seconds, the separate stop and start events are collapsed into a single `restart` entry. The stop entry is replaced in-place so the timeline remains clean. Restart entries include exit codes from the original `die` event.

## Log Capture

On `stop` and `restart` events, the agent captures up to the last 50 lines from the container's stdout and stderr. These lines are stored in `metadata.log_snippet` and surface in the timeline detail view.

## Service Name Inference

Blackbox tries to extract a meaningful service name from Docker event attributes in this order:

1. `com.docker.compose.project` label — used as the service name
2. `com.docker.swarm.service.name` label — normalized against the stack namespace
3. Sanitized container name — swarm replicas and generated prefixes are stripped

The display name additionally uses `com.docker.compose.service` when available, shown as `project · service`.

## Exclusions

To exclude containers or stacks from the timeline, go to **Admin > Data Sources**, open the Docker source for a node, and add exclusion rules. Exclusions are enforced server-side — events are received and then discarded before storage.

## Why Docker Is Virtual

Docker has no `DataSourceInstance` row in the database. It exists on every capable agent automatically and cannot be created or deleted via the catalog. The admin UI shows it with a "Built-in" badge.
