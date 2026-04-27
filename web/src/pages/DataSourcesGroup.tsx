import { useCallback, useEffect, useRef, useState } from 'react'
import { CheckCircle, Circle, Copy } from 'lucide-react'
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

function isSourceVisible(inst: DataSourceInstance): boolean {
  if (!inst.enabled) return false
  if (inst.type.startsWith('webhook_')) {
    const cfg = parseSourceConfig<{ secret?: string }>(inst)
    return Boolean(cfg?.secret)
  }
  return true
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
  const [creatingSource, setCreatingSource] = useState<CreateSourceInput | null>(null)

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

  const selectedTypeDef: SourceTypeDef | null = selectedInstance
    ? (sourceTypes.find(t => t.type === selectedInstance.type) ?? null)
    : null

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
      setCreatingSource(null)
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

  function handleCatalogSelect(input: CreateSourceInput) {
    setSaveError(null)
    setCatalogNode(null)
    setSelection(null)
    setCreatingSource(input)
  }

  function selectSidebar(sel: Selection) {
    setCatalogNode(null)
    setCreatingSource(null)
    setSelection(sel)
  }

  if (loading) return <div style={{ padding: 24, color: 'var(--muted)', fontSize: 13 }}>Loading...</div>
  if (error) return <div style={{ padding: 24, color: 'var(--danger)', fontSize: 13 }}>{error}</div>
  if (!sources) return null

  const nodeNames = Object.keys(sources.nodes).sort()

  return (
    <div style={{ display: 'flex', minHeight: '100%', gap: 24, gridColumn: '1 / -1' }}>
      {/* Sidebar */}
      <div style={{ width: 200, flexShrink: 0, borderRight: '1px solid var(--border)' }}>
        {/* Server section */}
        <SidebarSection label="Server" />
        {sources.server.filter(isSourceVisible).map(inst => (
          <SidebarTab
            key={inst.id}
            label={inst.name}
            type={inst.type}
            active={selection?.kind === 'server' && selection.id === inst.id}
            onClick={() => selectSidebar({ kind: 'server', id: inst.id })}
          />
        ))}
        <AddSourceButton onClick={() => {
          setCreatingSource(null)
          setSelection(null)
          setCatalogNode('__server__')
        }} />

        {/* Orphans */}
        {sources.orphans.length > 0 && (
          <>
            <SidebarSection label="Orphans" />
            {sources.orphans.filter(isSourceVisible).map(inst => (
              <SidebarTab
                key={inst.id}
                label={inst.name}
                type={inst.type}
                active={selection?.kind === 'orphan' && selection.id === inst.id}
                onClick={() => selectSidebar({ kind: 'orphan', id: inst.id })}
              />
            ))}
          </>
        )}

        {/* Per-node sections */}
        {nodeNames.map(nodeName => {
          const ns = sources.nodes[nodeName]
          const isOnline = ns.status === 'online'
          return (
            <div key={nodeName}>
              <SidebarSection
                label={nodeName}
                dot={<Circle size={6} fill={isOnline ? 'var(--success)' : 'var(--muted)'} color={isOnline ? 'var(--success)' : 'var(--muted)'} />}
              />
              <SidebarTab
                label="Docker"
                type="docker"
                active={selection?.kind === 'docker' && selection.nodeName === nodeName}
                onClick={() => selectSidebar({ kind: 'docker', nodeName })}
              />
              {ns.sources.filter(isSourceVisible).map(inst => (
                <SidebarTab
                  key={inst.id}
                  label={inst.name}
                  type={inst.type}
                  active={selection?.kind === 'node' && selection.nodeName === nodeName && selection.id === inst.id}
                  onClick={() => selectSidebar({ kind: 'node', nodeName, id: inst.id })}
                />
              ))}
              <AddSourceButton onClick={() => {
                setCreatingSource(null)
                setSelection(null)
                setCatalogNode(nodeName)
              }} />
            </div>
          )
        })}
      </div>

      {/* Content area */}
      <div style={{ flex: 1, minWidth: 0, position: 'relative' }}>
        {catalogNode !== null && (
          <SourceCatalog
            nodeName={catalogNode === '__server__' ? null : catalogNode}
            nodeInfo={catalogNode === '__server__' ? null : sources.nodes[catalogNode] ?? null}
            sourceTypes={sourceTypes}
            existingSources={catalogNode === '__server__' ? sources.server : (sources.nodes[catalogNode]?.sources ?? [])}
            onSelect={handleCatalogSelect}
            onClose={() => setCatalogNode(null)}
          />
        )}

        {creatingSource && (
          <SourceConfigPanel
            instance={draftInstanceFromInput(creatingSource)}
            typeDef={sourceTypes.find(t => t.type === creatingSource.type) ?? null}
            saving={saving}
            saveError={saveError}
            deleteConfirming={false}
            creating={true}
            onSave={(input) => handleCreate({
              ...creatingSource,
              name: input.name ?? creatingSource.name,
              enabled: input.enabled ?? creatingSource.enabled,
              config: input.config ?? creatingSource.config,
            })}
            onDelete={() => setCreatingSource(null)}
            onCancelDelete={() => setCreatingSource(null)}
          />
        )}

        {selectedInstance && (
          <SourceConfigPanel
            instance={selectedInstance}
            typeDef={selectedTypeDef}
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

        {!creatingSource && !selectedInstance && selection?.kind !== 'docker' && catalogNode === null && (
          <div style={{ padding: 24, color: 'var(--muted)', fontSize: 13 }}>
            Select a source from the sidebar to configure it.
          </div>
        )}
      </div>
    </div>
  )
}

function SidebarSection({ label, dot }: { label: string; dot?: React.ReactNode }) {
  return (
    <div style={{
      fontSize: 10, letterSpacing: '0.12em', color: 'var(--muted)',
      padding: '16px 14px 6px',
      textTransform: 'uppercase',
      display: 'flex', alignItems: 'center', gap: 6,
    }}>
      {dot}
      {label}
    </div>
  )
}

function SidebarTab({ label, type, active, onClick }: {
  label: string; type: string; active: boolean; onClick: () => void
}) {
  const color = getSourceCardColors(type).text
  return (
    <button
      type="button"
      onClick={onClick}
      style={{
        display: 'flex', alignItems: 'center', gap: 8,
        width: '100%', padding: '10px 14px 10px 12px',
        fontSize: 12, letterSpacing: '0.06em',
        color: active ? 'var(--text)' : 'var(--muted)',
        background: active ? 'linear-gradient(90deg, rgba(255,51,51,0.14), rgba(255,51,51,0.04) 70%, transparent)' : 'transparent',
        border: 'none',
        borderLeft: active ? '2px solid var(--accent)' : '2px solid transparent',
        cursor: 'pointer', fontFamily: 'inherit', textAlign: 'left',
        fontWeight: active ? 600 : 400,
      }}
    >
      <span style={{ color, flexShrink: 0, display: 'inline-flex', alignItems: 'center', justifyContent: 'center' }}>
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
        display: 'block', width: '100%', padding: '6px 14px 12px',
        fontSize: 11, letterSpacing: '0.06em', color: 'var(--muted)',
        background: 'transparent', border: 'none', borderLeft: '2px solid transparent',
        cursor: 'pointer', fontFamily: 'inherit', textAlign: 'left',
      }}
    >
      + add source
    </button>
  )
}

function SourceConfigPanel({ creating = false, instance, typeDef, saving, saveError, deleteConfirming, onSave, onDelete, onCancelDelete }: {
  creating?: boolean
  instance: DataSourceInstance
  typeDef: SourceTypeDef | null
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
  const isSingleton = typeDef?.singleton ?? true
  const unitsArray = Array.isArray(localCfg.units) ? localCfg.units.map(String) : []

  return (
    <div>
      {/* Header */}
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 12, marginBottom: 20 }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 10, minWidth: 0 }}>
          <span style={{ color: typeColor, display: 'inline-flex', alignItems: 'center', justifyContent: 'center', flexShrink: 0 }}>
            <SourceIcon type={instance.type} size={20} strokeWidth={1.7} />
          </span>
          <div style={{ fontSize: 13, letterSpacing: '0.08em', color: 'var(--text)', minWidth: 0 }}>
            {name}
            <span style={{ marginLeft: 10, fontSize: 10, color: 'var(--muted)', border: '1px solid var(--border)', padding: '2px 6px', letterSpacing: '0.08em' }}>
              {typeLabel}
            </span>
          </div>
        </div>
        <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
          {creating ? (
            <button type="button" onClick={onDelete} style={{ fontSize: 11, color: 'var(--muted)', border: '1px solid var(--border)', padding: '4px 10px', background: 'transparent', cursor: 'pointer', fontFamily: 'inherit' }}>
              Cancel
            </button>
          ) : (
            <>
              {deleteConfirming && (
                <button type="button" onClick={onCancelDelete} style={{ fontSize: 11, color: 'var(--muted)', border: '1px solid var(--border)', padding: '4px 10px', background: 'transparent', cursor: 'pointer', fontFamily: 'inherit' }}>
                  Cancel
                </button>
              )}
              <button type="button" onClick={onDelete} style={{ fontSize: 11, color: deleteConfirming ? 'var(--danger)' : 'var(--muted)', border: '1px solid var(--border)', padding: '4px 10px', background: 'transparent', cursor: 'pointer', fontFamily: 'inherit' }}>
                {deleteConfirming ? 'Click again to confirm' : 'Remove'}
              </button>
            </>
          )}
        </div>
      </div>

      <div style={{ border: '1px solid var(--border)' }}>
        <ConfigRow label="Enabled">
          <label style={{ display: 'flex', alignItems: 'center', gap: 8, cursor: 'pointer' }}>
            <input type="checkbox" checked={enabled} onChange={e => setEnabled(e.target.checked)} />
            <span style={{ fontSize: 12, color: 'var(--muted)' }}>{enabled ? 'Active' : 'Disabled'}</span>
          </label>
        </ConfigRow>

        {/* Only show Name field for non-singleton types (e.g. Proxmox) */}
        {!isSingleton && (
          <ConfigRow label="Name">
            <input
              value={name}
              onChange={e => setName(e.target.value)}
              style={inputStyle}
            />
          </ConfigRow>
        )}

        {isWebhook && (
          <>
            <ConfigRow label="Endpoint">
              {endpointPath ? (
                <WebhookURLRow path={endpointPath} />
              ) : (
                <div style={{ fontSize: 12, color: 'var(--muted)' }}>No webhook endpoint registered for this source type</div>
              )}
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
                style={inputStyle}
              />
              {!creating && !editedFields.has('secret') && (
                <div style={{ fontSize: 10, color: 'var(--muted)', marginTop: 4 }}>
                  Leave blank to keep existing secret
                </div>
              )}
            </ConfigRow>
          </>
        )}

        {instance.type === 'systemd' && (
          <ConfigRow label="Watched Units">
            <SystemdUnitEditor
              units={unitsArray}
              onChange={units => setLocalCfg(c => ({ ...c, units }))}
            />
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
              <span style={{ fontSize: 12, color: 'var(--muted)' }}>Strip likely secrets from file change diffs</span>
            </label>
          </ConfigRow>
        )}

        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', padding: '12px 16px', borderTop: '1px solid var(--border)' }}>
          {saveError ? <span style={{ fontSize: 11, color: 'var(--danger)' }}>{saveError}</span> : <span />}
          <button
            type="button"
            disabled={saving}
            onClick={() => {
              const configPayload = Object.fromEntries(
                Object.entries(localCfg).filter(([key]) => creating || !(key === 'secret' && !editedFields.has(key))),
              )
              if (Array.isArray(configPayload.units)) {
                configPayload.units = (configPayload.units as unknown[])
                  .map(v => String(v).trim())
                  .filter(v => v !== '')
              }
              onSave({ name: isSingleton ? undefined : name, enabled, config: configPayload })
            }}
            style={{ fontSize: 11, border: '1px solid var(--border)', padding: '5px 14px', background: 'transparent', color: 'var(--text)', cursor: 'pointer', fontFamily: 'inherit' }}
          >
            {saving ? (creating ? 'Creating...' : 'Saving...') : (creating ? 'Create' : 'Save')}
          </button>
        </div>
      </div>
    </div>
  )
}

const inputStyle: React.CSSProperties = {
  background: 'var(--surface)',
  border: '1px solid var(--border)',
  color: 'var(--text)',
  fontFamily: 'inherit',
  fontSize: 12,
  padding: '5px 10px',
  width: '100%',
}

function WebhookURLRow({ path }: { path: string }) {
  const [copied, setCopied] = useState(false)
  const [copyError, setCopyError] = useState<string | null>(null)
  const url = `${window.location.origin}${path}`

  function handleCopy() {
    setCopyError(null)
    if (!navigator?.clipboard) { setCopyError('clipboard unavailable'); return }
    navigator.clipboard.writeText(url)
      .then(() => { setCopied(true); setTimeout(() => setCopied(false), 2000) })
      .catch(() => { setCopyError('copy failed') })
  }

  return (
    <>
      <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
        <span style={{ fontSize: 12, color: 'var(--text)', flex: 1, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{url}</span>
        <button
          type="button"
          onClick={handleCopy}
          style={{ background: 'transparent', border: '1px solid var(--border)', color: copied ? 'var(--accent)' : 'var(--muted)', padding: '4px 8px', fontFamily: 'inherit', fontSize: 11, cursor: 'pointer', display: 'flex', alignItems: 'center', gap: 4, flexShrink: 0 }}
        >
          {copied ? <CheckCircle size={12} /> : <Copy size={12} />}
          {copied ? 'COPIED' : 'COPY'}
        </button>
      </div>
      {copyError && <div style={{ fontSize: 10, color: 'var(--danger)', marginTop: 4 }}>{copyError}</div>}
    </>
  )
}

function SystemdUnitEditor({ units, onChange }: { units: string[]; onChange: (units: string[]) => void }) {
  const [newUnit, setNewUnit] = useState('')
  const inputRef = useRef<HTMLInputElement>(null)

  function addUnit() {
    const trimmed = newUnit.trim()
    if (!trimmed || units.includes(trimmed)) return
    onChange([...units, trimmed])
    setNewUnit('')
    inputRef.current?.focus()
  }

  function removeUnit(index: number) {
    onChange(units.filter((_, i) => i !== index))
  }

  return (
    <div>
      {units.length === 0 && (
        <div style={{ fontSize: 12, color: 'var(--muted)', marginBottom: 8 }}>No units configured.</div>
      )}
      {units.map((unit, i) => (
        <div key={i} style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '5px 0', borderBottom: '1px solid var(--border)' }}>
          <span style={{ fontSize: 12, color: 'var(--text)' }}>{unit}</span>
          <button
            type="button"
            onClick={() => removeUnit(i)}
            style={{ fontSize: 11, color: 'var(--muted)', background: 'transparent', border: 'none', cursor: 'pointer', fontFamily: 'inherit', padding: '0 4px' }}
          >
            Remove
          </button>
        </div>
      ))}
      <div style={{ display: 'flex', gap: 8, marginTop: 8 }}>
        <input
          ref={inputRef}
          value={newUnit}
          onChange={e => setNewUnit(e.target.value)}
          placeholder="nginx.service"
          onKeyDown={e => { if (e.key === 'Enter') { e.preventDefault(); addUnit() } }}
          style={{ ...inputStyle, flex: 1 }}
        />
        <button
          type="button"
          onClick={addUnit}
          disabled={!newUnit.trim()}
          style={{ fontSize: 11, border: '1px solid var(--border)', padding: '5px 12px', background: 'transparent', color: newUnit.trim() ? 'var(--text)' : 'var(--muted)', cursor: newUnit.trim() ? 'pointer' : 'default', fontFamily: 'inherit', flexShrink: 0 }}
        >
          Add
        </button>
      </div>
    </div>
  )
}

function draftInstanceFromInput(input: CreateSourceInput): DataSourceInstance {
  return {
    id: '__draft__',
    type: input.type,
    scope: input.scope,
    node_id: input.node_id,
    name: input.name,
    config: JSON.stringify(input.config),
    enabled: input.enabled,
    created_at: '',
    updated_at: '',
  }
}

function ConfigRow({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div style={{ display: 'flex', alignItems: 'flex-start', padding: '12px 16px', borderBottom: '1px solid var(--border)', gap: 16 }}>
      <div style={{ fontSize: 11, color: 'var(--muted)', width: 140, flexShrink: 0, paddingTop: 3 }}>{label}</div>
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
      <div style={{ marginBottom: 20 }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 10, fontSize: 13, letterSpacing: '0.08em', color: 'var(--text)' }}>
          <span style={{ color: getSourceCardColors('docker').text, display: 'inline-flex', alignItems: 'center', justifyContent: 'center' }}>
            <SourceIcon type="docker" size={20} strokeWidth={1.7} />
          </span>
          <span>
            Docker
            <span style={{ marginLeft: 10, fontSize: 10, color: 'var(--muted)', border: '1px solid var(--border)', padding: '2px 6px' }}>Always on</span>
          </span>
        </div>
        <div style={{ fontSize: 12, color: 'var(--muted)', marginTop: 6 }}>Container lifecycle events — runs on every agent automatically.</div>
      </div>

      <div style={{ border: '1px solid var(--border)' }}>
        <div style={{ padding: '10px 16px', borderBottom: '1px solid var(--border)', fontSize: 11, color: 'var(--muted)', letterSpacing: '0.1em', textTransform: 'uppercase' }}>
          Excluded Targets
        </div>
        {excludes.length === 0 && (
          <div style={{ padding: '12px 16px', fontSize: 12, color: 'var(--muted)' }}>No exclusions configured.</div>
        )}
        {excludes.map(ex => (
          <div key={ex.id} style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '8px 16px', borderBottom: '1px solid var(--border)', fontSize: 12 }}>
            <span>
              <span style={{ fontSize: 10, color: 'var(--muted)', marginRight: 8, border: '1px solid var(--border)', padding: '1px 5px' }}>{ex.type}</span>
              {ex.name}
            </span>
            <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
              {confirmingExcludeId === ex.id && (
                <button
                  type="button"
                  onClick={onCancelRemoveExclude}
                  disabled={deletingExcludeId === ex.id}
                  style={{ fontSize: 11, color: 'var(--muted)', background: 'transparent', border: 'none', cursor: 'pointer', fontFamily: 'inherit' }}
                >
                  Cancel
                </button>
              )}
              <button
                type="button"
                onClick={() => onRemoveExclude(ex.id)}
                disabled={deletingExcludeId === ex.id}
                style={{ fontSize: 11, color: confirmingExcludeId === ex.id ? 'var(--danger)' : 'var(--muted)', background: 'transparent', border: 'none', cursor: 'pointer', fontFamily: 'inherit' }}
              >
                {deletingExcludeId === ex.id ? 'Removing...' : confirmingExcludeId === ex.id ? 'Click again to confirm' : 'Remove'}
              </button>
            </div>
          </div>
        ))}
        <div style={{ padding: '10px 16px', display: 'flex', gap: 8, alignItems: 'center' }}>
          <select
            value={excludeType}
            onChange={e => onExcludeTypeChange(e.target.value as 'container' | 'stack')}
            style={{ background: 'var(--surface)', border: '1px solid var(--border)', color: 'var(--text)', fontFamily: 'inherit', fontSize: 12, padding: '5px 8px' }}
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
              if (!canAddExclude) { e.preventDefault(); return }
              onAddExclude()
            }}
            style={{ flex: 1, background: 'var(--surface)', border: '1px solid var(--border)', color: 'var(--text)', fontFamily: 'inherit', fontSize: 12, padding: '5px 8px' }}
          />
          <button
            type="button"
            onClick={onAddExclude}
            disabled={!canAddExclude}
            style={{ fontSize: 11, border: '1px solid var(--border)', padding: '5px 12px', background: 'transparent', color: canAddExclude ? 'var(--text)' : 'var(--muted)', cursor: canAddExclude ? 'pointer' : 'default', fontFamily: 'inherit' }}
          >
            {addingExclude ? 'Adding...' : 'Add'}
          </button>
        </div>
        {excludeError && <div style={{ padding: '0 16px 10px', fontSize: 11, color: 'var(--danger)' }}>{excludeError}</div>}
      </div>
    </div>
  )
}
