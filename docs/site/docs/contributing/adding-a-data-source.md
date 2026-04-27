---
title: Adding A Data Source
---

# Adding A Data Source

This is one of the most important contributor workflows in Blackbox.

## Understand The Current Model First

Today, built-in source types are defined centrally and include:

- A stable type string
- A scope: `agent` or `server`
- Singleton behavior
- A human-readable name and description
- A mechanism label used by the catalog UI

Current built-ins include:

- `docker`
- `systemd`
- `filewatcher`
- `webhook_uptime_kuma`
- `webhook_watchtower`

## Design Questions To Answer

Before writing code, decide:

- Is the source agent-scoped or server-scoped?
- Is it a singleton per target?
- Does it require secrets?
- What config shape should be stored?
- What capability, if any, should gate it?
- What event shapes will it emit?
- How should incidents use its evidence?

## Implementation Areas

### 1. Register The Source Type

Add the new built-in type definition alongside the existing source catalog in
the server handlers.

### 2. Decide Scope And Singleton Rules

If the type should only exist once per node or once per server, make sure the
model and handler behavior reflect that.

### 3. Add Config Validation

Extend source config validation so bad payloads fail clearly.

Questions to cover:

- Required keys
- Allowed types
- Secret handling
- Normalization rules

### 4. Add Redaction Rules If Needed

If the source stores credentials, tokens, or secrets, add redaction support so
the UI never receives raw secret values back.

### 5. Implement The Ingestion Path

Choose the correct side:

- Agent-side watcher or collector for node-local sources
- Server-side webhook or poll path for central sources

### 6. Emit Normalized Entries

New source entries should fit the timeline shape well:

- Stable `source`
- Useful `service`
- Useful `event`
- Clear `content`
- Structured `metadata`

### 7. Add Tests

At minimum, cover:

- Source type listing
- Create and update validation
- Secret preservation or redaction if used
- Ingestion behavior
- Incident or timeline side effects when relevant

### 8. Update Docs

Add or update:

- A user-facing page under `Data Sources`
- Any deployment or configuration pages touched by the change
- This contributor guide if the source workflow itself changed

## Practical Advice

- Prefer a source model that operators can explain in one sentence.
- Keep config small and explicit.
- Reuse existing event shapes when possible instead of inventing completely new
  semantics.
- Document capability expectations early, especially for agent-scoped sources.
