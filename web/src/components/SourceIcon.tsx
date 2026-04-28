import type { CSSProperties } from 'react'
import { CircleHelp, Cog, FileSearch } from 'lucide-react'

import { getSourceIconSpec } from './sourceIcons'

interface Props {
  type: string
  size?: number
  strokeWidth?: number
  style?: CSSProperties
}

export default function SourceIcon({ type, size = 16, strokeWidth = 1.8, style }: Props) {
  const spec = getSourceIconSpec(type)

  if (spec.kind === 'brand') {
    if (spec.name === 'docker') return <DockerMark size={size} style={style} />
    if (spec.name === 'uptime-kuma') return <UptimeKumaMark size={size} style={style} />
    if (spec.name === 'watchtower') return <WatchtowerMark size={size} style={style} />
    if (spec.name === 'komodo') return <KomodoMark size={size} style={style} />
    return <CircleHelp size={size} strokeWidth={strokeWidth} style={style} />
  }

  if (spec.name === 'systemd') return <Cog size={size} strokeWidth={strokeWidth} style={style} />
  if (spec.name === 'filewatcher') return <FileSearch size={size} strokeWidth={strokeWidth} style={style} />
  return <CircleHelp size={size} strokeWidth={strokeWidth} style={style} />
}

function DockerMark({ size, style }: { size: number; style?: CSSProperties }) {
  return (
    <svg viewBox="0 0 24 24" width={size} height={size} aria-hidden="true" style={style}>
      <g fill="currentColor">
        <rect x="4" y="10" width="3" height="3" rx="0.5" />
        <rect x="7.75" y="10" width="3" height="3" rx="0.5" />
        <rect x="11.5" y="10" width="3" height="3" rx="0.5" />
        <rect x="7.75" y="6.25" width="3" height="3" rx="0.5" />
        <rect x="11.5" y="6.25" width="3" height="3" rx="0.5" />
        <path d="M4.2 14.5c0 2.9 2.34 5.25 5.24 5.25h4.1c3.08 0 5.62-2.17 6.18-5.07.76.28 1.57.16 2.08-.36.24-.24.4-.56.45-.93-.63-.28-1.37-.35-2.05-.16-.38-1.43-1.68-2.5-3.22-2.5h-1.68c-.42 0-.82.15-1.12.43l-.83.77H4.2v2.57Z" />
      </g>
    </svg>
  )
}

function UptimeKumaMark({ size, style }: { size: number; style?: CSSProperties }) {
  return (
    <svg viewBox="0 0 24 24" width={size} height={size} aria-hidden="true" style={style}>
      <rect x="4" y="4" width="16" height="16" rx="4" fill="none" stroke="currentColor" strokeWidth="1.8" />
      <circle cx="9" cy="10" r="1.1" fill="currentColor" />
      <circle cx="15" cy="10" r="1.1" fill="currentColor" />
      <path
        d="M7.4 15h2.05l1.28-2.35 2.06 4.1 1.34-2.32H16.6"
        fill="none"
        stroke="currentColor"
        strokeWidth="1.8"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  )
}

function WatchtowerMark({ size, style }: { size: number; style?: CSSProperties }) {
  return (
    <svg viewBox="0 0 24 24" width={size} height={size} aria-hidden="true" style={style}>
      <path
        d="M12 4.3 15.2 7v2.35l1.62 8.25H7.18L8.8 9.35V7L12 4.3Z"
        fill="none"
        stroke="currentColor"
        strokeWidth="1.8"
        strokeLinejoin="round"
      />
      <path
        d="M9.8 17.6V20h4.4v-2.4M10.3 11.2h3.4M10 8.6h4"
        fill="none"
        stroke="currentColor"
        strokeWidth="1.8"
        strokeLinecap="round"
      />
      <path
        d="M6.1 9.9c1.2-.9 2.4-1.35 3.6-1.35M17.9 9.9c-1.2-.9-2.4-1.35-3.6-1.35"
        fill="none"
        stroke="currentColor"
        strokeWidth="1.4"
        strokeLinecap="round"
        opacity="0.75"
      />
    </svg>
  )
}

function KomodoMark({ size, style }: { size: number; style?: CSSProperties }) {
  return (
    <svg viewBox="0 0 24 24" width={size} height={size} aria-hidden="true" style={style}>
      <rect x="3" y="3" width="18" height="18" rx="1" fill="none" stroke="currentColor" strokeWidth="1.8" />
      <path
        d="M8.5 7.5v9M8.5 12l3.5-4.5M8.5 12l4 4.5"
        fill="none"
        stroke="currentColor"
        strokeWidth="1.8"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  )
}
