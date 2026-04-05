import { useCallback, useEffect, useRef, useState } from 'react'
import type { CSSProperties } from 'react'
import { Navigate } from 'react-router-dom'
import {
  createAdminOIDCProvider,
  deleteAdminOIDCProvider,
  deleteAdminUser,
  fetchAdminConfig,
  forceLogoutUser,
  getOIDCPolicy,
  listAdminOIDCProviders,
  listAdminUsers,
  revokeInvite,
  setOIDCPolicy,
  updateFileWatcherSettings,
  updateAdminOIDCProvider,
  updateAdminUser,
} from '../api/client'
import type { AdminUser, OIDCProviderConfig } from '../api/client'
import { readErrorMessage } from '../api/errorUtils'
import { useSession } from '../session'
import PageHeader from '../components/PageHeader'

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

type Tab = 'invites' | 'users' | 'oidc' | 'settings'
type OIDCPolicy = 'open' | 'existing_only' | 'invite_required'

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
  return date.toISOString().substring(0, 16).replace('T', ' ')
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

  if (!isAdmin) return <Navigate to="/timeline" replace />

  return (
    <div>
      <PageHeader
        title="ADMIN /"
        actions={(['invites', 'users', 'oidc', 'settings'] as Tab[]).map(t => (
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
                {u.created_at ? new Date(u.created_at).toISOString().substring(0, 10) : '-'}
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

  const loadSettings = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      const config = await fetchAdminConfig()
      setRedactSecrets(config.file_watcher_redact_secrets)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load settings')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    void loadSettings()
  }, [loadSettings])

  async function handleSave() {
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

  return (
    <section style={panelStyle}>
      <div style={panelHeaderStyle}>
        <span style={panelLabelStyle}>FILE WATCHER</span>
      </div>

      {message && <div style={{ color: 'var(--success)', fontSize: '12px', marginBottom: 12 }}>{message}</div>}
      {error && <div style={{ color: 'var(--danger)', fontSize: '12px', marginBottom: 12 }}>{error}</div>}

      {loading ? (
        <div style={{ color: 'var(--muted)', fontSize: '12px' }}>loading...</div>
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
            disabled={saving}
            style={{
              background: saving ? 'var(--border)' : 'var(--accent)',
              color: '#000',
              border: 'none',
              padding: '8px 16px',
              fontFamily: 'inherit',
              fontSize: '12px',
              fontWeight: 'bold',
              letterSpacing: '0.1em',
              cursor: saving ? 'not-allowed' : 'pointer',
            }}
          >
            {saving ? 'SAVING...' : 'SAVE'}
          </button>
        </>
      )}
    </section>
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
