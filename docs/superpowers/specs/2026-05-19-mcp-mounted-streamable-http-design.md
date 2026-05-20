# Mounted Streamable HTTP MCP Design

Date: 2026-05-19
Status: Approved for planning

## Summary

Blackbox will replace its separate-port MCP HTTP/SSE server with a mounted MCP
endpoint on the main server listener. The new public endpoint will live at
`/mcp`, use the current server-wide bearer token model, remain toggleable from
`Admin > System`, and return `503 Service Unavailable` when disabled.

This is a clean break from the current port-based design. Legacy `mcp_port`
settings will be ignored at runtime, and Blackbox will surface a one-time admin
warning explaining that MCP moved onto the main server at `/mcp`.

## Goals

- Move MCP under the main Blackbox server instead of binding a second port.
- Switch from the legacy HTTP+SSE transport to Streamable HTTP.
- Preserve the existing server-wide MCP bearer token workflow.
- Keep MCP enable/disable behavior in the admin UI.
- Show a copyable MCP endpoint URL in the admin UI.
- Document the transport and endpoint change clearly as a breaking change.

## Non-Goals

- Preserve backward compatibility with the old separate-port SSE endpoint.
- Introduce user-scoped auth, OAuth, or session auth for MCP.
- Keep `mcp_port` as an active runtime setting.
- Add unrelated MCP tools or expand MCP feature scope.

## Current State

Blackbox currently starts a separate MCP server that:

- validates and stores `mcp_port`
- binds its own TCP listener
- serves the legacy `mcp-go` SSE transport
- is documented as a separate externally exposed port

The main Blackbox Chi router does not currently mount MCP handlers.

## Proposed Architecture

### Transport

Blackbox will upgrade to a Streamable HTTP-capable `mcp-go` release and use its
mounted HTTP transport rather than managing a second listener.

The preferred implementation is:

1. keep the existing MCP tool definitions and business logic
2. replace the separate-listener manager lifecycle with a mounted MCP handler
3. mount that handler into the main server at `/mcp`

If the upgraded SDK cannot exactly match Blackbox's required public behavior,
Blackbox will keep `/mcp` as the public contract and add a small server-side
adapter around the SDK handler rather than owning the whole transport.

### Routing

The main server will expose MCP from the primary HTTP listener at `/mcp`.

Behavior:

- when enabled, authenticated MCP requests are served under `/mcp`
- when disabled, `/mcp` returns `503` with a JSON error payload
- no dedicated MCP TCP listener is created

### Auth

Blackbox will keep the existing server-wide MCP bearer token model:

- token is stored in app settings
- token is generated on first enable if absent
- token can be regenerated from the admin UI
- requests must send `Authorization: Bearer <token>`

This change affects routing and transport only, not auth semantics.

### Origin Validation

Because Streamable HTTP is mounted on the main server, Blackbox must add origin
validation to reduce browser-based abuse risk.

Behavior:

- requests with no `Origin` header are allowed
- requests with an `Origin` header must match the configured or inferred server
  origin
- mismatched origins are rejected before MCP processing

This keeps non-browser and server-to-server clients working while enforcing a
basic browser safety check.

## Settings Model

### Kept Settings

- `mcp_enabled`
- `mcp_auth_token`

### Legacy-Only Settings

- `mcp_port`

`mcp_port` remains readable for migration detection only. It has no runtime
effect after this change.

### Admin Config Contract

The admin config API will stop exposing an editable MCP port field. Instead, it
will expose MCP endpoint display data derived from:

1. stored `base_url`, when present
2. the current browser origin in the web UI, when `base_url` is empty

The UI will show a full copyable endpoint URL such as
`https://blackbox.example.com/mcp`.

## Admin UI Behavior

The MCP settings card in `Admin > System` will:

- keep the enable/disable toggle
- remove the port input
- keep token status and token regeneration controls
- show the derived MCP endpoint URL
- show whether MCP is effectively running
- show a one-time migration warning when legacy `mcp_port` data is detected

The migration warning is informational only. It explains that MCP moved from a
separate port to `/mcp` on the main server.

## Migration Behavior

### Runtime

- legacy `mcp_port` values are ignored
- MCP enablement depends only on `mcp_enabled` and `mcp_auth_token`

### User-Facing Migration Notice

On upgrade, when legacy port-based MCP configuration is detected, Blackbox will
surface a one-time admin warning. The warning must explain:

- MCP no longer listens on a dedicated port
- clients must be updated to target `/mcp`
- existing bearer token behavior is unchanged

The warning should be dismissible or otherwise tracked so it is not shown on
every page load forever.

## Protocol Contract

The public MCP contract for Blackbox after this change is:

- endpoint path: `/mcp`
- mounted on the main server listener
- bearer-token protected
- toggleable
- disabled state returns `503`

The implementation should align with current Streamable HTTP expectations and
use `/mcp` as the canonical server URL users configure in clients.

## Error Handling

Expected cases:

- disabled MCP: `503 Service Unavailable` with JSON error body
- missing or invalid bearer token: `401 Unauthorized`
- origin mismatch: request rejected before reaching MCP handler
- unsupported or invalid MCP requests: delegated to the transport handler's
  normal protocol error handling

The disabled and auth failure responses should stay predictable and concise so
they are easy to diagnose in external MCP clients.

## Testing Strategy

Follow test-driven development for all production changes.

### Backend Tests

Add tests for:

- mounted `/mcp` routing when enabled
- `503` response when MCP is disabled
- `401` response for missing or invalid bearer token
- origin validation rejecting mismatched origins
- legacy `mcp_port` ignored at runtime
- migration warning surfacing when legacy settings exist
- admin config shape no longer exposing editable port data
- token regeneration continuing to work

### Frontend Tests

Add tests for the MCP admin card:

- endpoint URL display
- absence of port field
- migration warning rendering
- continued token regeneration flow

## Documentation Changes

Update all MCP docs and product copy to reflect the new model:

- README MCP section
- integration docs for MCP setup
- reverse proxy guidance
- any examples that currently reference `/sse` or a dedicated MCP port

The docs must explicitly call this a breaking change for existing remote MCP
clients.

## Risks

### Dependency Upgrade Risk

Upgrading `mcp-go` may require adaptation to a different transport API or
request lifecycle.

Mitigation:

- isolate transport-specific code behind a narrow internal wrapper
- keep MCP tool registration unchanged where possible

### Migration Risk

Existing users may have clients still pointed at the old dedicated port.

Mitigation:

- show a one-time migration warning
- update docs and examples clearly

### Browser-Safety Risk

Mounting MCP onto the main server without origin checks increases exposure to
browser-based request abuse.

Mitigation:

- validate `Origin` when present
- keep bearer-token auth in place

## Implementation Boundaries

The implementation should stay focused on:

- transport migration
- route mounting
- settings cleanup
- admin UI updates
- migration warning
- tests and docs

Do not add new MCP capabilities or expand auth scope as part of this change.
