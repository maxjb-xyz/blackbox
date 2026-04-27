---
title: Agent Tokens
---

# Agent Tokens

The server authenticates agents using a node-name to token mapping.

## Server-Side Format

`AGENT_TOKENS` accepts comma-separated or newline-separated pairs:

```bash
AGENT_TOKENS="node-01=token1,node-02=token2,nas=token3"
```

## Agent-Side Match

Each agent must send:

- `NODE_NAME` matching one configured key.
- `AGENT_TOKEN` matching the corresponding value.

## Good Practices

- Use one unique token per node.
- Rotate tokens if a machine is reprovisioned or exposed.
- Keep names stable so node history remains understandable.

## Generate Strong Tokens

```bash
openssl rand -hex 32
```
