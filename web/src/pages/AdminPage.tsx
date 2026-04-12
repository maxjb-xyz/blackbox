import { useCallback, useEffect, useRef, useState } from 'react'
import type { CSSProperties } from 'react'
import { Navigate } from 'react-router-dom'
import {
  createAdminOIDCProvider,
  deleteAdminOIDCProvider,
  deleteAdminUser,
  fetchAdminConfig,
  fetchNodes,
  fetchSystemdSettings,
  forceLogoutUser,
  getOIDCPolicy,
  listAdminOIDCProviders,
  listAdminUsers,
  revokeInvite,
  setOIDCPolicy,
  updateAISettings,
  updateFileWatcherSettings,
  updateAdminOIDCProvider,
  updateAdminUser,
  updateSystemdSettings,
} from '../api/client'
import type { AdminUser, Node, OIDCProviderConfig } from '../api/client'
import { readErrorMessage } from '../api/errorUtils'
import { useSession } from '../session'
import PageHeader from '../components/PageHeader'
import { formatLocalDate, formatLocalTimestamp } from '../utils/time'

interface InviteCode {
  id: string
  code: string
  used: boolean
  created_at: string
  expires_at: string
}

interface OIDCProviderFormState {
  id: string
  name: string
  issuer: string
  client_id: string
  client_secret: string
  require_verified_email: boolean
  enabled: boolean
}

type Tab = 'invites' | 'users' | 'oidc' | 'settings' | 'systemd'
type OIDCPolicy = 'open' | 'existing_only' | 'invite_required'

const UNIT_SUFFIXES = ['.service','.socket','.device','.mount','.automount',
  '.swap','.target','.path','.timer','.slice','.scope']
const normalizeUnit = (name: string) =>
  UNIT_SUFFIXES.some(s => name.endsWith(s)) ? name : name + '.service'

const OIDC_POLICY_OPTIONS: Array<{ value: OIDCPolicy; description: string }> = [
  { value: 'open', description: 'Any OIDC user can sign in (new accounts created automatically)' },
  { value: 'existing_only', description: 'Only users with existing accounts can sign in via OIDC' },
  { value: 'invite_required', description: 'New OIDC users must have an invite code' },
]

function normalizeInvite(invite: Record<string, unknown>): InviteCode {
  return {
    id: String(invite.id ?? invite.ID ?? ''),
    code: String(invite.code ?? invite.Code ?? ''),
    used: Boolean(invite.used ?? invite.Used ?? invite.used_by ?? invite.UsedBy),
    created_at: String(invite.created_at ?? invite.CreatedAt ?? ''),
    expires_at: String(invite.expires_at ?? invite.ExpiresAt ?? ''),
  }
}

function normalizeOIDCPolicy(value: string | null | undefined): OIDCPolicy {
  if (value === 'existing_only' || value === 'invite_required') return value
  return 'open'
}

function formatInviteTimestamp(value: string): string {
  if (!value) return '-'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '-'
  return formatLocalTimestamp(date) || '-'
}

function emptyOIDCProviderForm(): OIDCProviderFormState {
  return {
    id: '',
    name: '',
    issuer: '',
    client_id: '',
    client_secret: '',
    require_verified_email: true,
    enabled: true,
  }
}

function oidcCallbackURL(providerID: string): string {
  const trimmed = providerID.trim()
  if (!trimmed) return ''
  return `${window.location.origin}/api/auth/oidc/${encodeURIComponent(trimmed)}/callback`
}

export default function AdminPage() {
  const { user } = useSession()
  const isAdmin = user?.is_admin === true
  const [tab, setTab] = useState<Tab>('invites')
  const [nodes, setNodes] = useState<Node[]>([])
  const [systemdSettings, setSystemdSettings] = useState<Record<string, string[]>>({})
  const [systemdInputs, setSystemdInputs] = useState<Record<string, string>>({})
  const [systemdSaving, setSystemdSaving] = useState<Record<string, boolean>>({})
  const [systemdSaveError, setSystemdSaveError] = useState<Record<string, string | null>>({})
  const [nodesError, setNodesError] = useState<string | null>(null)
  const [systemdError, setSystemdError] = useState<string | null>(null)

  const handleAddUnit = useCallback((nodeName: string) => {
    const units = systemdSettings[nodeName] ?? []
    const raw = (systemdInputs[nodeName] ?? '').trim()
    if (!raw) return
    const val = normalizeUnit(raw)
    if (units.map(normalizeUnit).includes(val)) return

    setSystemdSettings(prev => ({ ...prev, [nodeName]: [...units, val] }))
    setSystemdInputs(prev => ({ ...prev, [nodeName]: '' }))
    setSystemdSaveError(prev => ({ ...prev, [nodeName]: null }))
  }, [systemdInputs, systemdSettings])

  useEffect(() => {
    if (tab !== 'systemd') return
    void fetchNodes()
      .then(data => {
        setNodesError(null)
        setNodes(data)
      })
      .catch(err => {
        setNodesError(err instanceof Error ? err.message : 'Failed to fetch nodes')
      })
  }, [tab])

  useEffect(() => {
    if (tab !== 'systemd') return
    void fetchSystemdSettings()
      .then(data => {
        setSystemdError(null)
        setSystemdSettings(data)
      })
      .catch(err => {
        setSystemdError(err instanceof Error ? err.message : 'Failed to fetch systemd settings')
      })
  }, [tab])

  if (!isAdmin) return <Navigate to="/timeline" replace />

  return (
    <div>
      <PageHeader
        title="ADMIN /"
        actions={(['invites', 'users', 'oidc', 'settings', 'systemd'] as Tab[]).map(t => (
          <button
            key={t}
            onClick={() => setTab(t)}
            style={{
              background: 'none',
              border: 'none',
              borderBottom: tab === t ? '1px solid var(--accent)' : '1px solid transparent',
              color: tab === t ? 'var(--accent)' : 'var(--muted)',
              fontSize: '11px',
              letterSpacing: '0.1em',
              cursor: 'pointer',
              fontFamily: 'inherit',
              padding: '2px 0',
            }}
          >
            {t.toUpperCase()}
          </button>
        ))}
      />

      <div style={{ padding: 24, maxWidth: 960, margin: '0 auto' }}>
        {tab === 'invites' && <InvitesTab />}
        {tab === 'users' && <UsersTab currentUserId={user?.user_id ?? ''} />}
        {tab === 'oidc' && <OIDCTab />}
        {tab === 'settings' && <SettingsTab />}
        {tab === 'systemd' && (
          <div>
            <div
              style={{
                marginBottom: 16,
                fontSize: 11,
                color: 'var(--muted)',
                textTransform: 'uppercase',
                letterSpacing: '0.08em',
              }}
            >
              Per-Node Systemd Units
            </div>
            {nodesError && (
              <div style={{ color: 'var(--danger)', fontSize: 11, marginBottom: 8 }}>
                Failed to load nodes: {nodesError}
              </div>
            )}
            {systemdError && (
              <div style={{ color: 'var(--danger)', fontSize: 11, marginBottom: 8 }}>
                Failed to load systemd settings: {systemdError}
              </div>
            )}
            {nodes.length === 0 && (
              <div style={{ color: 'var(--muted)', fontSize: 12 }}>No nodes registered yet.</div>
            )}
            {[...nodes].sort((a, b) => a.name.localeCompare(b.name)).map(node => {
              const units = systemdSettings[node.name] ?? []
              const inputVal = systemdInputs[node.name] ?? ''
              const saving = systemdSaving[node.name] ?? false
              const saveError = systemdSaveError[node.name]

              return (
                <div
                  key={node.name}
                  style={{ border: '1px solid var(--border)', marginBottom: 12, padding: 12 }}
                >
                  <div
                    style={{
                      fontSize: 12,
                      color: 'var(--text)',
                      marginBottom: 8,
                      fontWeight: 'bold',
                    }}
                  >
                    {node.name}
                  </div>
                  {units.length === 0 && (
                    <div style={{ fontSize: 11, color: 'var(--muted)', marginBottom: 8 }}>
                      No units configured.
                    </div>
                  )}
                  {units.map(unit => (
                    <div
                      key={unit}
                      style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 4 }}
                    >
                      <span style={{ fontSize: 12, color: 'var(--text)', flex: 1 }}>{unit}</span>
                      <button
                        onClick={() => {
                          const next = units.filter(u => u !== unit)
                          setSystemdSettings(prev => ({ ...prev, [node.name]: next }))
                        }}
                        style={{
                          background: 'transparent',
                          border: 'none',
                          color: 'var(--muted)',
                          cursor: 'pointer',
                          fontSize: 14,
                          padding: '0 4px',
                        }}
                      >
                        ×
                      </button>
                    </div>
                  ))}
                  <div style={{ display: 'flex', gap: 8, marginTop: 8 }}>
                    <input
                      type="text"
                      placeholder="e.g. nginx.service"
                      value={inputVal}
                      onChange={e =>
                        setSystemdInputs(prev => ({ ...prev, [node.name]: e.target.value }))
                      }
                      onKeyDown={e => {
                        if (e.key === 'Enter') {
                          handleAddUnit(node.name)
                        }
                      }}
                      style={{
                        flex: 1,
                        background: 'var(--surface)',
                        border: '1px solid var(--border)',
                        color: 'var(--text)',
                        fontFamily: 'inherit',
                        fontSize: 12,
                        padding: '4px 8px',
                      }}
                    />
                    <button
                      onClick={() => handleAddUnit(node.name)}
                      style={{
                        background: 'transparent',
                        border: '1px solid var(--border)',
                        color: 'var(--muted)',
                        cursor: 'pointer',
                        fontFamily: 'inherit',
                        fontSize: 12,
                        padding: '4px 10px',
                      }}
                    >
                      Add
                    </button>
                  </div>
                  {saveError && (
                    <div style={{ color: 'var(--danger)', fontSize: 11, marginTop: 8 }}>
                      Failed to save {node.name}: {saveError}
                    </div>
                  )}
                  <button
                    disabled={saving}
                    onClick={async () => {
                      setSystemdSaveError(prev => ({ ...prev, [node.name]: null }))
                      setSystemdSaving(prev => ({ ...prev, [node.name]: true }))
                      try {
                        await updateSystemdSettings(node.name, systemdSettings[node.name] ?? [])
                      } catch (err) {
                        setSystemdSaveError(prev => ({
                          ...prev,
                          [node.name]:
                            err instanceof Error ? err.message : `Failed to update systemd settings for ${node.name}`,
                        }))
                      } finally {
                        setSystemdSaving(prev => ({ ...prev, [node.name]: false }))
                      }
                    }}
                    style={{
                      marginTop: 8,
                      background: 'transparent',
                      border: '1px solid var(--border)',
                      color: saving ? 'var(--muted)' : 'var(--text)',
                      cursor: saving ? 'not-allowed' : 'pointer',
                      fontFamily: 'inherit',
                      fontSize: 12,
                      padding: '4px 12px',
                    }}
                  >
                    {saving ? 'Saving…' : 'Save'}
                  </button>
                </div>
              )
            })}
          </div>
        )}
      </div>
    </div>
  )
}

function InvitesTab() {
  const [invites, setInvites] = useState<InviteCode[]>([])
  const [loading, setLoading] = useState(false)
  const [creating, setCreating] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [revokeLoadingId, setRevokeLoadingId] = useState<string | null>(null)
  const [copiedInviteId, setCopiedInviteId] = useState<string | null>(null)
  const copyResetRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  const loadInvites = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      const res = await fetch('/api/auth/invite', { credentials: 'same-origin' })
      if (!res.ok) {
        setError(await readErrorMessage(res, 'Failed to load invites'))
        return
      }
      const data = (await res.json()) as Record<string, unknown>[]
      setInvites(data.map(normalizeInvite))
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load invites')
    } finally {
      setLoading(false)
    }
  }, [])

  const createInvite = useCallback(async () => {
    setCreating(true)
    setError(null)
    try {
      const res = await fetch('/api/auth/invite', { method: 'POST', credentials: 'same-origin' })
      if (!res.ok) {
        setError(await readErrorMessage(res, 'Failed to create invite'))
        return
      }
      await loadInvites()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create invite')
    } finally {
      setCreating(false)
    }
  }, [loadInvites])

  useEffect(() => {
    void loadInvites()
  }, [loadInvites])

  useEffect(() => {
    return () => {
      if (copyResetRef.current) clearTimeout(copyResetRef.current)
    }
  }, [])

  async function handleCopyLink(invite: InviteCode) {
    setError(null)
    try {
      await navigator.clipboard.writeText(
        `${window.location.origin}/register?code=${encodeURIComponent(invite.code)}`,
      )
      const copiedId = invite.id || invite.code
      setCopiedInviteId(copiedId)
      if (copyResetRef.current) clearTimeout(copyResetRef.current)
      copyResetRef.current = setTimeout(() => {
        setCopiedInviteId(current => (current === copiedId ? null : current))
      }, 1500)
    } catch (err) {
      console.error('copy invite link failed', err)
      setError('Failed to copy invite link')
    }
  }

  async function handleRevoke(invite: InviteCode) {
    if (!invite.id) {
      setError('Invite id missing')
      return
    }
    setRevokeLoadingId(invite.id)
    setError(null)
    try {
      await revokeInvite(invite.id)
      await loadInvites()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to revoke invite')
    } finally {
      setRevokeLoadingId(null)
    }
  }

  return (
    <>
      <button
        onClick={() => void createInvite()}
        disabled={creating}
        style={{
          background: creating ? 'var(--border)' : 'var(--accent)',
          color: '#000',
          border: 'none',
          padding: '8px 16px',
          fontFamily: 'inherit',
          fontSize: '12px',
          fontWeight: 'bold',
          letterSpacing: '0.1em',
          cursor: creating ? 'not-allowed' : 'pointer',
          marginBottom: 16,
        }}
      >
        {creating ? 'CREATING...' : 'CREATE INVITE'}
      </button>

      {error && <div style={{ color: 'var(--danger)', fontSize: '12px', marginBottom: 12 }}>{error}</div>}

      {loading ? (
        <div style={{ color: 'var(--muted)', fontSize: '12px' }}>loading...</div>
      ) : invites.length === 0 ? (
        <div style={{ color: 'var(--muted)', fontSize: '12px' }}>no invite codes</div>
      ) : (
        <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: '12px' }}>
          <thead>
            <tr style={{ color: 'var(--muted)', fontSize: '10px', letterSpacing: '0.1em' }}>
              <th style={{ textAlign: 'left', padding: '4px 8px 4px 0', borderBottom: '1px solid var(--border)' }}>CODE</th>
              <th style={{ textAlign: 'left', padding: '4px 8px', borderBottom: '1px solid var(--border)' }}>STATUS</th>
              <th style={{ textAlign: 'left', padding: '4px 8px', borderBottom: '1px solid var(--border)' }}>EXPIRES</th>
              <th style={{ textAlign: 'left', padding: '4px 8px', borderBottom: '1px solid var(--border)' }}>CREATED</th>
              <th style={{ textAlign: 'left', padding: '4px 0 4px 8px', borderBottom: '1px solid var(--border)' }}>ACTIONS</th>
            </tr>
          </thead>
          <tbody>
            {invites.map((invite, index) => {
              const rowKey = invite.id || `${invite.code}-${invite.created_at || index}-${index}`
              const copied = copiedInviteId === (invite.id || invite.code)
              const revoking = revokeLoadingId === invite.id

              return (
                <tr key={rowKey}>
                  <td style={{ padding: '6px 8px 6px 0', color: 'var(--text)', fontFamily: 'inherit' }}>{invite.code}</td>
                  <td style={{ padding: '6px 8px', color: invite.used ? 'var(--muted)' : 'var(--accent)' }}>
                    {invite.used ? 'USED' : 'AVAILABLE'}
                  </td>
                  <td style={{ padding: '6px 8px', color: 'var(--muted)' }}>
                    {formatInviteTimestamp(invite.expires_at)}
                  </td>
                  <td style={{ padding: '6px 8px', color: 'var(--muted)' }}>
                    {formatInviteTimestamp(invite.created_at)}
                  </td>
                  <td style={{ padding: '6px 0 6px 8px' }}>
                    <div style={{ display: 'flex', gap: 8, alignItems: 'center', flexWrap: 'wrap' }}>
                      <button
                        onClick={() => void handleCopyLink(invite)}
                        style={{ ...actionBtnStyle, color: copied ? 'var(--accent)' : 'var(--muted)', borderColor: copied ? 'var(--accent)' : 'var(--border)' }}
                      >
                        {copied ? 'COPIED!' : 'COPY LINK'}
                      </button>
                      <button
                        onClick={() => void handleRevoke(invite)}
                        disabled={revoking}
                        style={{ ...actionBtnStyle, color: 'var(--danger)', borderColor: 'var(--danger)', cursor: revoking ? 'not-allowed' : 'pointer' }}
                      >
                        {revoking ? 'REVOKING...' : 'REVOKE'}
                      </button>
                    </div>
                  </td>
                </tr>
              )
            })}
          </tbody>
        </table>
      )}
    </>
  )
}

function UsersTab({ currentUserId }: { currentUserId: string }) {
  const [users, setUsers] = useState<AdminUser[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [confirmDeleteId, setConfirmDeleteId] = useState<string | null>(null)
  const [actionLoading, setActionLoading] = useState<string | null>(null)

  const load = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      setUsers(await listAdminUsers())
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load users')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    void load()
  }, [load])

  async function handleToggleAdmin(u: AdminUser) {
    setActionLoading(u.id)
    try {
      await updateAdminUser(u.id, !u.is_admin)
      await load()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to update user')
    } finally {
      setActionLoading(null)
    }
  }

  async function handleForceLogout(u: AdminUser) {
    setActionLoading(u.id + '-logout')
    try {
      await forceLogoutUser(u.id)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to force logout')
    } finally {
      setActionLoading(null)
    }
  }

  async function handleDelete(u: AdminUser) {
    if (actionLoading !== null) return
    setActionLoading(u.id + '-delete')
    try {
      await deleteAdminUser(u.id)
      await load()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to delete user')
    } finally {
      setActionLoading(null)
      setConfirmDeleteId(null)
    }
  }

  if (loading) return <div style={{ color: 'var(--muted)', fontSize: '12px' }}>loading...</div>

  return (
    <>
      {error && <div style={{ color: 'var(--danger)', fontSize: '12px', marginBottom: 12 }}>{error}</div>}
      <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: '12px' }}>
        <thead>
          <tr style={{ color: 'var(--muted)', fontSize: '10px', letterSpacing: '0.1em' }}>
            <th style={{ textAlign: 'left', padding: '4px 8px 4px 0', borderBottom: '1px solid var(--border)' }}>USERNAME</th>
            <th style={{ textAlign: 'left', padding: '4px 8px', borderBottom: '1px solid var(--border)' }}>ROLE</th>
            <th style={{ textAlign: 'left', padding: '4px 8px', borderBottom: '1px solid var(--border)' }}>CREATED</th>
            <th style={{ textAlign: 'left', padding: '4px 0 4px 8px', borderBottom: '1px solid var(--border)' }}>ACTIONS</th>
          </tr>
        </thead>
        <tbody>
          {users.map(u => (
            <tr key={u.id}>
              <td style={{ padding: '8px 8px 8px 0', color: 'var(--text)' }}>{u.username}</td>
              <td style={{ padding: '8px', color: u.is_admin ? 'var(--accent)' : 'var(--muted)' }}>
                {u.is_admin ? 'ADMIN' : 'USER'}
              </td>
              <td style={{ padding: '8px', color: 'var(--muted)' }}>
                {u.created_at ? formatLocalDate(u.created_at) || '-' : '-'}
              </td>
              <td style={{ padding: '8px 0 8px 8px' }}>
                {u.id !== currentUserId && (
                  <div style={{ display: 'flex', gap: 8, alignItems: 'center', flexWrap: 'wrap' }}>
                    <button
                      onClick={() => void handleToggleAdmin(u)}
                      disabled={actionLoading === u.id}
                      style={actionBtnStyle}
                    >
                      {u.is_admin ? 'DEMOTE' : 'PROMOTE'}
                    </button>
                    <button
                      onClick={() => void handleForceLogout(u)}
                      disabled={actionLoading === u.id + '-logout'}
                      style={actionBtnStyle}
                    >
                      FORCE LOGOUT
                    </button>
                    {confirmDeleteId === u.id ? (
                      <>
                        <span style={{ color: 'var(--danger)', fontSize: '11px' }}>CONFIRM?</span>
                        <button
                          onClick={() => void handleDelete(u)}
                          disabled={actionLoading !== null}
                          style={{ ...actionBtnStyle, color: 'var(--danger)', borderColor: 'var(--danger)' }}
                        >
                          YES
                        </button>
                        <button onClick={() => setConfirmDeleteId(null)} disabled={actionLoading !== null} style={actionBtnStyle}>NO</button>
                      </>
                    ) : (
                      <button
                        onClick={() => setConfirmDeleteId(u.id)}
                        disabled={actionLoading === u.id + '-delete'}
                        style={{ ...actionBtnStyle, color: 'var(--danger)', borderColor: 'var(--danger)' }}
                      >
                        DELETE
                      </button>
                    )}
                  </div>
                )}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </>
  )
}

function OIDCTab() {
  const [providers, setProviders] = useState<OIDCProviderConfig[]>([])
  const [providersLoading, setProvidersLoading] = useState(false)
  const [providerFormMode, setProviderFormMode] = useState<'create' | 'edit' | null>(null)
  const [editingProviderId, setEditingProviderId] = useState<string | null>(null)
  const [providerForm, setProviderForm] = useState<OIDCProviderFormState>(emptyOIDCProviderForm)
  const [providerSaving, setProviderSaving] = useState(false)
  const [providerDeletingId, setProviderDeletingId] = useState<string | null>(null)
  const [providerError, setProviderError] = useState<string | null>(null)
  const [providerMessage, setProviderMessage] = useState<string | null>(null)

  const [policy, setPolicy] = useState<OIDCPolicy>('open')
  const [policyLoading, setPolicyLoading] = useState(false)
  const [policySaving, setPolicySaving] = useState(false)
  const [policyError, setPolicyError] = useState<string | null>(null)
  const [policyMessage, setPolicyMessage] = useState<string | null>(null)

  const loadProviders = useCallback(async () => {
    setProvidersLoading(true)
    setProviderError(null)
    try {
      setProviders(await listAdminOIDCProviders())
    } catch (err) {
      setProviderError(err instanceof Error ? err.message : 'Failed to load OIDC providers')
    } finally {
      setProvidersLoading(false)
    }
  }, [])

  const loadPolicy = useCallback(async () => {
    setPolicyLoading(true)
    setPolicyError(null)
    try {
      const data = await getOIDCPolicy()
      setPolicy(normalizeOIDCPolicy(data.policy))
    } catch (err) {
      setPolicyError(err instanceof Error ? err.message : 'Failed to load OIDC policy')
    } finally {
      setPolicyLoading(false)
    }
  }, [])

  useEffect(() => {
    void loadProviders()
    void loadPolicy()
  }, [loadPolicy, loadProviders])

  function openCreateForm() {
    setProviderFormMode('create')
    setEditingProviderId(null)
    setProviderForm(emptyOIDCProviderForm())
    setProviderError(null)
    setProviderMessage(null)
  }

  function openEditForm(provider: OIDCProviderConfig) {
    setProviderFormMode('edit')
    setEditingProviderId(provider.id)
    setProviderForm({
      id: provider.id,
      name: provider.name,
      issuer: provider.issuer,
      client_id: provider.client_id,
      client_secret: '',
      require_verified_email: provider.require_verified_email,
      enabled: provider.enabled,
    })
    setProviderError(null)
    setProviderMessage(null)
  }

  function closeProviderForm() {
    setProviderFormMode(null)
    setEditingProviderId(null)
    setProviderForm(emptyOIDCProviderForm())
  }

  async function handleProviderSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (!providerFormMode) return

    const providerID = providerForm.id.trim()
    const name = providerForm.name.trim()
    const issuer = providerForm.issuer.trim()
    const clientID = providerForm.client_id.trim()
    const clientSecret = providerForm.client_secret.trim()
    const redirectURL = oidcCallbackURL(providerID)

    if (!providerID || !name || !issuer || !clientID || !redirectURL) {
      setProviderError('Provider ID, name, issuer, and client ID are required')
      return
    }
    if (providerFormMode === 'create' && !clientSecret) {
      setProviderError('Client secret is required')
      return
    }

    setProviderSaving(true)
    setProviderError(null)
    setProviderMessage(null)

    try {
      if (providerFormMode === 'create') {
        await createAdminOIDCProvider({
          id: providerID,
          name,
          issuer,
          client_id: clientID,
          client_secret: clientSecret,
          redirect_url: redirectURL,
          require_verified_email: providerForm.require_verified_email,
          enabled: providerForm.enabled,
        })
        setProviderMessage('OIDC provider created')
      } else if (editingProviderId) {
        const updatePayload = {
          id: providerID,
          name,
          issuer,
          client_id: clientID,
          redirect_url: redirectURL,
          require_verified_email: providerForm.require_verified_email,
          enabled: providerForm.enabled,
          ...(clientSecret ? { client_secret: clientSecret } : {}),
        }
        await updateAdminOIDCProvider(editingProviderId, updatePayload)
        setProviderMessage('OIDC provider updated')
      }

      closeProviderForm()
      await loadProviders()
    } catch (err) {
      setProviderError(err instanceof Error ? err.message : 'Failed to save OIDC provider')
    } finally {
      setProviderSaving(false)
    }
  }

  async function handleDeleteProvider(provider: OIDCProviderConfig) {
    if (!window.confirm(`Delete OIDC provider "${provider.name}"?`)) return

    setProviderDeletingId(provider.id)
    setProviderError(null)
    setProviderMessage(null)

    try {
      await deleteAdminOIDCProvider(provider.id)
      if (editingProviderId === provider.id) closeProviderForm()
      await loadProviders()
      setProviderMessage('OIDC provider deleted')
    } catch (err) {
      setProviderError(err instanceof Error ? err.message : 'Failed to delete OIDC provider')
    } finally {
      setProviderDeletingId(null)
    }
  }

  async function handleSavePolicy() {
    setPolicySaving(true)
    setPolicyError(null)
    setPolicyMessage(null)
    try {
      await setOIDCPolicy(policy)
      setPolicyMessage('OIDC policy updated')
    } catch (err) {
      setPolicyError(err instanceof Error ? err.message : 'Failed to update OIDC policy')
    } finally {
      setPolicySaving(false)
    }
  }

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 24 }}>
      <section style={panelStyle}>
        <div style={panelHeaderStyle}>
          <span style={panelLabelStyle}>PROVIDERS</span>
          <button type="button" onClick={openCreateForm} style={actionBtnStyle}>
            ADD PROVIDER
          </button>
        </div>

        {providerMessage && <div style={{ color: 'var(--success)', fontSize: '12px', marginBottom: 12 }}>{providerMessage}</div>}
        {providerError && <div style={{ color: 'var(--danger)', fontSize: '12px', marginBottom: 12 }}>{providerError}</div>}

        {providerFormMode && (
          <form onSubmit={handleProviderSubmit} style={{ border: '1px solid var(--border)', padding: 16, marginBottom: 16 }}>
            <div style={{ display: 'grid', gap: 12, gridTemplateColumns: 'repeat(auto-fit, minmax(220px, 1fr))' }}>
              <label style={fieldWrapperStyle}>
                <span style={fieldLabelStyle}>PROVIDER ID</span>
                <input
                  type="text"
                  value={providerForm.id}
                  onChange={e => setProviderForm(prev => ({ ...prev, id: e.target.value }))}
                  required
                  style={inputStyle}
                />
              </label>

              <label style={fieldWrapperStyle}>
                <span style={fieldLabelStyle}>NAME</span>
                <input
                  type="text"
                  value={providerForm.name}
                  onChange={e => setProviderForm(prev => ({ ...prev, name: e.target.value }))}
                  required
                  style={inputStyle}
                />
              </label>

              <label style={fieldWrapperStyle}>
                <span style={fieldLabelStyle}>ISSUER</span>
                <input
                  type="text"
                  value={providerForm.issuer}
                  onChange={e => setProviderForm(prev => ({ ...prev, issuer: e.target.value }))}
                  required
                  style={inputStyle}
                />
              </label>

              <label style={fieldWrapperStyle}>
                <span style={fieldLabelStyle}>CLIENT ID</span>
                <input
                  type="text"
                  value={providerForm.client_id}
                  onChange={e => setProviderForm(prev => ({ ...prev, client_id: e.target.value }))}
                  required
                  style={inputStyle}
                />
              </label>

              <label style={fieldWrapperStyle}>
                <span style={fieldLabelStyle}>CLIENT SECRET</span>
                <input
                  type="password"
                  value={providerForm.client_secret}
                  onChange={e => setProviderForm(prev => ({ ...prev, client_secret: e.target.value }))}
                  required={providerFormMode === 'create'}
                  placeholder={providerFormMode === 'edit' ? '***' : ''}
                  style={inputStyle}
                />
              </label>

              <label style={{ ...fieldWrapperStyle, gridColumn: '1 / -1' }}>
                <span style={fieldLabelStyle}>CALLBACK URL</span>
                <input
                  type="text"
                  value={oidcCallbackURL(providerForm.id)}
                  readOnly
                  style={{ ...inputStyle, color: 'var(--muted)' }}
                />
                <span style={{ color: 'var(--muted)', fontSize: '11px' }}>
                  Update the provider ID to change the callback URL you register with your identity provider.
                </span>
              </label>

              <label style={{ ...fieldWrapperStyle, gridColumn: '1 / -1', flexDirection: 'row', alignItems: 'center', gap: 8 }}>
                <input
                  type="checkbox"
                  checked={providerForm.require_verified_email}
                  onChange={e => setProviderForm(prev => ({ ...prev, require_verified_email: e.target.checked }))}
                />
                <span style={{ color: 'var(--muted)', fontSize: '11px', letterSpacing: '0.05em' }}>REQUIRE VERIFIED EMAIL FOR AUTO-LINKING</span>
              </label>

              <label style={{ ...fieldWrapperStyle, gridColumn: '1 / -1', flexDirection: 'row', alignItems: 'center', gap: 8 }}>
                <input
                  type="checkbox"
                  checked={providerForm.enabled}
                  onChange={e => setProviderForm(prev => ({ ...prev, enabled: e.target.checked }))}
                />
                <span style={{ color: 'var(--muted)', fontSize: '11px', letterSpacing: '0.05em' }}>ENABLED</span>
              </label>
            </div>

            <div style={{ display: 'flex', gap: 8, marginTop: 16, flexWrap: 'wrap' }}>
              <button
                type="submit"
                disabled={providerSaving}
                style={{
                  background: providerSaving ? 'var(--border)' : 'var(--accent)',
                  color: '#000',
                  border: 'none',
                  padding: '8px 16px',
                  fontFamily: 'inherit',
                  fontSize: '12px',
                  fontWeight: 'bold',
                  letterSpacing: '0.1em',
                  cursor: providerSaving ? 'not-allowed' : 'pointer',
                }}
              >
                {providerSaving ? 'SAVING...' : providerFormMode === 'create' ? 'CREATE PROVIDER' : 'SAVE CHANGES'}
              </button>
              <button type="button" onClick={closeProviderForm} style={actionBtnStyle}>
                CANCEL
              </button>
            </div>
          </form>
        )}

        {providersLoading ? (
          <div style={{ color: 'var(--muted)', fontSize: '12px' }}>loading...</div>
        ) : providers.length === 0 ? (
          <div style={{ color: 'var(--muted)', fontSize: '12px' }}>no oidc providers configured</div>
        ) : (
          <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: '12px' }}>
            <thead>
              <tr style={{ color: 'var(--muted)', fontSize: '10px', letterSpacing: '0.1em' }}>
                <th style={{ textAlign: 'left', padding: '4px 8px 4px 0', borderBottom: '1px solid var(--border)' }}>NAME</th>
                <th style={{ textAlign: 'left', padding: '4px 8px', borderBottom: '1px solid var(--border)' }}>ID</th>
                <th style={{ textAlign: 'left', padding: '4px 8px', borderBottom: '1px solid var(--border)' }}>ISSUER</th>
                <th style={{ textAlign: 'left', padding: '4px 8px', borderBottom: '1px solid var(--border)' }}>STATUS</th>
                <th style={{ textAlign: 'left', padding: '4px 8px', borderBottom: '1px solid var(--border)' }}>CREATED</th>
                <th style={{ textAlign: 'left', padding: '4px 0 4px 8px', borderBottom: '1px solid var(--border)' }}>ACTIONS</th>
              </tr>
            </thead>
            <tbody>
              {providers.map(provider => (
                <tr key={provider.id}>
                  <td style={{ padding: '8px 8px 8px 0', color: 'var(--text)' }}>{provider.name}</td>
                  <td style={{ padding: '8px', color: 'var(--muted)' }}>{provider.id}</td>
                  <td style={{ padding: '8px', color: 'var(--muted)', maxWidth: 320 }}>
                    <div style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }} title={provider.issuer}>
                      {provider.issuer}
                    </div>
                  </td>
                  <td style={{ padding: '8px', color: provider.enabled ? 'var(--success)' : 'var(--muted)' }}>
                    {provider.enabled ? 'ENABLED' : 'DISABLED'}
                  </td>
                  <td style={{ padding: '8px', color: 'var(--muted)' }}>
                    {formatInviteTimestamp(provider.created_at)}
                  </td>
                  <td style={{ padding: '8px 0 8px 8px' }}>
                    <div style={{ display: 'flex', gap: 8, alignItems: 'center', flexWrap: 'wrap' }}>
                      <button type="button" onClick={() => openEditForm(provider)} style={actionBtnStyle}>
                        EDIT
                      </button>
                      <button
                        type="button"
                        onClick={() => void handleDeleteProvider(provider)}
                        disabled={providerDeletingId === provider.id}
                        style={{ ...actionBtnStyle, color: 'var(--danger)', borderColor: 'var(--danger)', cursor: providerDeletingId === provider.id ? 'not-allowed' : 'pointer' }}
                      >
                        {providerDeletingId === provider.id ? 'DELETING...' : 'DELETE'}
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </section>

      <section style={panelStyle}>
        <div style={panelHeaderStyle}>
          <span style={panelLabelStyle}>ACCESS POLICY</span>
        </div>

        {policyMessage && <div style={{ color: 'var(--success)', fontSize: '12px', marginBottom: 12 }}>{policyMessage}</div>}
        {policyError && <div style={{ color: 'var(--danger)', fontSize: '12px', marginBottom: 12 }}>{policyError}</div>}

        {policyLoading ? (
          <div style={{ color: 'var(--muted)', fontSize: '12px' }}>loading...</div>
        ) : (
          <>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 12, marginBottom: 16 }}>
              {OIDC_POLICY_OPTIONS.map(option => (
                <label
                  key={option.value}
                  style={{
                    border: '1px solid var(--border)',
                    padding: '10px 12px',
                    cursor: 'pointer',
                    background: policy === option.value ? 'var(--surface)' : 'transparent',
                  }}
                >
                  <div style={{ display: 'flex', gap: 10, alignItems: 'flex-start' }}>
                    <input
                      type="radio"
                      name="oidc-policy"
                      checked={policy === option.value}
                      onChange={() => setPolicy(option.value)}
                      style={{ marginTop: 2 }}
                    />
                    <div>
                      <div style={{ color: policy === option.value ? 'var(--accent)' : 'var(--text)', fontSize: '11px', letterSpacing: '0.1em' }}>
                        {option.value.toUpperCase()}
                      </div>
                      <div style={{ color: 'var(--muted)', fontSize: '12px', marginTop: 4, lineHeight: '1.5' }}>
                        {option.description}
                      </div>
                    </div>
                  </div>
                </label>
              ))}
            </div>

            <button
              type="button"
              onClick={() => void handleSavePolicy()}
              disabled={policySaving}
              style={{
                background: policySaving ? 'var(--border)' : 'var(--accent)',
                color: '#000',
                border: 'none',
                padding: '8px 16px',
                fontFamily: 'inherit',
                fontSize: '12px',
                fontWeight: 'bold',
                letterSpacing: '0.1em',
                cursor: policySaving ? 'not-allowed' : 'pointer',
              }}
            >
              {policySaving ? 'SAVING...' : 'SAVE'}
            </button>
          </>
        )}
      </section>
    </div>
  )
}

function SettingsTab() {
  const [redactSecrets, setRedactSecrets] = useState(true)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [message, setMessage] = useState<string | null>(null)
  const [aiProvider, setAIProvider] = useState<'ollama' | 'openai_compat'>('ollama')
  const [aiURL, setAIURL] = useState('')
  const [aiModel, setAIModel] = useState('')
  const [aiAPIKey, setAIAPIKey] = useState('')
  const [aiAPIKeySet, setAIAPIKeySet] = useState(false)
  const [aiMode, setAIMode] = useState<'analysis' | 'enhanced'>('analysis')
  const [aiSaving, setAISaving] = useState(false)
  const [aiError, setAIError] = useState<string | null>(null)
  const [aiSuccess, setAISuccess] = useState(false)
  const [initialLoaded, setInitialLoaded] = useState(false)
  const aiSuccessTimerRef = useRef<number | null>(null)

  const loadSettings = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      const config = await fetchAdminConfig()
      setRedactSecrets(config.file_watcher_redact_secrets)
      setAIProvider(config.ai_provider ?? 'ollama')
      setAIURL(config.ai_url ?? '')
      setAIModel(config.ai_model ?? '')
      setAIAPIKeySet(config.ai_api_key_set ?? false)
      setAIMode(config.ai_mode ?? 'analysis')
      setInitialLoaded(true)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load settings')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    void loadSettings()
  }, [loadSettings])

  useEffect(() => {
    return () => {
      if (aiSuccessTimerRef.current !== null) {
        window.clearTimeout(aiSuccessTimerRef.current)
      }
    }
  }, [])

  async function handleSave() {
    if (!initialLoaded) return
    setSaving(true)
    setError(null)
    setMessage(null)
    try {
      const updated = await updateFileWatcherSettings(redactSecrets)
      setRedactSecrets(updated.redact_secrets)
      setMessage('File watcher settings updated')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save settings')
    } finally {
      setSaving(false)
    }
  }

  async function saveAISettings(e: React.FormEvent) {
    e.preventDefault()
    if (!initialLoaded) return
    setAISaving(true)
    setAIError(null)
    setAISuccess(false)
    try {
      await updateAISettings(aiProvider, aiURL, aiModel, aiAPIKey, aiMode)
      if (aiSuccessTimerRef.current !== null) {
        window.clearTimeout(aiSuccessTimerRef.current)
      }
      setAISuccess(true)
      aiSuccessTimerRef.current = window.setTimeout(() => {
        setAISuccess(false)
        aiSuccessTimerRef.current = null
      }, 2500)
    } catch (err) {
      setAIError(err instanceof Error ? err.message : 'Save failed')
    } finally {
      setAISaving(false)
    }
  }

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 24 }}>
      <section style={panelStyle}>
        <div style={panelHeaderStyle}>
          <span style={panelLabelStyle}>FILE WATCHER</span>
        </div>

        {message && <div style={{ color: 'var(--success)', fontSize: '12px', marginBottom: 12 }}>{message}</div>}
        {error && <div style={{ color: 'var(--danger)', fontSize: '12px', marginBottom: 12 }}>{error}</div>}

        {loading ? (
          <div style={{ color: 'var(--muted)', fontSize: '12px' }}>loading...</div>
        ) : !initialLoaded ? (
          <div style={{ color: 'var(--danger)', fontSize: '12px' }}>
            Load settings before editing file watcher configuration.
          </div>
        ) : (
          <>
            <label
              style={{
                display: 'flex',
                gap: 10,
                alignItems: 'flex-start',
                border: '1px solid var(--border)',
                padding: '12px 14px',
                marginBottom: 16,
              }}
            >
              <input
                type="checkbox"
                checked={redactSecrets}
                onChange={e => setRedactSecrets(e.target.checked)}
                disabled={!initialLoaded || saving}
                style={{ marginTop: 2 }}
              />
              <div>
                <div style={{ color: 'var(--text)', fontSize: '11px', letterSpacing: '0.1em' }}>REDACT SECRETS IN FILE DIFFS</div>
                <div style={{ color: 'var(--muted)', fontSize: '12px', marginTop: 4, lineHeight: '1.5' }}>
                  When enabled, obvious secret-bearing keys such as tokens, passwords, and client secrets are masked before file diffs are uploaded by agents.
                </div>
                <div style={{ color: 'var(--muted)', fontSize: '12px', marginTop: 8, lineHeight: '1.5' }}>
                  Agents refresh this setting periodically. Changing it affects newly captured file diffs, not historical entries already stored in Blackbox.
                </div>
              </div>
            </label>

            <button
              type="button"
              onClick={() => void handleSave()}
              disabled={!initialLoaded || saving}
              style={{
                background: !initialLoaded || saving ? 'var(--border)' : 'var(--accent)',
                color: '#000',
                border: 'none',
                padding: '8px 16px',
                fontFamily: 'inherit',
                fontSize: '12px',
                fontWeight: 'bold',
                letterSpacing: '0.1em',
                cursor: !initialLoaded || saving ? 'not-allowed' : 'pointer',
              }}
            >
              {saving ? 'SAVING...' : 'SAVE'}
            </button>
          </>
        )}
      </section>

      <section style={panelStyle}>
        <h3 style={{ fontSize: 11, color: 'var(--muted)', letterSpacing: '0.1em', margin: '0 0 12px 0' }}>
          AI PROVIDER
        </h3>
        {!initialLoaded ? (
          <div style={{ color: loading ? 'var(--muted)' : 'var(--danger)', fontSize: 12 }}>
            {loading ? 'loading...' : 'Load settings before editing AI provider configuration.'}
          </div>
        ) : (
          <form onSubmit={saveAISettings} style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
            <div style={{ fontSize: 11, color: 'var(--muted)', marginBottom: 8 }}>PROVIDER</div>
            <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap', marginBottom: 12 }}>
              {(['ollama', 'openai_compat'] as const).map(p => {
                const selected = aiProvider === p
                return (
                  <button
                    key={p}
                    type="button"
                    onClick={() => { setAIProvider(p); setAISuccess(false) }}
                    disabled={aiSaving}
                    style={{
                      padding: '4px 12px',
                      fontSize: 12,
                      border: '1px solid',
                      borderColor: selected ? OLLAMA_MODE_ACTIVE_COLOR : 'var(--border)',
                      background: selected ? 'rgba(168, 85, 247, 0.1)' : 'transparent',
                      color: selected ? OLLAMA_MODE_ACTIVE_COLOR : 'var(--muted)',
                      cursor: aiSaving ? 'not-allowed' : 'pointer',
                      fontFamily: 'inherit',
                    }}
                  >
                    {p === 'ollama' ? 'OLLAMA' : 'OPENAI COMPATIBLE'}
                  </button>
                )
              })}
            </div>
            <label style={{ fontSize: 11, color: 'var(--muted)', display: 'block', marginBottom: 8 }}>
              {aiProvider === 'openai_compat' ? 'BASE URL' : 'OLLAMA URL'}
              <input
                type="url"
                value={aiURL}
                onChange={e => { setAIURL(e.target.value); setAISuccess(false) }}
                placeholder={aiProvider === 'openai_compat' ? 'https://api.openai.com' : 'http://192.168.1.10:11434'}
                disabled={!initialLoaded || aiSaving}
                style={{ display: 'block', width: '100%', marginTop: 4, ...FILTER_CONTROL_STYLE }}
              />
            </label>
            {aiProvider === 'openai_compat' && (
              <label style={{ fontSize: 11, color: 'var(--muted)', display: 'block', marginBottom: 8 }}>
                API KEY
                <input
                  type="password"
                  value={aiAPIKey}
                  onChange={e => { setAIAPIKey(e.target.value); setAISuccess(false) }}
                  placeholder={aiAPIKeySet ? '[key set — leave blank to keep]' : 'sk-...'}
                  disabled={!initialLoaded || aiSaving}
                  style={{ display: 'block', width: '100%', marginTop: 4, ...FILTER_CONTROL_STYLE }}
                />
              </label>
            )}
            <label style={{ fontSize: 11, color: 'var(--muted)', display: 'block', marginBottom: 8 }}>
              MODEL
              <input
                type="text"
                value={aiModel}
                onChange={e => { setAIModel(e.target.value); setAISuccess(false) }}
                placeholder={aiProvider === 'openai_compat' ? 'gpt-4o-mini' : 'llama3.2'}
                disabled={!initialLoaded || aiSaving}
                style={{ display: 'block', width: '100%', marginTop: 4, ...FILTER_CONTROL_STYLE }}
              />
            </label>
            {aiURL.trim() !== '' && aiModel.trim() !== '' && (
              <div style={{ marginTop: 4 }}>
                <div style={{ fontSize: 11, color: 'var(--muted)', marginBottom: 4 }}>MODE</div>
                <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
                  {(['analysis', 'enhanced'] as const).map(mode => {
                    const selected = aiMode === mode
                    return (
                      <button
                        key={mode}
                        type="button"
                        onClick={() => { setAIMode(mode); setAISuccess(false) }}
                        disabled={aiSaving}
                        style={{
                          padding: '4px 12px',
                          fontSize: 12,
                          border: '1px solid',
                          borderColor: selected ? OLLAMA_MODE_ACTIVE_COLOR : 'var(--border)',
                          background: selected ? 'rgba(168, 85, 247, 0.1)' : 'transparent',
                          color: selected ? OLLAMA_MODE_ACTIVE_COLOR : 'var(--muted)',
                          cursor: aiSaving ? 'not-allowed' : 'pointer',
                          fontFamily: 'inherit',
                        }}
                      >
                        {mode.toUpperCase()}
                      </button>
                    )
                  })}
                </div>
                {aiMode === 'enhanced' && (
                  <div style={{ marginTop: 8, fontSize: 11, color: 'var(--muted)' }}>
                    Enhanced mode: AI correlates timeline events after deterministic links settle.
                  </div>
                )}
              </div>
            )}
            {aiError && <div style={{ color: 'var(--danger)', fontSize: 11 }}>{aiError}</div>}
            {aiSuccess && <div style={{ color: 'var(--success)', fontSize: 11 }}>saved</div>}
            <button
              type="submit"
              disabled={!initialLoaded || aiSaving}
              style={{ alignSelf: 'flex-start', fontSize: 11, padding: '4px 12px', cursor: 'pointer', fontFamily: 'inherit' }}
            >
              {aiSaving ? 'SAVING...' : 'SAVE'}
            </button>
          </form>
        )}
      </section>
    </div>
  )
}

const panelStyle: CSSProperties = {
  border: '1px solid var(--border)',
  padding: 16,
}

const panelHeaderStyle: CSSProperties = {
  display: 'flex',
  justifyContent: 'space-between',
  alignItems: 'center',
  marginBottom: 16,
  gap: 12,
  flexWrap: 'wrap',
}

const panelLabelStyle: CSSProperties = {
  color: 'var(--muted)',
  fontSize: '10px',
  letterSpacing: '0.1em',
}

const fieldWrapperStyle: CSSProperties = {
  display: 'flex',
  flexDirection: 'column',
  gap: 4,
}

const fieldLabelStyle: CSSProperties = {
  color: 'var(--muted)',
  fontSize: '11px',
  letterSpacing: '0.05em',
}

const inputStyle: CSSProperties = {
  width: '100%',
  background: 'var(--bg)',
  border: '1px solid var(--border)',
  color: 'var(--text)',
  padding: '8px 10px',
  fontFamily: 'inherit',
  fontSize: '12px',
  outline: 'none',
}

const FILTER_CONTROL_STYLE: CSSProperties = {
  background: 'var(--bg)',
  border: '1px solid var(--border)',
  color: 'var(--text)',
  fontSize: '12px',
  padding: '8px 10px',
  fontFamily: 'inherit',
  outline: 'none',
}

const OLLAMA_MODE_ACTIVE_COLOR = '#a855f7'

const actionBtnStyle: CSSProperties = {
  background: 'none',
  border: '1px solid var(--border)',
  color: 'var(--muted)',
  fontSize: '10px',
  padding: '2px 8px',
  fontFamily: 'inherit',
  cursor: 'pointer',
  letterSpacing: '0.05em',
}
