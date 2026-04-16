import type { CSSProperties, ReactNode } from 'react'

const barStyle: CSSProperties = {
  padding: '24px 24px 18px',
  borderBottom: '1px solid var(--border)',
  display: 'flex',
  alignItems: 'baseline',
  justifyContent: 'space-between',
  gap: 12,
  flexWrap: 'wrap',
}

const titleStyle: CSSProperties = {
  fontSize: '18px',
  fontWeight: 700,
  letterSpacing: '0.12em',
  color: '#F0F0F0',
}

const subtitleStyle: CSSProperties = {
  fontSize: '12px',
  color: 'var(--muted)',
  letterSpacing: '0.08em',
}

export default function PageHeader({
  title,
  subtitle,
  titleActions,
  actions,
}: {
  title: ReactNode
  subtitle?: ReactNode
  titleActions?: ReactNode
  actions?: ReactNode
}) {
  return (
    <div className="page-header" style={barStyle}>
      <div className="page-header-main">
        <div className="page-header-title-row">
          <span className="page-header-title" style={titleStyle}>{title}</span>
          {titleActions ? <div className="page-header-title-actions">{titleActions}</div> : null}
        </div>
        {subtitle && <span className="page-header-subtitle" style={subtitleStyle}>{subtitle}</span>}
      </div>
      {actions ? (
        <div className="page-header-actions" style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}>
          {actions}
        </div>
      ) : null}
    </div>
  )
}
