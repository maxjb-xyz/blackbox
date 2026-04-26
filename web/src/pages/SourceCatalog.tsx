import { useEffect, useId, useRef, type KeyboardEvent } from 'react'
import type { NodeSources, SourceTypeDef, DataSourceInstance, CreateSourceInput } from '../api/client'
import SourceIcon from '../components/SourceIcon'
import { getSourceCardColors } from '../components/sourceIcons'

interface Props {
  nodeName: string | null
  nodeInfo: NodeSources | null
  sourceTypes: SourceTypeDef[]
  existingSources: DataSourceInstance[]
  onSelect: (input: CreateSourceInput) => void
  onClose: () => void
}

export default function SourceCatalog({ nodeName, nodeInfo, sourceTypes, existingSources, onSelect, onClose }: Props) {
  const dialogRef = useRef<HTMLDivElement | null>(null)
  const titleId = useId()
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

  useEffect(() => {
    const dialog = dialogRef.current
    const previousFocus = document.activeElement instanceof HTMLElement ? document.activeElement : null
    if (!dialog) return undefined

    const focusables = getFocusableElements(dialog)
    ;(focusables[0] ?? dialog).focus()

    return () => {
      previousFocus?.focus()
    }
  }, [])

  function handleKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    if (event.key === 'Escape') {
      event.preventDefault()
      onClose()
      return
    }
    if (event.key !== 'Tab' || !dialogRef.current) return

    const focusables = getFocusableElements(dialogRef.current)
    if (focusables.length === 0) {
      event.preventDefault()
      dialogRef.current.focus()
      return
    }

    const first = focusables[0]
    const last = focusables[focusables.length - 1]
    const active = document.activeElement

    if (event.shiftKey && active === first) {
      event.preventDefault()
      last.focus()
      return
    }
    if (!event.shiftKey && active === last) {
      event.preventDefault()
      first.focus()
    }
  }

  return (
    <div
      ref={dialogRef}
      role="dialog"
      aria-modal="true"
      aria-labelledby={titleId}
      tabIndex={-1}
      onKeyDown={handleKeyDown}
      style={{
        position: 'absolute', top: 0, left: 0, right: 0, bottom: 0,
        background: 'var(--bg)', border: '1px solid var(--border)',
        zIndex: 10, display: 'flex', flexDirection: 'column',
      }}
    >
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '14px 20px', borderBottom: '1px solid var(--border)' }}>
        <div id={titleId} style={{ fontSize: 11, letterSpacing: '0.14em', color: 'var(--muted)', textTransform: 'uppercase' }}>Add Source</div>
        <div style={{ display: 'flex', alignItems: 'center', gap: 20 }}>
          {nodeName && nodeInfo && (
            <div style={{ fontSize: 10, color: 'var(--muted)', letterSpacing: '0.06em' }}>
              {nodeName} · {nodeInfo.agent_version}
            </div>
          )}
          {nodeName === null && (
            <div style={{ fontSize: 10, color: 'var(--muted)', letterSpacing: '0.06em' }}>Server</div>
          )}
          <button type="button" aria-label="Close dialog" onClick={onClose} style={{ fontSize: 14, color: 'var(--muted)', background: 'transparent', border: 'none', cursor: 'pointer', fontFamily: 'inherit', lineHeight: 1 }}>
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

function SourceCard({ typeDef, added, virtual, onClick }: {
  typeDef: SourceTypeDef; added: boolean; virtual: boolean; onClick: () => void
}) {
  const colors = getSourceCardColors(typeDef.type)
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
          <div
            style={{
              width: 32,
              height: 32,
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              border: `1px solid ${colors.border}`,
              background: 'rgba(255,255,255,0.02)',
              color: colors.text,
            }}
          >
            <SourceIcon type={typeDef.type} size={18} />
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
    case 'webhook_uptime_kuma':
    case 'webhook_watchtower': return { secret: '' }
    default: return {}
  }
}

function getFocusableElements(container: HTMLElement): HTMLElement[] {
  const selector = [
    'button:not([disabled])',
    '[href]',
    'input:not([disabled])',
    'select:not([disabled])',
    'textarea:not([disabled])',
    '[tabindex]:not([tabindex="-1"])',
  ].join(',')
  return Array.from(container.querySelectorAll<HTMLElement>(selector))
}
