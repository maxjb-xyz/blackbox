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

- `docker` — virtual agent source (no DB row, always present)
- `systemd` — agent-scoped, requires `WATCH_SYSTEMD=true` on the agent
- `filewatcher` — agent-scoped, requires `WATCH_PATHS` configured on the agent
- `webhook_uptime_kuma` — server-scoped webhook source
- `webhook_watchtower` — server-scoped webhook source

## Design Questions To Answer

Before writing code, decide:

- Is the source agent-scoped or server-scoped?
- Is it a singleton per target?
- Does it require secrets?
- What config shape should be stored?
- What capability string, if any, should gate it?
- What event shapes will it emit?
- How should incidents use its evidence?

## Touchpoint Map

Every source type touches at least six locations across the codebase. Use this as a checklist.

### Backend

| File | What to change |
|------|----------------|
| `server/internal/handlers/admin_sources.go` | Add entry to `knownSourceTypes`; add config validation case in `validateSourceConfig`; add redaction key in `sensitiveKeysFor` if storing secrets |
| `server/internal/models/data_source.go` | Add the type string to `agentScopedSingletonSourceTypes` or `serverScopedSingletonSourceTypes` |
| `server/internal/handlers/datasource_migration.go` | Add startup seeding logic if the source should be auto-created on first run or migrated from legacy config |
| `server/main.go` | For webhook sources: add a `PrimeWebhookSecretCache` call at startup and a route using `middleware.WebhookAuthFunc` |

### Frontend

| File | What to change |
|------|----------------|
| `web/src/components/sourceIcons.ts` | Add the type to `SourceVisualType`, `SourceIconName`, `SOURCE_CARD_COLORS`, and `SOURCE_ICON_SPECS` |
| `web/src/components/SourceIcon.tsx` | Add a render branch for the new icon name; brand icons need a custom SVG component, generic icons need a Lucide mapping |
| `web/src/pages/SourceCatalog.tsx` | Add a `buildDefaultConfig` case so the catalog pre-fills config when the user clicks Add |
| `web/src/pages/DataSourcesGroup.tsx` | Add an entry to `DOCS_URLS` so the edit panel shows a link to the setup guide; without this the banner renders nothing |

## Three Flows In Detail

### Flow 1: Agent-Scoped Source (e.g. `systemd`, `filewatcher`)

Agent-scoped sources are collected by the agent process and pushed to the server. The UI only offers them when the agent advertises the capability.

**Capability handshake.** The agent builds its capability list in `agent/main.go` in `buildCapabilities`. Add your capability string there so the agent advertises it on heartbeat. The server stores capabilities on the `Node` row. `SourceCatalog.tsx` filters the add-source dialog at render time: types not in `caps` are shown as "unavailable" until the agent is updated. If you skip this step, the source will appear greyed out on every node even when the agent is running it.

**Config flow for agent-scoped sources:**

1. Add the type to `knownSourceTypes` in `admin_sources.go` with `Scope: "agent"`.
2. Add the type string to `agentScopedSingletonSourceTypes` in `data_source.go`.
3. Add validation in `validateSourceConfig`. If config needs normalization (like `systemd`'s unit-name appending), do it there so both create and update paths benefit.
4. Add a `buildDefaultConfig` case in `SourceCatalog.tsx` so the catalog dialog pre-fills the form.
5. Add source icon and colors in `sourceIcons.ts`.
6. Implement the agent-side watcher or collector and push events via `/api/agent/push`.

**Startup seeding.** If the source should be auto-created for every capable node on first startup (like `filewatcher`), add logic in `datasource_migration.go`. The migration runs inside a transaction on every startup and is gated by the `data_sources_migrated` app setting key so it only seeds once.

### Flow 2: Server-Side Webhook Source (e.g. `webhook_uptime_kuma`, `webhook_watchtower`)

Webhook sources receive events over HTTP rather than from an agent. They require a secret for authentication.

**Config and validation.** Types with the `webhook_` prefix automatically get secret validation (non-empty string required) and redaction (zeroed before the config is returned to the UI) because `validateSourceConfig` and `sensitiveKeysFor` both key off that prefix. New webhook types get both behaviors for free if they follow the naming convention.

**Route wiring in `server/main.go`.** Two things must happen:

1. A `PrimeWebhookSecretCache` call at startup (alongside the existing calls around line 96) so the secret is hot before the first inbound request.
2. A route using `middleware.WebhookAuthFunc` wrapping `handlers.GetCachedWebhookSecret` for your source type (see the pattern around lines 282–294). The cache invalidates automatically when the source is created, updated, or deleted via the admin API.

**Handler.** Add a handler function (e.g. `WebhookMyService`) that parses the inbound payload and emits normalized timeline entries via `eventHub`.

**Startup seeding.** Webhook sources are only auto-seeded when `WEBHOOK_SECRET` was set at startup (legacy behavior). New installs create them explicitly via the catalog. Add a seeding block in `datasource_migration.go` only if you need parity with the existing webhook sources.

### Flow 3: Virtual Source (e.g. `docker`)

Virtual sources have no `DataSourceInstance` row. They are always present on capable nodes and cannot be created or deleted via the catalog UI.

- Add to `knownSourceTypes` as normal so the catalog shows the card.
- Block creation in `CreateSource` with a guard like the existing `docker` check.
- Keep the type in `agentScopedSingletonSourceTypes` for `IsSingletonSourceType` compatibility.
- The catalog marks it as `virtual` and renders a "Built-in" badge instead of an Add button.

## Implementation Steps

### 1. Register The Source Type

Add the entry to `knownSourceTypes` in `server/internal/handlers/admin_sources.go`. The `Type` string becomes the stable identifier used everywhere.

### 2. Decide Scope And Singleton Rules

Add the type string to the correct slice in `server/internal/models/data_source.go` (`agentScopedSingletonSourceTypes` or `serverScopedSingletonSourceTypes`). The handler uses these slices to enforce singleton constraints at create time.

### 3. Add Config Validation

Add a case to `validateSourceConfig` in `admin_sources.go`. Cover:

- Required keys
- Allowed types
- Normalization rules (e.g. trimming, deduplication)

### 4. Add Redaction Rules If Needed

If the source stores credentials or tokens, add the key name to `sensitiveKeysFor` in `admin_sources.go`. Redacted fields are zeroed before the config is returned to the UI. The `mergeSourceConfig` function also uses this list to preserve existing secret values when a client sends a blank field on update.

### 5. Implement The Ingestion Path

Choose the correct side:

- Agent-side watcher or collector for node-local sources; push via `/api/agent/push`
- Server-side webhook handler for central sources; wire the route in `server/main.go`

For webhook sources, wire `PrimeWebhookSecretCache` at startup and add a route with `middleware.WebhookAuthFunc` as described in Flow 2.

### 6. Add Capability Advertising (Agent-Scoped Only)

Update `buildCapabilities` in `agent/main.go` to include your capability string when the source is active. Without this, the UI shows the source as unavailable on all nodes even when the agent is running it.

### 7. Emit Normalized Entries

New source entries should fit the timeline shape well:

- Stable `source`
- Useful `service`
- Useful `event`
- Clear `content`
- Structured `metadata`

### 8. Update The Frontend

In `web/src/components/sourceIcons.ts`:

- Add the type to `SourceVisualType` and `SourceIconName`
- Add a color entry in `COLORS` and `SOURCE_CARD_COLORS`
- Add an icon spec in `SOURCE_ICON_SPECS` — use `kind: 'brand'` for a custom SVG, `kind: 'generic'` for a Lucide icon

In `web/src/components/SourceIcon.tsx`:

- If `kind: 'brand'`, add an `if (spec.name === '...')` branch with the SVG component (see `DockerMark`, `UptimeKumaMark`, `WatchtowerMark` for examples). Without this branch, the icon falls back to a `CircleHelp` at runtime.
- If `kind: 'generic'`, add an `if (spec.name === '...')` branch with the Lucide icon component if you added a new `SourceIconName`. Existing Lucide-backed names (`systemd` → `Cog`, `filewatcher` → `FileSearch`) don't need a new branch.

In `web/src/pages/SourceCatalog.tsx`:

- Add a `buildDefaultConfig` case returning the pre-filled config shape for your type

In `web/src/pages/DataSourcesGroup.tsx`:

- Add an entry to `DOCS_URLS` mapping your type string to the full URL of its docs page. The edit panel banner renders nothing if the entry is missing — no error, just no link.

### 9. Handle Startup Seeding And Migration

If the source should be auto-created on first run, add logic in `server/internal/handlers/datasource_migration.go`. The function runs inside a transaction and is safe to call on every startup. It uses the `data_sources_migrated` app setting key to ensure seeding only happens once.

### 10. Add Tests

At minimum, cover:

- Source type listing
- Create and update validation
- Secret preservation or redaction if used
- Ingestion behavior
- Incident or timeline side effects when relevant

### 11. Update Docs

Add or update:

- A user-facing page under `Data Sources`
- Any deployment or configuration pages touched by the change
- This contributor guide if the source workflow itself changed

## Practical Advice

- Prefer a source model that operators can explain in one sentence.
- Keep config small and explicit.
- Reuse existing event shapes when possible instead of inventing completely new semantics.
- The `webhook_` prefix convention gives you secret validation and redaction for free — use it for any HTTP-inbound source.
- If your agent-scoped source needs an env-var gate (like `WATCH_SYSTEMD`), add the check in `buildCapabilities` so the UI reflects availability correctly.
- Document capability expectations early. The capability handshake is easy to miss and causes confusing "unavailable" states in the UI.
