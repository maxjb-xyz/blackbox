---
title: Project Structure
---

# Project Structure

Blackbox is organized as a small monorepo.

## Major Directories

- `server/` - Go server, database, auth, handlers, incident engine, embedded UI
- `agent/` - Go agent, local watchers, queueing, push behavior
- `web/` - React UI source for the embedded application
- `shared/` - shared Go packages and types
- `docs/` - documentation assets, internal specs, and the Docusaurus site
- `demo/` - demo deployment support

## Useful Mental Model

- If the behavior is about storing, correlating, or exposing data, start in
  `server/`.
- If the behavior is about watching a node locally, start in `agent/`.
- If the behavior is about presentation, navigation, or admin UX, start in
  `web/`.
