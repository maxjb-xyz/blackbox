export type SourceCardColors = {
  border: string
  bg: string
  text: string
  topBar: string
}

export type SourceVisualType =
  | 'docker'
  | 'systemd'
  | 'filewatcher'
  | 'webhook_uptime_kuma'
  | 'webhook_watchtower'
  | 'webhook_komodo'
  | 'fallback'

export type SourceIconName =
  | 'docker'
  | 'uptime-kuma'
  | 'watchtower'
  | 'komodo'
  | 'systemd'
  | 'filewatcher'
  | 'fallback'

export type SourceIconSpec =
  | { kind: 'brand'; name: Extract<SourceIconName, 'docker' | 'uptime-kuma' | 'watchtower' | 'komodo'> }
  | { kind: 'generic'; name: Extract<SourceIconName, 'systemd' | 'filewatcher' | 'fallback'> }

export const COLORS = {
  fallback: { border: '#222', bg: '#111', text: '#666', accent: '#666' },
  docker: { border: '#1a3a5a', bg: '#0d1e2e', text: '#3a7abd', accent: '#1a4a7a' },
  systemd: { border: '#3a2a5a', bg: '#1a1228', text: '#7a5abd', accent: '#4a3a7a' },
  filewatcher: { border: '#5a3a1a', bg: '#281a0d', text: '#bd7a3a', accent: '#7a4a1a' },
  uptimeKuma: { border: '#1a5a3a', bg: '#0d2818', text: '#3abd7a', accent: '#1a7a4a' },
  watchtower: { border: '#24516a', bg: '#0d1f2b', text: '#57b8d9', accent: '#2f7398' },
  komodo: { border: '#1a4a2e', bg: '#0a1f14', text: '#2dbd72', accent: '#1a7a45' },
} as const

const FALLBACK_COLORS: SourceCardColors = {
  border: COLORS.fallback.border,
  bg: COLORS.fallback.bg,
  text: COLORS.fallback.text,
  topBar: 'none',
}

const SOURCE_CARD_COLORS: Record<SourceVisualType, SourceCardColors> = {
  docker: {
    border: COLORS.docker.border,
    bg: COLORS.docker.bg,
    text: COLORS.docker.text,
    topBar: `linear-gradient(90deg,${COLORS.docker.accent},transparent 60%)`,
  },
  systemd: {
    border: COLORS.systemd.border,
    bg: COLORS.systemd.bg,
    text: COLORS.systemd.text,
    topBar: `linear-gradient(90deg,${COLORS.systemd.accent},transparent 60%)`,
  },
  filewatcher: {
    border: COLORS.filewatcher.border,
    bg: COLORS.filewatcher.bg,
    text: COLORS.filewatcher.text,
    topBar: `linear-gradient(90deg,${COLORS.filewatcher.accent},transparent 60%)`,
  },
  webhook_uptime_kuma: {
    border: COLORS.uptimeKuma.border,
    bg: COLORS.uptimeKuma.bg,
    text: COLORS.uptimeKuma.text,
    topBar: `linear-gradient(90deg,${COLORS.uptimeKuma.accent},transparent 60%)`,
  },
  webhook_watchtower: {
    border: COLORS.watchtower.border,
    bg: COLORS.watchtower.bg,
    text: COLORS.watchtower.text,
    topBar: `linear-gradient(90deg,${COLORS.watchtower.accent},transparent 60%)`,
  },
  webhook_komodo: {
    border: COLORS.komodo.border,
    bg: COLORS.komodo.bg,
    text: COLORS.komodo.text,
    topBar: `linear-gradient(90deg,${COLORS.komodo.accent},transparent 60%)`,
  },
  fallback: FALLBACK_COLORS,
}

const SOURCE_ICON_SPECS: Record<SourceVisualType, SourceIconSpec> = {
  docker: { kind: 'brand', name: 'docker' },
  systemd: { kind: 'generic', name: 'systemd' },
  filewatcher: { kind: 'generic', name: 'filewatcher' },
  webhook_uptime_kuma: { kind: 'brand', name: 'uptime-kuma' },
  webhook_watchtower: { kind: 'brand', name: 'watchtower' },
  webhook_komodo: { kind: 'brand', name: 'komodo' },
  fallback: { kind: 'generic', name: 'fallback' },
}

function asSourceVisualType(type: string | null | undefined): SourceVisualType {
  if (typeof type !== 'string') return 'fallback'
  if (Object.prototype.hasOwnProperty.call(SOURCE_CARD_COLORS, type)) return type as SourceVisualType
  return 'fallback'
}

export function getSourceCardColors(type: string | null | undefined): SourceCardColors {
  return SOURCE_CARD_COLORS[asSourceVisualType(type)]
}

export function getSourceIconSpec(type: string | null | undefined): SourceIconSpec {
  return SOURCE_ICON_SPECS[asSourceVisualType(type)]
}
