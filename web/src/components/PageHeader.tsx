import type { CSSProperties, ReactNode } from 'react'

const barStyle: CSSProperties = {
  padding: '18px 24px',
  borderBottom: '1px solid var(--border)',
  background: 'var(--surface)',
  display: 'flex',
  alignItems: 'center',
  justifyContent: 'space-between',
  gap: 12,
  flexWrap: 'wrap',
}

const titleStyle: CSSProperties = {
  color: 'var(--muted)',
  fontSize: '12px',
  letterSpacing: '0.1em',
}

export default function PageHeader({ title, actions }: { title: ReactNode; actions?: ReactNode }) {
  return (
    <div style={barStyle}>
      <span style={titleStyle}>{title}</span>
      {actions ? <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}>{actions}</div> : null}
    </div>
  )
}
