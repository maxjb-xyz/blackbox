import type { NodeSources, SourceTypeDef, DataSourceInstance, CreateSourceInput } from '../api/client'

interface Props {
  nodeName: string | null
  nodeInfo: NodeSources | null
  sourceTypes: SourceTypeDef[]
  existingSources: DataSourceInstance[]
  onSelect: (input: CreateSourceInput) => void
  onClose: () => void
}

export default function SourceCatalog({ nodeName, nodeInfo, sourceTypes, existingSources, onSelect, onClose }: Props) {
  const scope = nodeName === null ? 'server' : 'agent'
  const caps = nodeInfo?.capabilities ?? []

  const scopedTypes = sourceTypes.filter(t => t.scope === scope)

  const available = scope === 'server'
    ? scopedTypes
    : scopedTypes.filter(t => t.type === 'docker' || caps.includes(t.type))

  const unavailable = scope === 'agent'
    ? scopedTypes.filter(t => t.type !== 'docker' && !caps.includes(t.type))
    : []

  function isAdded(type: string): boolean {
    const typeDef = sourceTypes.find(t => t.type === type)
    if (!typeDef?.singleton) return false
    return existingSources.some(s => s.type === type)
  }

  function handleSelect(typeDef: SourceTypeDef) {
    if (typeDef.type === 'docker') return
    if (isAdded(typeDef.type)) return
    onSelect({
      type: typeDef.type,
      scope: typeDef.scope,
      node_id: nodeName ?? undefined,
      name: typeDef.name,
      config: buildDefaultConfig(typeDef.type),
      enabled: true,
    })
  }

  return (
    <div style={{
      position: 'absolute', top: 0, left: 0, right: 0, bottom: 0,
      background: 'var(--bg)', border: '1px solid var(--border)',
      zIndex: 10, display: 'flex', flexDirection: 'column',
    }}>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '14px 20px', borderBottom: '1px solid var(--border)' }}>
        <div style={{ fontSize: 11, letterSpacing: '0.14em', color: 'var(--muted)', textTransform: 'uppercase' }}>Add Source</div>
        <div style={{ display: 'flex', alignItems: 'center', gap: 20 }}>
          {nodeName && nodeInfo && (
            <div style={{ fontSize: 10, color: 'var(--muted)', letterSpacing: '0.06em' }}>
              {nodeName} · {nodeInfo.agent_version}
            </div>
          )}
          {nodeName === null && (
            <div style={{ fontSize: 10, color: 'var(--muted)', letterSpacing: '0.06em' }}>Server</div>
          )}
          <button type="button" onClick={onClose} style={{ fontSize: 14, color: 'var(--muted)', background: 'transparent', border: 'none', cursor: 'pointer', fontFamily: 'inherit', lineHeight: 1 }}>
            ✕
          </button>
        </div>
      </div>

      <div style={{ flex: 1, padding: 20, overflowY: 'auto' }}>
        {available.length > 0 && (
          <>
            <div style={{ fontSize: 9, letterSpacing: '0.14em', color: 'var(--muted)', textTransform: 'uppercase', marginBottom: 12 }}>
              {scope === 'server' ? 'Server sources' : 'Available on this node'}
            </div>
            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, 1fr)', gap: 10, marginBottom: 24 }}>
              {available.map(typeDef => (
                <SourceCard
                  key={typeDef.type}
                  typeDef={typeDef}
                  added={isAdded(typeDef.type)}
                  virtual={typeDef.type === 'docker'}
                  onClick={() => handleSelect(typeDef)}
                />
              ))}
            </div>
          </>
        )}

        {unavailable.length > 0 && (
          <>
            <div style={{ fontSize: 9, letterSpacing: '0.14em', color: 'var(--muted)', textTransform: 'uppercase', marginBottom: 6 }}>Not available on this node</div>
            <div style={{ fontSize: 10, color: 'var(--muted)', marginBottom: 14 }}>Update the agent to unlock these sources</div>
            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, 1fr)', gap: 10, opacity: 0.25, pointerEvents: 'none' }}>
              {unavailable.map(typeDef => (
                <SourceCard key={typeDef.type} typeDef={typeDef} added={false} virtual={false} onClick={() => {}} />
              ))}
            </div>
          </>
        )}
      </div>
    </div>
  )
}

const TYPE_COLORS: Record<string, { border: string; bg: string; text: string; topBar: string }> = {
  docker:              { border: '#1a3a5a', bg: '#0d1e2e', text: '#3a7abd', topBar: 'linear-gradient(90deg,#1a4a7a,transparent 60%)' },
  systemd:             { border: '#3a2a5a', bg: '#1a1228', text: '#7a5abd', topBar: 'linear-gradient(90deg,#4a3a7a,transparent 60%)' },
  filewatcher:         { border: '#5a3a1a', bg: '#281a0d', text: '#bd7a3a', topBar: 'linear-gradient(90deg,#7a4a1a,transparent 60%)' },
  proxmox:             { border: '#5a2018', bg: '#28100d', text: '#bd4a3a', topBar: 'linear-gradient(90deg,#7a2a1a,transparent 60%)' },
  webhook_uptime_kuma: { border: '#1a5a3a', bg: '#0d2818', text: '#3abd7a', topBar: 'linear-gradient(90deg,#1a7a4a,transparent 60%)' },
  webhook_watchtower:  { border: '#1a5a3a', bg: '#0d2818', text: '#3abd7a', topBar: 'linear-gradient(90deg,#1a7a4a,transparent 60%)' },
}

const TYPE_SHORT: Record<string, string> = {
  docker: 'DCK', systemd: 'SYS', filewatcher: 'FILE',
  proxmox: 'PVE', webhook_uptime_kuma: 'WHK', webhook_watchtower: 'WHK',
}

function SourceCard({ typeDef, added, virtual, onClick }: {
  typeDef: SourceTypeDef; added: boolean; virtual: boolean; onClick: () => void
}) {
  const colors = TYPE_COLORS[typeDef.type] ?? { border: '#222', bg: '#111', text: '#666', topBar: 'none' }
  const short = TYPE_SHORT[typeDef.type] ?? '???'
  const disabled = added || virtual

  return (
    <button
      type="button"
      onClick={disabled ? undefined : onClick}
      disabled={disabled}
      style={{
        border: `1px solid ${colors.border}`,
        background: colors.bg,
        padding: 0,
        cursor: disabled ? 'default' : 'pointer',
        fontFamily: 'inherit',
        color: 'inherit',
        textAlign: 'left',
        position: 'relative',
        overflow: 'hidden',
        opacity: disabled ? 0.4 : 1,
        width: '100%',
      }}
    >
      <div style={{ position: 'absolute', top: 0, left: 0, right: 0, height: 1, background: colors.topBar }} />

      <div style={{ padding: '14px 16px 16px' }}>
        <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', marginBottom: 10 }}>
          <div style={{ width: 28, height: 28, display: 'flex', alignItems: 'center', justifyContent: 'center', fontSize: 9, fontWeight: 'bold', letterSpacing: '0.08em', border: `1px solid ${colors.border}`, background: colors.bg, color: colors.text }}>
            {short}
          </div>
          {(added || virtual) && (
            <div style={{ fontSize: 9, padding: '2px 6px', letterSpacing: '0.08em', border: `1px solid ${colors.border}`, color: colors.text }}>
              {virtual ? 'Built-in' : 'Added'}
            </div>
          )}
        </div>
        <div style={{ fontSize: 12, letterSpacing: '0.08em', color: '#c0c0c0', marginBottom: 5 }}>{typeDef.name}</div>
        <div style={{ fontSize: 10, color: '#3a3a3a', lineHeight: 1.6 }}>{typeDef.description}</div>
      </div>

      <div style={{ padding: '8px 16px', borderTop: '1px solid #181818', display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
        <span style={{ fontSize: 9, color: '#2a2a2a', letterSpacing: '0.08em' }}>{typeDef.mechanism}</span>
        {!disabled && <span style={{ fontSize: 11, color: '#2a2a2a' }}>→</span>}
      </div>
    </button>
  )
}

function buildDefaultConfig(type: string): Record<string, unknown> {
  switch (type) {
    case 'systemd': return { units: [] }
    case 'filewatcher': return { redact_secrets: true }
    case 'proxmox': return { url: '', api_token: '', insecure_skip_verify: false, poll_interval_seconds: 10 }
    case 'webhook_uptime_kuma':
    case 'webhook_watchtower': return { secret: '' }
    default: return {}
  }
}
