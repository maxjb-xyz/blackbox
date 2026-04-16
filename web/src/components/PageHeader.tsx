import type { ReactNode } from 'react'

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
    <div className="page-header">
      <div className="page-header-main">
        <div className="page-header-title-row">
          <span className="page-header-title">{title}</span>
          {titleActions ? <div className="page-header-title-actions">{titleActions}</div> : null}
        </div>
        {subtitle && <span className="page-header-subtitle">{subtitle}</span>}
      </div>
      {actions ? (
        <div className="page-header-actions">
          {actions}
        </div>
      ) : null}
    </div>
  )
}
