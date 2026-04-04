import { useNodePulse } from '../components/NodePulse'

function formatTimestamp(ts?: string | null) {
  if (!ts) return '-'
  const date = new Date(ts)
  if (Number.isNaN(date.getTime())) return '-'
  return date.toISOString().replace('T', ' ').substring(0, 19)
}

export default function NodesPage() {
  const { nodes, loading, error, lastUpdated } = useNodePulse()

  return (
    <div style={{ padding: 0 }}>
      <div style={{ padding: '18px 24px', borderBottom: '1px solid var(--border)', background: 'var(--surface)' }}>
        <span style={{ color: 'var(--muted)', fontSize: '11px', letterSpacing: '0.1em' }}>
          NODES / AGENT REGISTRY
        </span>
      </div>

      <div style={{ maxWidth: 960, margin: '0 auto' }}>
        {(loading || error || lastUpdated) && (
          <div style={{ padding: '10px 24px', borderBottom: '1px solid var(--border)', fontSize: '11px', color: 'var(--muted)' }}>
            {loading && !lastUpdated && 'checking agent registry...'}
            {!loading && error && !lastUpdated && 'failed to load agent registry'}
            {error && lastUpdated && `showing cached node data from ${formatTimestamp(lastUpdated.toISOString())}`}
            {!error && lastUpdated && `last updated ${formatTimestamp(lastUpdated.toISOString())}`}
          </div>
        )}

        {nodes.length === 0 ? (
          <div style={{ padding: '32px 24px', color: 'var(--muted)', fontSize: '12px', textAlign: 'center' }}>
            {error && !lastUpdated ? 'failed to load agent registry' : loading ? 'checking agent registry...' : 'no agents registered'}
          </div>
        ) : (
          <table className="nodes-table">
            <thead>
              <tr>
                <th scope="col" />
                <th scope="col">NAME</th>
                <th scope="col">LAST SEEN</th>
                <th scope="col">OS</th>
                <th scope="col">VERSION</th>
                <th scope="col">IP</th>
              </tr>
            </thead>
            <tbody>
              {nodes.map(node => (
                <tr key={node.id}>
                  <td>
                    <span
                      className="nodes-status-dot"
                      style={{ color: node.status === 'online' ? 'var(--success)' : 'var(--danger)' }}
                      role="img"
                      aria-label={node.status}
                    >
                      ●
                    </span>
                  </td>
                  <td className="nodes-cell-truncate" style={{ color: 'var(--text)' }}>
                    {node.name}
                  </td>
                  <td style={{ color: 'var(--muted)' }}>{formatTimestamp(node.last_seen)}</td>
                  <td className="nodes-cell-truncate" style={{ color: 'var(--text)' }}>
                    {node.os_info || '-'}
                  </td>
                  <td style={{ color: 'var(--muted)' }}>{node.agent_version || '-'}</td>
                  <td style={{ color: 'var(--muted)' }}>{node.ip_address || '-'}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </div>
  )
}
