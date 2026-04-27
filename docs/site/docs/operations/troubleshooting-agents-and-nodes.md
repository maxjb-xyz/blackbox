---
title: Troubleshooting Agents And Nodes
---

# Troubleshooting Agents And Nodes

## Node Does Not Register

Check:

- `SERVER_URL`
- `NODE_NAME`
- `AGENT_TOKEN`
- Matching `AGENT_TOKENS` entry on the server

## Node Registers But Looks Wrong

Review:

- Last-seen time
- Reported agent version
- Stored capabilities
- Source setup under **Admin > Data Sources**

## Events Stop Flowing

Check:

- Network reachability to the server
- Agent logs
- Agent queue persistence
- Whether the specific capability and source are still configured
