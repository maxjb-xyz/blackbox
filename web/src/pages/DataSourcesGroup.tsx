import { useCallback, useEffect, useState } from 'react'
import { Circle } from 'lucide-react'
import type {
  DataSourceInstance,
  SourceTypeDef,
  CreateSourceInput,
  UpdateSourceInput,
  SourcesResponse,
} from '../api/client'
import {
  fetchSources,
  fetchSourceTypes,
  createSource,
  updateSource,
  deleteSource,
  createExcludedTarget,
  deleteExcludedTarget,
  listExcludedTargets,
  parseSourceConfig,
} from '../api/client'
import type { ExcludedTarget } from '../api/client'
import SourceIcon from '../components/SourceIcon'
import { getSourceCardColors } from '../components/sourceIcons'
import SourceCatalog from './SourceCatalog'

type Selection =
  | { kind: 'server'; id: string }
  | { kind: 'node'; nodeName: string; id: string }
  | { kind: 'orphan'; id: string }
  | { kind: 'docker'; nodeName: string }
  | null

const WEBHOOK_ENDPOINTS: Record<string, string> = {
  webhook_uptime_kuma: '/api/webhooks/uptime',
  webhook_watchtower: '/api/webhooks/watchtower',
}

export default function DataSourcesGroup() {
  const [sources, setSources] = useState<SourcesResponse | null>(null)
  const [sourceTypes, setSourceTypes] = useState<SourceTypeDef[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [selection, setSelection] = useState<Selection>(null)
  const [catalogNode, setCatalogNode] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)
  const [saveError, setSaveError] = useState<string | null>(null)
  const [excludes, setExcludes] = useState<ExcludedTarget[]>([])
  const [excludeInput, setExcludeInput] = useState('')
  const [excludeType, setExcludeType] = useState<'container' | 'stack'>('container')
  const [excludeError, setExcludeError] = useState<string | null>(null)
  const [addingExclude, setAddingExclude] = useState(false)
  const [confirmingDeleteId, setConfirmingDeleteId] = useState<string | null>(null)
  const [confirmingExcludeId, setConfirmingExcludeId] = useState<string | null>(null)
  const [deletingExcludeId, setDeletingExcludeId] = useState<string | null>(null)

  const load = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      const [s, t] = await Promise.all([fetchSources(), fetchSourceTypes()])
      setSources(s)
      setSourceTypes(t)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to load sources')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => { void load() }, [load])

  const loadExcludes = useCallback(async () => {
    setExcludeError(null)
    try {
      setExcludes(await listExcludedTargets())
    } catch (error) {
      console.error('Failed to load excluded targets', error)
      setExcludeError(error instanceof Error ? error.message : 'Failed to load excluded targets')
    }
  }, [])

  useEffect(() => {
    if (selection?.kind === 'docker') void loadExcludes()
  }, [selection?.kind, loadExcludes])

  useEffect(() => {
    setConfirmingDeleteId(null)
    setConfirmingExcludeId(null)
  }, [selection])

  const selectedInstance: DataSourceInstance | null = (() => {
    if (!sources) return null
    if (selection?.kind === 'server') return sources.server.find(s => s.id === selection.id) ?? null
    if (selection?.kind === 'node') {
      const ns = sources.nodes[selection.nodeName]
      return ns?.sources.find(s => s.id === selection.id) ?? null
    }
    if (selection?.kind === 'orphan') return sources.orphans.find(s => s.id === selection.id) ?? null
    return null
  })()

  async function handleSave(id: string, input: UpdateSourceInput) {
    setSaving(true)
    setSaveError(null)
    try {
      await updateSource(id, input)
      await load()
    } catch (e) {
      setSaveError(e instanceof Error ? e.message : 'Save failed')
    } finally {
      setSaving(false)
    }
  }

  async function handleDelete(id: string) {
    if (saving) return
    if (confirmingDeleteId !== id) {
      setConfirmingDeleteId(id)
      return
    }
    setSaving(true)
    setSaveError(null)
    try {
      await deleteSource(id)
      setSelection(null)
      setConfirmingDeleteId(null)
      await load()
    } catch (e) {
      setSaveError(e instanceof Error ? e.message : 'Delete failed')
    } finally {
      setSaving(false)
    }
  }

  async function handleCreate(input: CreateSourceInput) {
    setSaving(true)
    setSaveError(null)
    try {
      const inst = await createSource(input)
      await load()
      setCatalogNode(null)
      if (input.scope === 'server') {
        setSelection({ kind: 'server', id: inst.id })
      } else if (input.node_id) {
        setSelection({ kind: 'node', nodeName: input.node_id, id: inst.id })
      }
    } catch (e) {
      setSaveError(e instanceof Error ? e.message : 'Create failed')
    } finally {
      setSaving(false)
    }
  }

  if (loading) return <div style={{ padding: 20, color: 'var(--muted)', fontSize: 12 }}>Loading...</div>
  if (error) return <div style={{ padding: 20, color: 'var(--danger)', fontSize: 12 }}>{error}</div>
  if (!sources) return null

  const nodeNames = Object.keys(sources.nodes).sort()

  return (
    <div style={{ display: 'flex', minHeight: '100%' }}>
      <div style={{ width: 180, flexShrink: 0, borderRight: '1px solid var(--border)' }}>
        <div style={{ marginBottom: 4 }}>
          <div style={{ fontSize: 9, letterSpacing: '0.14em', color: 'var(--muted)', padding: '10px 12px 4px', textTransform: 'uppercase' }}>
            Server
          </div>
          {sources.server.map(inst => (
            <SidebarTab
              key={inst.id}
              label={inst.name}
              type={inst.type}
              active={selection?.kind === 'server' && selection.id === inst.id}
              enabled={inst.enabled}
              onClick={() => setSelection({ kind: 'server', id: inst.id })}
            />
          ))}
          <AddSourceButton onClick={() => setCatalogNode('__server__')} />
        </div>

        {sources.orphans.length > 0 && (
          <div style={{ marginTop: 8 }}>
            <div style={{ fontSize: 9, letterSpacing: '0.14em', color: 'var(--muted)', padding: '10px 12px 4px', textTransform: 'uppercase' }}>
              Orphans
            </div>
            {sources.orphans.map(inst => (
              <SidebarTab
                key={inst.id}
                label={inst.name}
                type={inst.type}
                active={selection?.kind === 'orphan' && selection.id === inst.id}
                enabled={inst.enabled}
                onClick={() => setSelection({ kind: 'orphan', id: inst.id })}
              />
            ))}
          </div>
        )}

        {nodeNames.map(nodeName => {
          const ns = sources.nodes[nodeName]
          const isOnline = ns.status === 'online'
          return (
            <div key={nodeName} style={{ marginTop: 8 }}>
              <div style={{ fontSize: 9, letterSpacing: '0.12em', color: 'var(--muted)', padding: '10px 12px 4px', textTransform: 'uppercase', display: 'flex', alignItems: 'center', gap: 5 }}>
                <Circle size={5} fill={isOnline ? 'var(--success)' : 'var(--muted)'} color={isOnline ? 'var(--success)' : 'var(--muted)'} />
                {nodeName}
              </div>
              <SidebarTab
                label="Docker"
                type="docker"
                active={selection?.kind === 'docker' && selection.nodeName === nodeName}
                enabled={true}
                onClick={() => setSelection({ kind: 'docker', nodeName })}
              />
              {ns.sources.map(inst => (
                <SidebarTab
                  key={inst.id}
                  label={inst.name}
                  type={inst.type}
                  active={selection?.kind === 'node' && selection.nodeName === nodeName && selection.id === inst.id}
                  enabled={inst.enabled}
                  onClick={() => setSelection({ kind: 'node', nodeName, id: inst.id })}
                />
              ))}
              <AddSourceButton onClick={() => setCatalogNode(nodeName)} />
            </div>
          )
        })}
      </div>

      <div style={{ flex: 1, padding: '0 0 0 24px', position: 'relative' }}>
        {catalogNode !== null && (
          <SourceCatalog
            nodeName={catalogNode === '__server__' ? null : catalogNode}
            nodeInfo={catalogNode === '__server__' ? null : sources.nodes[catalogNode] ?? null}
            sourceTypes={sourceTypes}
            existingSources={catalogNode === '__server__' ? sources.server : (sources.nodes[catalogNode]?.sources ?? [])}
            onSelect={handleCreate}
            onClose={() => setCatalogNode(null)}
          />
        )}

        {selectedInstance && (
          <SourceConfigPanel
            instance={selectedInstance}
            saving={saving}
            saveError={saveError}
            deleteConfirming={confirmingDeleteId === selectedInstance.id}
            onSave={(input) => handleSave(selectedInstance.id, input)}
            onDelete={() => handleDelete(selectedInstance.id)}
            onCancelDelete={() => setConfirmingDeleteId(null)}
          />
        )}

        {selection?.kind === 'docker' && (
          <DockerPanel
            excludes={excludes}
            excludeInput={excludeInput}
            excludeType={excludeType}
            excludeError={excludeError}
            addingExclude={addingExclude}
            onExcludeInputChange={setExcludeInput}
            onExcludeTypeChange={setExcludeType}
            onAddExclude={async () => {
              const trimmedName = excludeInput.trim()
              if (trimmedName === '') {
                setExcludeError('Name is required')
                return
              }
              setExcludeError(null)
              setAddingExclude(true)
              try {
                await createExcludedTarget({ type: excludeType, name: trimmedName })
                setExcludeInput('')
                await loadExcludes()
              } catch (e) {
                setExcludeError(e instanceof Error ? e.message : 'Failed to add')
              } finally {
                setAddingExclude(false)
              }
            }}
            onRemoveExclude={async (id) => {
              if (deletingExcludeId) return
              if (confirmingExcludeId !== id) {
                setConfirmingExcludeId(id)
                return
              }
              setExcludeError(null)
              setDeletingExcludeId(id)
              try {
                await deleteExcludedTarget(id)
                setConfirmingExcludeId(null)
                await loadExcludes()
              } catch (error) {
                console.error('Failed to delete excluded target', error)
                setExcludeError(error instanceof Error ? error.message : 'Failed to delete')
              } finally {
                setDeletingExcludeId(null)
              }
            }}
            confirmingExcludeId={confirmingExcludeId}
            deletingExcludeId={deletingExcludeId}
            onCancelRemoveExclude={() => setConfirmingExcludeId(null)}
          />
        )}

        {!selectedInstance && selection?.kind !== 'docker' && catalogNode === null && (
          <div style={{ padding: 20, color: 'var(--muted)', fontSize: 12 }}>
            Select a source from the sidebar to configure it.
          </div>
        )}
      </div>
    </div>
  )
}

function SidebarTab({ label, type, active, enabled, onClick }: {
  label: string; type: string; active: boolean; enabled: boolean; onClick: () => void
}) {
  const color = getSourceCardColors(type).text
  return (
    <button
      type="button"
      onClick={onClick}
      style={{
        display: 'flex', alignItems: 'center', gap: 7, width: '100%',
        padding: '6px 12px', fontSize: 11, letterSpacing: '0.08em',
        color: active ? 'var(--text)' : 'var(--muted)',
        background: active ? 'var(--surface)' : 'transparent',
        border: 'none', borderRight: `2px solid ${active ? 'var(--muted)' : 'transparent'}`,
        cursor: 'pointer', fontFamily: 'inherit', textAlign: 'left',
        opacity: enabled ? 1 : 0.5,
      }}
    >
      <span style={{ width: 16, height: 16, color, flexShrink: 0, display: 'inline-flex', alignItems: 'center', justifyContent: 'center' }}>
        <SourceIcon type={type} size={14} strokeWidth={1.7} />
      </span>
      {label}
    </button>
  )
}

function AddSourceButton({ onClick }: { onClick: () => void }) {
  return (
    <button
      type="button"
      onClick={onClick}
      style={{
        display: 'block', width: '100%', padding: '5px 12px',
        fontSize: 10, letterSpacing: '0.08em', color: 'var(--muted)',
        background: 'transparent', border: 'none', cursor: 'pointer',
        fontFamily: 'inherit', textAlign: 'left',
      }}
    >
      + add source
    </button>
  )
}

function SourceConfigPanel({ instance, saving, saveError, deleteConfirming, onSave, onDelete, onCancelDelete }: {
  instance: DataSourceInstance
  saving: boolean
  saveError: string | null
  deleteConfirming: boolean
  onSave: (input: UpdateSourceInput) => void
  onDelete: () => void
  onCancelDelete: () => void
}) {
  const initialConfig = (): Record<string, unknown> => parseSourceConfig<Record<string, unknown>>(instance) ?? {}
  const [enabled, setEnabled] = useState(instance.enabled)
  const [name, setName] = useState(instance.name)
  const [localCfg, setLocalCfg] = useState<Record<string, unknown>>(initialConfig)
  const [editedFields, setEditedFields] = useState<Set<string>>(new Set())

  useEffect(() => {
    setEnabled(instance.enabled)
    setName(instance.name)
    setLocalCfg(parseSourceConfig<Record<string, unknown>>(instance) ?? {})
    setEditedFields(new Set())
  }, [instance.id, instance.enabled, instance.name, instance.config])

  const typeLabel = instance.type.replace(/_/g, ' ').replace(/\b\w/g, c => c.toUpperCase())
  const isWebhook = instance.type.startsWith('webhook_')
  const endpointPath = WEBHOOK_ENDPOINTS[instance.type]
  const typeColor = getSourceCardColors(instance.type).text
  const unitsArray = Array.isArray(localCfg.units) ? localCfg.units.map(String) : []

  return (
    <div>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 12, marginBottom: 16 }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 10, minWidth: 0 }}>
          <span style={{ color: typeColor, display: 'inline-flex', alignItems: 'center', justifyContent: 'center', flexShrink: 0 }}>
            <SourceIcon type={instance.type} size={18} strokeWidth={1.7} />
          </span>
          <div style={{ fontSize: 11, letterSpacing: '0.1em', color: 'var(--muted)', textTransform: 'uppercase', minWidth: 0 }}>
            {name}
            <span style={{ marginLeft: 8, fontSize: 9, color: 'var(--muted)', border: '1px solid var(--border)', padding: '1px 5px', letterSpacing: '0.08em' }}>
              {typeLabel}
            </span>
          </div>
        </div>
        <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
          <span style={{ position: 'absolute', width: 1, height: 1, padding: 0, margin: -1, overflow: 'hidden', clip: 'rect(0, 0, 0, 0)', whiteSpace: 'nowrap', border: 0 }} aria-live="polite">
            {deleteConfirming ? 'Click again to confirm removing this source' : 'Remove source'}
          </span>
          {deleteConfirming && (
            <button type="button" onClick={onCancelDelete} style={{ fontSize: 10, color: 'var(--muted)', border: '1px solid var(--border)', padding: '3px 8px', background: 'transparent', cursor: 'pointer', fontFamily: 'inherit' }}>
              Cancel
            </button>
          )}
          <button type="button" aria-label={deleteConfirming ? 'Click again to confirm removing this source' : 'Remove source'} onClick={onDelete} style={{ fontSize: 10, color: deleteConfirming ? 'var(--danger)' : 'var(--muted)', border: '1px solid var(--border)', padding: '3px 8px', background: 'transparent', cursor: 'pointer', fontFamily: 'inherit' }}>
            {deleteConfirming ? 'Click again to confirm' : 'Remove'}
          </button>
        </div>
      </div>

      <div style={{ border: '1px solid var(--border)' }}>
        <ConfigRow label="Enabled">
          <label style={{ display: 'flex', alignItems: 'center', gap: 8, cursor: 'pointer' }}>
            <input type="checkbox" checked={enabled} onChange={e => setEnabled(e.target.checked)} />
            <span style={{ fontSize: 11, color: 'var(--muted)' }}>{enabled ? 'Active' : 'Disabled'}</span>
          </label>
        </ConfigRow>

        <ConfigRow label="Name">
          <input
            value={name}
            onChange={e => setName(e.target.value)}
            style={{ background: 'var(--surface)', border: '1px solid var(--border)', color: 'var(--text)', fontFamily: 'inherit', fontSize: 11, padding: '4px 8px', width: '100%' }}
          />
        </ConfigRow>

        {isWebhook && (
          <>
            <ConfigRow label="Endpoint">
              <div style={{ color: 'var(--muted)', fontSize: 11 }}>{endpointPath ?? 'No webhook endpoint registered for this source type'}</div>
              <div style={{ fontSize: 9, color: 'var(--muted)', marginTop: 3 }}>
                {endpointPath ? 'Set this URL in your monitoring tool' : 'This webhook type needs a registered endpoint before it can be configured.'}
              </div>
            </ConfigRow>
            <ConfigRow label="Secret Token">
              <input
                type="password"
                value={String(localCfg.secret ?? '')}
                onChange={e => {
                  setLocalCfg(c => ({ ...c, secret: e.target.value }))
                  setEditedFields(f => new Set(f).add('secret'))
                }}
                placeholder="Enter webhook secret"
                style={{ background: 'var(--surface)', border: '1px solid var(--border)', color: 'var(--text)', fontFamily: 'inherit', fontSize: 11, padding: '4px 8px', width: '100%' }}
              />
              {!editedFields.has('secret') && (
                <div style={{ fontSize: 9, color: 'var(--muted)', marginTop: 3 }}>
                  Leave blank to keep existing secret
                </div>
              )}
            </ConfigRow>
          </>
        )}

        {instance.type === 'systemd' && (
          <ConfigRow label="Watched Units">
            <textarea
              rows={4}
              value={unitsArray.join('\n')}
              onChange={e => {
                const parsedLines = e.target.value.split('\n')
                setLocalCfg(c => ({ ...c, units: parsedLines }))
              }}
              placeholder="nginx.service&#10;caddy.service"
              style={{ background: 'var(--surface)', border: '1px solid var(--border)', color: 'var(--text)', fontFamily: 'inherit', fontSize: 11, padding: '4px 8px', width: '100%', resize: 'vertical' }}
            />
            <div style={{ fontSize: 9, color: 'var(--muted)', marginTop: 3 }}>One unit per line.</div>
          </ConfigRow>
        )}

        {instance.type === 'filewatcher' && (
          <ConfigRow label="Redact Secrets">
            <label style={{ display: 'flex', alignItems: 'center', gap: 8, cursor: 'pointer' }}>
              <input
                type="checkbox"
                checked={Boolean(localCfg.redact_secrets)}
                onChange={e => setLocalCfg(c => ({ ...c, redact_secrets: e.target.checked }))}
              />
              <span style={{ fontSize: 11, color: 'var(--muted)' }}>Strip likely secrets from file change diffs</span>
            </label>
          </ConfigRow>
        )}

        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', padding: '10px 14px', borderTop: '1px solid var(--border)' }}>
          {saveError ? <span style={{ fontSize: 10, color: 'var(--danger)' }}>{saveError}</span> : <span />}
          <button
            type="button"
            disabled={saving}
            onClick={() => {
              const configPayload = Object.fromEntries(
                Object.entries(localCfg).filter(([key]) => !(key === 'secret' && !editedFields.has(key))),
              )
              if (Array.isArray(configPayload.units)) {
                configPayload.units = configPayload.units
                  .map(value => String(value).trim())
                  .filter(value => value !== '')
              }
              onSave({ name, enabled, config: configPayload })
            }}
            style={{ fontSize: 10, border: '1px solid var(--border)', padding: '4px 12px', background: 'transparent', color: 'var(--text)', cursor: 'pointer', fontFamily: 'inherit' }}
          >
            {saving ? 'Saving...' : 'Save'}
          </button>
        </div>
      </div>
    </div>
  )
}

function ConfigRow({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div style={{ display: 'flex', alignItems: 'flex-start', padding: '10px 14px', borderBottom: '1px solid var(--border)', gap: 12 }}>
      <div style={{ fontSize: 10, color: 'var(--muted)', width: 130, flexShrink: 0, paddingTop: 2 }}>{label}</div>
      <div style={{ flex: 1 }}>{children}</div>
    </div>
  )
}

function DockerPanel({ excludes, excludeInput, excludeType, excludeError, addingExclude, confirmingExcludeId, deletingExcludeId, onExcludeInputChange, onExcludeTypeChange, onAddExclude, onRemoveExclude, onCancelRemoveExclude }: {
  excludes: ExcludedTarget[]
  excludeInput: string
  excludeType: 'container' | 'stack'
  excludeError: string | null
  addingExclude: boolean
  confirmingExcludeId: string | null
  deletingExcludeId: string | null
  onExcludeInputChange: (v: string) => void
  onExcludeTypeChange: (v: 'container' | 'stack') => void
  onAddExclude: () => void
  onRemoveExclude: (id: string) => void
  onCancelRemoveExclude: () => void
}) {
  const canAddExclude = excludeInput.trim() !== '' && !addingExclude

  return (
    <div>
      <div style={{ marginBottom: 16 }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 10, fontSize: 11, letterSpacing: '0.1em', color: 'var(--muted)', textTransform: 'uppercase' }}>
          <span style={{ color: getSourceCardColors('docker').text, display: 'inline-flex', alignItems: 'center', justifyContent: 'center' }}>
            <SourceIcon type="docker" size={18} strokeWidth={1.7} />
          </span>
          <span>
            Docker
            <span style={{ marginLeft: 8, fontSize: 9, color: 'var(--muted)', border: '1px solid var(--border)', padding: '1px 5px' }}>Always on</span>
          </span>
        </div>
        <div style={{ fontSize: 10, color: 'var(--muted)', marginTop: 3 }}>Container lifecycle events — runs on every agent automatically.</div>
      </div>

      <div style={{ border: '1px solid var(--border)' }}>
        <div style={{ padding: '10px 14px', borderBottom: '1px solid var(--border)', fontSize: 10, color: 'var(--muted)', letterSpacing: '0.08em', textTransform: 'uppercase' }}>
          Excluded Targets
        </div>
        {excludes.length === 0 && (
          <div style={{ padding: '10px 14px', fontSize: 10, color: 'var(--muted)' }}>No exclusions configured.</div>
        )}
        {excludes.map(ex => (
          <div key={ex.id} style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '7px 14px', borderBottom: '1px solid var(--border)', fontSize: 11 }}>
            <span>
              <span style={{ fontSize: 9, color: 'var(--muted)', marginRight: 8, border: '1px solid var(--border)', padding: '1px 4px' }}>{ex.type}</span>
              {ex.name}
            </span>
            <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
              {confirmingExcludeId === ex.id && (
                <button
                  type="button"
                  onClick={onCancelRemoveExclude}
                  disabled={deletingExcludeId === ex.id}
                  style={{ fontSize: 10, color: 'var(--muted)', background: 'transparent', border: 'none', cursor: 'pointer', fontFamily: 'inherit' }}
                >
                  Cancel
                </button>
              )}
              <button
                type="button"
                onClick={() => onRemoveExclude(ex.id)}
                disabled={deletingExcludeId === ex.id}
                style={{ fontSize: 10, color: confirmingExcludeId === ex.id ? 'var(--danger)' : 'var(--muted)', background: 'transparent', border: 'none', cursor: 'pointer', fontFamily: 'inherit' }}
              >
                {deletingExcludeId === ex.id ? 'Removing...' : confirmingExcludeId === ex.id ? 'Click again to confirm' : 'Remove'}
              </button>
            </div>
          </div>
        ))}
        <div style={{ padding: '10px 14px', display: 'flex', gap: 8, alignItems: 'center' }}>
          <select
            value={excludeType}
            onChange={e => onExcludeTypeChange(e.target.value as 'container' | 'stack')}
            style={{ background: 'var(--surface)', border: '1px solid var(--border)', color: 'var(--text)', fontFamily: 'inherit', fontSize: 11, padding: '4px 8px' }}
          >
            <option value="container">container</option>
            <option value="stack">stack</option>
          </select>
          <input
            value={excludeInput}
            onChange={e => onExcludeInputChange(e.target.value)}
            placeholder="container-name"
            onKeyDown={e => {
              if (e.key !== 'Enter') return
              if (!canAddExclude) {
                e.preventDefault()
                return
              }
              onAddExclude()
            }}
            style={{ flex: 1, background: 'var(--surface)', border: '1px solid var(--border)', color: 'var(--text)', fontFamily: 'inherit', fontSize: 11, padding: '4px 8px' }}
          />
          <button
            type="button"
            onClick={onAddExclude}
            disabled={!canAddExclude}
            style={{ fontSize: 10, border: '1px solid var(--border)', padding: '4px 10px', background: 'transparent', color: canAddExclude ? 'var(--text)' : 'var(--muted)', cursor: canAddExclude ? 'pointer' : 'default', fontFamily: 'inherit' }}
          >
            {addingExclude ? 'Adding...' : 'Add'}
          </button>
        </div>
        {excludeError && <div style={{ padding: '0 14px 10px', fontSize: 10, color: 'var(--danger)' }}>{excludeError}</div>}
      </div>
    </div>
  )
}
