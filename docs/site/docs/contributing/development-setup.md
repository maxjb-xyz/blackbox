---
title: Development Setup
---

# Development Setup

## Requirements

- Go `1.22+`
- Node.js `20+`
- Docker if you want to build images

## Build From Source

```bash
git clone https://github.com/maxjb-xyz/blackbox.git
cd blackbox

cd web && npm install && npm run build:server && cd ..
cd server && go build -o blackbox-server ./... && cd ..
cd agent && go build -o blackbox-agent ./... && cd ..
```

## Run Locally

```bash
TZ=America/New_York JWT_SECRET=dev AGENT_TOKENS="local=devtoken" WEBHOOK_SECRET=dev ./server/blackbox-server
```

In another terminal:

```bash
TZ=America/New_York SERVER_URL=http://localhost:8080 AGENT_TOKEN=devtoken NODE_NAME=local ./agent/blackbox-agent
```

## Local Systemd Testing

On Linux, add `WATCH_SYSTEMD=true`, make the journal readable, then configure
the node's systemd source after registration.
