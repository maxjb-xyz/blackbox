import { useCallback, useEffect, useState } from 'react'
import { Circle } from 'lucide-react'
import type {
  DataSourceInstance,
  NodeSources,
  SourceTypeDef,
  CreateSourceInput,
  UpdateSourceInput,
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
} from '../api/client'
import type { ExcludedTarget } from '../api/client'
import SourceCatalog from './SourceCatalog'

type Selection =
  | { kind: 'server'; id: string }
  | { kind: 'node'; nodeName: string; id: string }
  | { kind: 'docker'; nodeName: string }
  | null

function parseConfig(config: string): Record<string, unknown> {
  try { return JSON.parse(config) } catch { return {} }
}

export default function DataSourcesGroup() {
  const [sources, setSources] = useState<{ server: DataSourceInstance[]; nodes: Record<string, NodeSources> } | null>(null)
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
    try { setExcludes(await listExcludedTargets()) } catch {}
  }, [])

  useEffect(() => {
    if (selection?.kind === 'docker') void loadExcludes()
  }, [selection, loadExcludes])

  const selectedInstance: DataSourceInstance | null = (() => {
    if (!sources) return null
    if (selection?.kind === 'server') return sources.server.find(s => s.id === selection.id) ?? null
    if (selection?.kind === 'node') {
      const ns = sources.nodes[selection.nodeName]
      return ns?.sources.find(s => s.id === selection.id) ?? null
    }
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
    if (!confirm('Remove this source?')) return
    try {
      await deleteSource(id)
      setSelection(null)
      await load()
    } catch (e) {
      setSaveError(e instanceof Error ? e.message : 'Delete failed')
    }
  }

  async function handleCreate(input: CreateSourceInput) {
    setSaving(true)
    setSaveError(null)
    try {
      const inst = await createSource(input)
      await load()
      setCatalogNode(null)
      if (input.scope === 'server') setSelection({ kind: 'server', id: inst.id })
      else setSelection({ kind: 'node', nodeName: input.node_id!, id: inst.id })
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
            onSave={(input) => handleSave(selectedInstance.id, input)}
            onDelete={() => handleDelete(selectedInstance.id)}
          />
        )}

        {selection?.kind === 'docker' && (
          <DockerPanel
            excludes={excludes}
            excludeInput={excludeInput}
            excludeType={excludeType}
            excludeError={excludeError}
            onExcludeInputChange={setExcludeInput}
            onExcludeTypeChange={setExcludeType}
            onAddExclude={async () => {
              setExcludeError(null)
              try {
                await createExcludedTarget({ type: excludeType, name: excludeInput.trim() })
                setExcludeInput('')
                await loadExcludes()
              } catch (e) {
                setExcludeError(e instanceof Error ? e.message : 'Failed to add')
              }
            }}
            onRemoveExclude={async (id) => {
              try { await deleteExcludedTarget(id); await loadExcludes() } catch {}
            }}
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
  const color = typeColor(type)
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
      <span style={{ width: 5, height: 5, background: color, flexShrink: 0, display: 'inline-block' }} />
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

function typeColor(type: string): string {
  const map: Record<string, string> = {
    docker: '#1a3a5a', systemd: '#3a2a5a', filewatcher: '#5a3a1a',
    proxmox: '#5a2a1a', webhook_uptime_kuma: '#1a5a3a', webhook_watchtower: '#1a5a3a',
  }
  return map[type] ?? 'var(--muted)'
}

function SourceConfigPanel({ instance, saving, saveError, onSave, onDelete }: {
  instance: DataSourceInstance
  saving: boolean
  saveError: string | null
  onSave: (input: UpdateSourceInput) => void
  onDelete: () => void
}) {
  const cfg = parseConfig(instance.config)
  const [enabled, setEnabled] = useState(instance.enabled)
  const [name, setName] = useState(instance.name)
  const [localCfg, setLocalCfg] = useState<Record<string, unknown>>(cfg)

  useEffect(() => {
    setEnabled(instance.enabled)
    setName(instance.name)
    setLocalCfg(parseConfig(instance.config))
  }, [instance.id, instance.enabled, instance.name, instance.config])

  const typeLabel = instance.type.replace(/_/g, ' ').replace(/\b\w/g, c => c.toUpperCase())
  const isWebhook = instance.type.startsWith('webhook_')
  const endpointPath = instance.type === 'webhook_uptime_kuma' ? '/api/webhooks/uptime' : '/api/webhooks/watchtower'

  return (
    <div>
      <div style={{ display: 'flex', alignItems: 'baseline', justifyContent: 'space-between', marginBottom: 16 }}>
        <div style={{ fontSize: 11, letterSpacing: '0.1em', color: 'var(--muted)', textTransform: 'uppercase' }}>
          {name}
          <span style={{ marginLeft: 8, fontSize: 9, color: 'var(--muted)', border: '1px solid var(--border)', padding: '1px 5px', letterSpacing: '0.08em' }}>
            {typeLabel}
          </span>
        </div>
        <button type="button" onClick={onDelete} style={{ fontSize: 10, color: 'var(--muted)', border: '1px solid var(--border)', padding: '3px 8px', background: 'transparent', cursor: 'pointer', fontFamily: 'inherit' }}>
          Remove
        </button>
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
              <div style={{ color: 'var(--muted)', fontSize: 11 }}>{endpointPath}</div>
              <div style={{ fontSize: 9, color: 'var(--muted)', marginTop: 3 }}>Set this URL in your monitoring tool</div>
            </ConfigRow>
            <ConfigRow label="Secret Token">
              <input
                type="password"
                value={String(localCfg.secret ?? '')}
                onChange={e => setLocalCfg(c => ({ ...c, secret: e.target.value }))}
                placeholder="Enter webhook secret"
                style={{ background: 'var(--surface)', border: '1px solid var(--border)', color: 'var(--text)', fontFamily: 'inherit', fontSize: 11, padding: '4px 8px', width: '100%' }}
              />
            </ConfigRow>
          </>
        )}

        {instance.type === 'systemd' && (
          <ConfigRow label="Watched Units">
            <textarea
              rows={4}
              value={((localCfg.units as string[]) ?? []).join('\n')}
              onChange={e => setLocalCfg(c => ({ ...c, units: e.target.value.split('\n').map(s => s.trim()).filter(Boolean) }))}
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

        {instance.type === 'proxmox' && (
          <>
            <ConfigRow label="Proxmox URL">
              <input
                value={String(localCfg.url ?? '')}
                onChange={e => setLocalCfg(c => ({ ...c, url: e.target.value }))}
                placeholder="https://pve01.example.com:8006"
                style={{ background: 'var(--surface)', border: '1px solid var(--border)', color: 'var(--text)', fontFamily: 'inherit', fontSize: 11, padding: '4px 8px', width: '100%' }}
              />
            </ConfigRow>
            <ConfigRow label="API Token">
              <input
                type="password"
                value={String(localCfg.api_token ?? '')}
                onChange={e => setLocalCfg(c => ({ ...c, api_token: e.target.value }))}
                placeholder="blackbox@pve!reader=uuid"
                style={{ background: 'var(--surface)', border: '1px solid var(--border)', color: 'var(--text)', fontFamily: 'inherit', fontSize: 11, padding: '4px 8px', width: '100%' }}
              />
            </ConfigRow>
            <ConfigRow label="Skip TLS Verify">
              <label style={{ display: 'flex', alignItems: 'center', gap: 8, cursor: 'pointer' }}>
                <input
                  type="checkbox"
                  checked={Boolean(localCfg.insecure_skip_verify)}
                  onChange={e => setLocalCfg(c => ({ ...c, insecure_skip_verify: e.target.checked }))}
                />
                <span style={{ fontSize: 11, color: 'var(--muted)' }}>Allow self-signed certificates</span>
              </label>
            </ConfigRow>
            <ConfigRow label="Poll Interval (s)">
              <input
                type="number"
                value={Number(localCfg.poll_interval_seconds ?? 10)}
                onChange={e => setLocalCfg(c => ({ ...c, poll_interval_seconds: Number(e.target.value) }))}
                min={5} max={300}
                style={{ background: 'var(--surface)', border: '1px solid var(--border)', color: 'var(--text)', fontFamily: 'inherit', fontSize: 11, padding: '4px 8px', width: 80 }}
              />
            </ConfigRow>
          </>
        )}

        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', padding: '10px 14px', borderTop: '1px solid var(--border)' }}>
          {saveError ? <span style={{ fontSize: 10, color: 'var(--danger)' }}>{saveError}</span> : <span />}
          <button
            type="button"
            disabled={saving}
            onClick={() => onSave({ name, enabled, config: localCfg })}
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
    <div style={{ display: 'flex', alignItems: 'flex-start', padding: '10px 14px', borderBottom: '1px solid #161616', gap: 12 }}>
      <div style={{ fontSize: 10, color: 'var(--muted)', width: 130, flexShrink: 0, paddingTop: 2 }}>{label}</div>
      <div style={{ flex: 1 }}>{children}</div>
    </div>
  )
}

function DockerPanel({ excludes, excludeInput, excludeType, excludeError, onExcludeInputChange, onExcludeTypeChange, onAddExclude, onRemoveExclude }: {
  excludes: ExcludedTarget[]
  excludeInput: string
  excludeType: 'container' | 'stack'
  excludeError: string | null
  onExcludeInputChange: (v: string) => void
  onExcludeTypeChange: (v: 'container' | 'stack') => void
  onAddExclude: () => void
  onRemoveExclude: (id: string) => void
}) {
  return (
    <div>
      <div style={{ marginBottom: 16 }}>
        <div style={{ fontSize: 11, letterSpacing: '0.1em', color: 'var(--muted)', textTransform: 'uppercase' }}>
          Docker
          <span style={{ marginLeft: 8, fontSize: 9, color: 'var(--muted)', border: '1px solid var(--border)', padding: '1px 5px' }}>Always on</span>
        </div>
        <div style={{ fontSize: 10, color: 'var(--muted)', marginTop: 3 }}>Container lifecycle events — runs on every agent automatically.</div>
      </div>

      <div style={{ border: '1px solid var(--border)' }}>
        <div style={{ padding: '10px 14px', borderBottom: '1px solid #161616', fontSize: 10, color: 'var(--muted)', letterSpacing: '0.08em', textTransform: 'uppercase' }}>
          Excluded Targets
        </div>
        {excludes.length === 0 && (
          <div style={{ padding: '10px 14px', fontSize: 10, color: 'var(--muted)' }}>No exclusions configured.</div>
        )}
        {excludes.map(ex => (
          <div key={ex.id} style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '7px 14px', borderBottom: '1px solid #161616', fontSize: 11 }}>
            <span>
              <span style={{ fontSize: 9, color: 'var(--muted)', marginRight: 8, border: '1px solid var(--border)', padding: '1px 4px' }}>{ex.type}</span>
              {ex.name}
            </span>
            <button type="button" onClick={() => onRemoveExclude(ex.id)} style={{ fontSize: 10, color: 'var(--muted)', background: 'transparent', border: 'none', cursor: 'pointer', fontFamily: 'inherit' }}>
              Remove
            </button>
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
            onKeyDown={e => { if (e.key === 'Enter') onAddExclude() }}
            style={{ flex: 1, background: 'var(--surface)', border: '1px solid var(--border)', color: 'var(--text)', fontFamily: 'inherit', fontSize: 11, padding: '4px 8px' }}
          />
          <button type="button" onClick={onAddExclude} style={{ fontSize: 10, border: '1px solid var(--border)', padding: '4px 10px', background: 'transparent', color: 'var(--text)', cursor: 'pointer', fontFamily: 'inherit' }}>
            Add
          </button>
        </div>
        {excludeError && <div style={{ padding: '0 14px 10px', fontSize: 10, color: 'var(--danger)' }}>{excludeError}</div>}
      </div>
    </div>
  )
}
