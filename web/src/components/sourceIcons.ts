export type SourceCardColors = {
  border: string
  bg: string
  text: string
  topBar: string
}

export type SourceIconSpec =
  | { kind: 'brand'; name: 'docker' | 'uptime-kuma' | 'watchtower' }
  | { kind: 'generic'; name: 'systemd' | 'filewatcher' | 'fallback' }

const FALLBACK_COLORS: SourceCardColors = {
  border: '#222',
  bg: '#111',
  text: '#666',
  topBar: 'none',
}

const SOURCE_CARD_COLORS: Record<string, SourceCardColors> = {
  docker:              { border: '#1a3a5a', bg: '#0d1e2e', text: '#3a7abd', topBar: 'linear-gradient(90deg,#1a4a7a,transparent 60%)' },
  systemd:             { border: '#3a2a5a', bg: '#1a1228', text: '#7a5abd', topBar: 'linear-gradient(90deg,#4a3a7a,transparent 60%)' },
  filewatcher:         { border: '#5a3a1a', bg: '#281a0d', text: '#bd7a3a', topBar: 'linear-gradient(90deg,#7a4a1a,transparent 60%)' },
  webhook_uptime_kuma: { border: '#1a5a3a', bg: '#0d2818', text: '#3abd7a', topBar: 'linear-gradient(90deg,#1a7a4a,transparent 60%)' },
  webhook_watchtower:  { border: '#24516a', bg: '#0d1f2b', text: '#57b8d9', topBar: 'linear-gradient(90deg,#2f7398,transparent 60%)' },
}

const SOURCE_ICON_SPECS: Record<string, SourceIconSpec> = {
  docker: { kind: 'brand', name: 'docker' },
  systemd: { kind: 'generic', name: 'systemd' },
  filewatcher: { kind: 'generic', name: 'filewatcher' },
  webhook_uptime_kuma: { kind: 'brand', name: 'uptime-kuma' },
  webhook_watchtower: { kind: 'brand', name: 'watchtower' },
}

export function getSourceCardColors(type: string): SourceCardColors {
  return SOURCE_CARD_COLORS[type] ?? FALLBACK_COLORS
}

export function getSourceIconSpec(type: string): SourceIconSpec {
  return SOURCE_ICON_SPECS[type] ?? { kind: 'generic', name: 'fallback' }
}
