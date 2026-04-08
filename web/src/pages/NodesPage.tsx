import { useNodePulse } from '../components/NodePulse'
import PageHeader from '../components/PageHeader'
import { formatLocalTimestamp } from '../utils/time'

function formatTimestamp(ts?: string | null | Date) {
  if (!ts) return '-'
  const date = new Date(ts as string)
  if (Number.isNaN(date.getTime())) return '-'
  return formatLocalTimestamp(date, { includeSeconds: true }) || '-'
}

function OsCell({ osInfo }: { osInfo: string | null | undefined }) {
  if (!osInfo) return <span style={{ color: 'var(--muted)' }}>-</span>

  // Try to split "linux/amd64 Ubuntu 24.04" into arch and tag
  const parts = osInfo.split(' ')
  const arch = parts[0]
  const tag = parts.slice(1).join(' ')

  return (
    <span>
      <span style={{ color: 'var(--text)' }}>{arch}</span>
      {tag && (
        <span
          style={{
            display: 'inline-block',
            fontSize: 10,
            color: 'var(--muted)',
            border: '1px solid var(--border)',
            padding: '1px 6px',
            letterSpacing: '0.08em',
            marginLeft: 8,
          }}
        >
          {tag}
        </span>
      )}
    </span>
  )
}

export default function NodesPage() {
  const { nodes, loading, error, lastUpdated } = useNodePulse()

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

      <div style={{ padding: '0 24px 48px' }}>
        {statusLine && (
          <div
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
          <table className="nodes-table">
            <thead>
              <tr>
                <th scope="col" />
                <th scope="col">NAME</th>
                <th scope="col">STATUS</th>
                <th scope="col">LAST SEEN</th>
                <th scope="col">OS</th>
                <th scope="col">VERSION</th>
                <th scope="col">IP</th>
              </tr>
            </thead>
            <tbody>
              {nodes.map(node => {
                const isOnline = node.status === 'online'
                return (
                  <tr key={node.id} style={{ opacity: isOnline ? 1 : 0.5 }}>
                    <td>
                      <span
                        className="nodes-status-dot"
                        style={{
                          background: isOnline ? 'var(--success)' : 'var(--danger)',
                        }}
                        role="img"
                        aria-label={node.status}
                      />
                    </td>
                    <td className="nodes-cell-truncate" style={{ color: 'var(--text)' }}>
                      {node.name}
                    </td>
                    <td style={{ fontSize: 11, letterSpacing: '0.1em', color: isOnline ? 'var(--success)' : 'var(--danger)' }}>
                      {node.status.toUpperCase()}
                    </td>
                    <td style={{ color: 'var(--muted)', fontSize: 12 }}>
                      {formatTimestamp(node.last_seen)}
                    </td>
                    <td>
                      <OsCell osInfo={node.os_info} />
                    </td>
                    <td style={{ color: 'var(--muted)', fontSize: 11 }}>
                      {node.agent_version || '-'}
                    </td>
                    <td style={{ color: 'var(--muted)', fontSize: 12 }}>
                      {node.ip_address || '-'}
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        )}
      </div>
    </div>
  )
}
