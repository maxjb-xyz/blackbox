# Project: Blackbox
**Architecture:** Distributed (Server + Agent)
**Tech Stack:** Go (Backend/Agent), React (Vite/Tailwind/Lucide), SQLite (GORM), Docker

## Core Philosophy
1. **The Forensic Timeline:** Every event (Docker, File change, Webhook) must be timestamped and correlated.
2. **The "Discipline" Fix:** Automation is the primary contributor. Manual notes are for "Why," not "What."
3. **The 2 AM Rule:** High density, low eye strain (Dark Mode), and zero-friction navigation for stressful troubleshooting.

## Technical Standards
- **Backend:** Go (standard net/http or Fiber). 
- **Database:** SQLite for portability. Store in `/data/lablog.db`.
- **Ingestion:** 
    - Docker: Watch `/var/run/docker.sock` for container events.
    - Files: Use `fsnotify` for config directory watching.
    - Webhooks: Generic JSON listeners for Watchtower and Uptime Kuma.
- **Frontend:** React + Tailwind. 
  - **Aesthetic:** Zerobyte (Dark #0B0B0B, Monospace fonts, tight padding, sharp borders, no rounded corners).
  - **Icons:** Lucide-react (14px-16px).

## Component Breakdown
1. **Blackbox Server (The Brain):**
   - Handles the React UI (embedded).
   - Manages the SQLite database and Auth (Local + OIDC).
   - Receives events from Agents via a protected API.
   - Handles external Webhooks (Uptime Kuma/Watchtower).

2. **Blackbox Agent (The Eyes):**
   - Lightweight Go binary/container.
   - Watches `/var/run/docker.sock` and local `WATCH_PATHS` (inotify).
   - Pushes events to the Server using a shared Secret/Token.

## Feature Priorities (V1)
- Automated Docker event logging (Start/Stop/Die).
- Inotify file watching for config deltas.
- Webhook ingestion for Uptime Kuma (Down/Up) and Watchtower (Updates).
- Chronological, searchable timeline with correlation logic.
- Server/Agent communication via gRPC or REST.
- Admin Bootstrap + OIDC on the Server.
- Real-time event streaming from Agent to Server.
