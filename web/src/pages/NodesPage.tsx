import type { CSSProperties } from 'react'
import { useNodePulse } from '../components/NodePulse'
import PageHeader from '../components/PageHeader'
import { formatLocalTimestamp } from '../utils/time'

interface NodeViewModel {
  id: string
  name: string
  status: string
  lastSeen: string
  agentVersion: string
  ipAddress: string
  osInfo: string | null | undefined
}

function formatTimestamp(ts?: string | null | Date) {
  if (!ts) return '-'
  const date = new Date(ts as string)
  if (Number.isNaN(date.getTime())) return '-'
  return formatLocalTimestamp(date, { includeSeconds: true }) || '-'
}

function OsCell({ osInfo }: { osInfo: string | null | undefined }) {
  if (!osInfo || !osInfo.trim()) return <span style={{ color: 'var(--muted)' }}>-</span>

  const info = osInfo.trim()
  let isDocker = false
  let osName = info

  if (info.startsWith('docker / ')) {
    isDocker = true
    osName = info.slice('docker / '.length)
  } else if (info === 'docker') {
    isDocker = true
    osName = ''
  }

  const pillStyle: CSSProperties = {
    display: 'inline-block',
    fontSize: 10,
    padding: '1px 6px',
    letterSpacing: '0.08em',
    verticalAlign: 'middle',
  }

  return (
    <span style={{ display: 'inline-flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}>
      {isDocker && (
        <span style={{ ...pillStyle, color: 'var(--info)', border: '1px solid var(--info)' }}>
          DOCKER
        </span>
      )}
      {osName && (
        <span style={{ color: 'var(--text)', fontSize: 12 }}>{osName}</span>
      )}
    </span>
  )
}

function getNodeStatusPresentation(status: string) {
  const isOnline = status === 'online'
  return {
    color: isOnline ? 'var(--success)' : 'var(--danger)',
    background: isOnline ? undefined : 'var(--danger-bg)',
    label: status.toUpperCase(),
    isOnline,
  }
}

function toNodeViewModel(node: ReturnType<typeof useNodePulse>['nodes'][number]): NodeViewModel {
  return {
    id: node.id,
    name: node.name,
    status: node.status,
    lastSeen: formatTimestamp(node.last_seen),
    agentVersion: node.agent_version || '-',
    ipAddress: node.ip_address || '-',
    osInfo: node.os_info,
  }
}

function NodeItem({ node, variant }: { node: NodeViewModel; variant: 'desktop' | 'mobile' }) {
  const status = getNodeStatusPresentation(node.status)

  if (variant === 'mobile') {
    return (
      <div
        className="nodes-mobile-card"
        style={{
          borderLeft: `3px solid ${status.color}`,
          background: status.isOnline ? 'var(--surface)' : 'var(--danger-bg)',
        }}
      >
        <div className="nodes-mobile-head">
          <div className="nodes-mobile-name">{node.name}</div>
          <div className="nodes-mobile-status" style={{ color: status.color }}>
            <span
              className="nodes-status-dot"
              aria-hidden="true"
              style={{ background: status.color }}
            />
            {status.label}
          </div>
        </div>

        <div className="nodes-mobile-grid">
          <div className="nodes-mobile-field">
            <span className="nodes-mobile-label">LAST SEEN</span>
            <span className="nodes-mobile-value">{node.lastSeen}</span>
          </div>
          <div className="nodes-mobile-field">
            <span className="nodes-mobile-label">VERSION</span>
            <span className="nodes-mobile-value">{node.agentVersion}</span>
          </div>
          <div className="nodes-mobile-field">
            <span className="nodes-mobile-label">OS</span>
            <span className="nodes-mobile-value"><OsCell osInfo={node.osInfo} /></span>
          </div>
          <div className="nodes-mobile-field">
            <span className="nodes-mobile-label">IP</span>
            <span className="nodes-mobile-value">{node.ipAddress}</span>
          </div>
        </div>
      </div>
    )
  }

  return (
    <tr style={status.background ? { background: status.background } : undefined}>
      <td className="nodes-cell-truncate" style={{ color: 'var(--text)' }}>
        {node.name}
      </td>
      <td style={{ fontSize: 11, letterSpacing: '0.1em', color: status.color }}>
        <span style={{ display: 'inline-flex', alignItems: 'center', gap: 8 }}>
          <span
            className="nodes-status-dot"
            aria-hidden="true"
            style={{ background: status.color }}
          />
          <span>{status.label}</span>
        </span>
      </td>
      <td style={{ color: 'var(--muted)', fontSize: 12 }}>
        {node.lastSeen}
      </td>
      <td>
        <OsCell osInfo={node.osInfo} />
      </td>
      <td style={{ color: 'var(--muted)', fontSize: 11 }}>
        {node.agentVersion}
      </td>
      <td style={{ color: 'var(--muted)', fontSize: 12 }}>
        {node.ipAddress}
      </td>
    </tr>
  )
}

export default function NodesPage() {
  const { nodes, loading, error, lastUpdated } = useNodePulse()
  const nodeItems = nodes.map(toNodeViewModel)

  const statusLine = (() => {
    if (loading && !lastUpdated) return 'checking agent registry...'
    if (!loading && error && !lastUpdated) return 'failed to load agent registry'
    if (error && lastUpdated) return `showing cached data from ${formatTimestamp(lastUpdated)}`
    if (lastUpdated) return `last updated ${formatTimestamp(lastUpdated)}`
    return null
  })()

  return (
    <div>
      <PageHeader title="NODES" subtitle="agent registry" />

      <div className="nodes-page-body" style={{ padding: '0 24px 48px' }}>
        {statusLine && (
          <div
            className="nodes-status-line"
            style={{
              padding: '10px 0 16px',
              fontSize: 11,
              color: 'var(--muted)',
              letterSpacing: '0.08em',
              borderBottom: '1px solid #1A1A1A',
              marginBottom: 20,
            }}
          >
            {statusLine}
          </div>
        )}

        {nodes.length === 0 ? (
          <div
            style={{
              padding: '48px 0',
              color: 'var(--muted)',
              fontSize: 12,
              textAlign: 'center',
              border: '1px dashed #1a1a1a',
            }}
          >
            {error && !lastUpdated
              ? 'failed to load agent registry'
              : loading
                ? 'checking agent registry...'
                : 'no agents registered'}
          </div>
        ) : (
          <>
            <div className="nodes-table-scroll nodes-desktop-table">
              <table className="nodes-table">
                <thead>
                  <tr>
                    <th scope="col">NAME</th>
                    <th scope="col">STATUS</th>
                    <th scope="col">LAST SEEN</th>
                    <th scope="col">OS</th>
                    <th scope="col">VERSION</th>
                    <th scope="col">IP</th>
                  </tr>
                </thead>
                <tbody>
                  {nodeItems.map(node => (
                    <NodeItem key={node.id} node={node} variant="desktop" />
                  ))}
                </tbody>
              </table>
            </div>

            <div className="nodes-mobile-list">
              {nodeItems.map(node => (
                <NodeItem key={`${node.id}-mobile`} node={node} variant="mobile" />
              ))}
            </div>
          </>
        )}
      </div>
    </div>
  )
}
