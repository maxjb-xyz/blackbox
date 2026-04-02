# Project: Blackbox
**Role:** Senior Full-Stack Engineer & Homelab Systems Expert
**Tech Stack:** Go (Backend), React (Vite/Tailwind/Lucide), SQLite (GORM), Docker

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

## Feature Priorities (V1)
- Automated Docker event logging (Start/Stop/Die).
- Inotify file watching for config deltas.
- Webhook ingestion for Uptime Kuma (Down/Up) and Watchtower (Updates).
- Chronological, searchable timeline with correlation logic.