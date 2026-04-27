---
title: Correlation Model
---

# Correlation Model

Blackbox starts with deterministic correlation before any optional AI layer.

## Signals Used

- Source-specific timing windows
- Same-node preference
- Service name inference
- Systemd failure and restart patterns
- Docker exit and restart behavior
- Watchtower update evidence
- Captured log snippets

## What It Produces

The engine ranks likely causes and builds an event chain the UI can explain.

## Why This Matters

It keeps the core of Blackbox understandable and operationally useful even when
no AI integration is configured.
