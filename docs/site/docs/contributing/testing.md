---
title: Testing
---

# Testing

Contributors should verify changes at the level they affect.

## Typical Commands

- Server tests: Go test commands inside `server/`
- Agent tests: Go test commands inside `agent/`
- Web checks: `npm` scripts inside `web/`
- Docs checks: build or preview checks inside `docs/site/`

## What To Cover

- New event normalization logic
- Source config validation rules
- Incident behavior changes
- UI flows affected by schema or API changes
- Docs updates when setup or contributor workflows change

## For Data Source Changes

Please include tests for:

- Source type registration
- Scope and singleton behavior
- Config validation
- Secret redaction behavior when applicable
- Agent or server ingestion path
