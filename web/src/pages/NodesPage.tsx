import { useNodePulse } from '../components/NodePulse'

function formatTimestamp(ts: string) {
  return new Date(ts).toISOString().replace('T', ' ').substring(0, 19)
}

export default function NodesPage() {
  const { nodes } = useNodePulse()

  return (
    <div style={{ padding: 0 }}>
      <div style={{ padding: '12px 16px', borderBottom: '1px solid var(--border)', background: 'var(--surface)' }}>
        <span style={{ color: 'var(--muted)', fontSize: '11px', letterSpacing: '0.1em' }}>
          NODES / AGENT REGISTRY
        </span>
      </div>

      <div
        style={{
          display: 'grid',
          gridTemplateColumns: '24px 120px 160px 180px 120px 120px',
          gap: '0 12px',
          padding: '4px 16px',
          borderBottom: '1px solid var(--border)',
          background: 'var(--surface)',
          color: 'var(--muted)',
          fontSize: '10px',
          letterSpacing: '0.1em',
        }}
      >
        <span />
        <span>NAME</span>
        <span>LAST SEEN</span>
        <span>OS</span>
        <span>VERSION</span>
        <span>IP</span>
      </div>

      {nodes.length === 0 ? (
        <div style={{ padding: '32px 16px', color: 'var(--muted)', fontSize: '12px', textAlign: 'center' }}>
          no agents registered
        </div>
      ) : (
        nodes.map(node => (
          <div
            key={node.id}
            style={{
              display: 'grid',
              gridTemplateColumns: '24px 120px 160px 180px 120px 120px',
              gap: '0 12px',
              padding: '6px 16px',
              borderBottom: '1px solid var(--border)',
              fontSize: '12px',
              alignItems: 'center',
            }}
          >
            <span
              style={{
                color: node.status === 'online' ? 'var(--accent)' : '#FF4444',
                fontSize: '14px',
                lineHeight: 1,
              }}
            >
              ●
            </span>
            <span style={{ color: 'var(--text)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
              {node.name}
            </span>
            <span style={{ color: 'var(--muted)' }}>{formatTimestamp(node.last_seen)}</span>
            <span style={{ color: 'var(--text)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
              {node.os_info || '—'}
            </span>
            <span style={{ color: 'var(--muted)' }}>{node.agent_version || '—'}</span>
            <span style={{ color: 'var(--muted)' }}>{node.ip_address || '—'}</span>
          </div>
        ))
      )}
    </div>
  )
}
