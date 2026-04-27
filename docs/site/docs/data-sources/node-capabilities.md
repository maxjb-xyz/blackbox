---
title: Node Capabilities
---

# Node Capabilities

Node capabilities describe what an agent on a given machine can actually do.

## Why They Matter

Capabilities help Blackbox avoid treating every node as identical. They power:

- Source setup defaults.
- Node-specific source availability in the admin UI.
- Better operator expectations about what a node should emit.

## Current Capability Areas

The current built-in model is centered on:

- `docker`
- `systemd`
- `filewatcher`

## How They Are Reported

Agents report capabilities to the server, and the server stores them with node
metadata. Some legacy fallback behavior exists for nodes that do not yet have a
stored capability set.

## Operational Use

When debugging a missing source or empty event stream, node capabilities are one
of the first things to inspect.
