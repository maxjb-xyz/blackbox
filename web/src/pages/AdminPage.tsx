import { useCallback, useEffect, useState } from 'react'
import { Navigate } from 'react-router-dom'
import { useSession } from '../session'
import {
  listAdminUsers, updateAdminUser, forceLogoutUser, deleteAdminUser,
} from '../api/client'
import type { AdminUser } from '../api/client'

interface InviteCode {
  code: string
  used: boolean
  created_at: string
}

function normalizeInvite(invite: Record<string, unknown>): InviteCode {
  return {
    code: String(invite.code ?? invite.Code ?? ''),
    used: Boolean(invite.used ?? invite.Used ?? invite.used_by ?? invite.UsedBy),
    created_at: String(invite.created_at ?? invite.CreatedAt ?? ''),
  }
}

async function readErrorMessage(res: Response, fallback: string) {
  const text = await res.text().catch(() => '')
  if (!text) return fallback
  try {
    const parsed = JSON.parse(text) as { error?: string }
    if (typeof parsed.error === 'string' && parsed.error) return parsed.error
  } catch { /* fall through */ }
  return text
}

type Tab = 'invites' | 'users'

export default function AdminPage() {
  const { user } = useSession()
  const isAdmin = user?.is_admin === true
  const [tab, setTab] = useState<Tab>('invites')

  if (!isAdmin) return <Navigate to="/timeline" replace />

  return (
    <div>
      <div style={{ padding: '18px 24px', borderBottom: '1px solid var(--border)', background: 'var(--surface)', display: 'flex', gap: 24, alignItems: 'center' }}>
        <span style={{ color: 'var(--muted)', fontSize: '11px', letterSpacing: '0.1em' }}>ADMIN /</span>
        {(['invites', 'users'] as Tab[]).map(t => (
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
      </div>

      <div style={{ padding: 24, maxWidth: 960, margin: '0 auto' }}>
        {tab === 'invites' ? <InvitesTab /> : <UsersTab currentUserId={user?.user_id ?? ''} />}
      </div>
    </div>
  )
}

function InvitesTab() {
  const [invites, setInvites] = useState<InviteCode[]>([])
  const [loading, setLoading] = useState(false)
  const [creating, setCreating] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const loadInvites = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      const res = await fetch('/api/auth/invite', { credentials: 'same-origin' })
      if (!res.ok) { setError(await readErrorMessage(res, 'Failed to load invites')); return }
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
      if (!res.ok) { setError(await readErrorMessage(res, 'Failed to create invite')); return }
      await loadInvites()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create invite')
    } finally {
      setCreating(false)
    }
  }, [loadInvites])

  useEffect(() => { void loadInvites() }, [loadInvites])

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
              <th style={{ textAlign: 'left', padding: '4px 0 4px 8px', borderBottom: '1px solid var(--border)' }}>CREATED</th>
            </tr>
          </thead>
          <tbody>
            {invites.map((invite, index) => (
              <tr key={`${invite.code}-${invite.created_at || index}`}>
                <td style={{ padding: '6px 8px 6px 0', color: 'var(--text)', fontFamily: 'inherit' }}>{invite.code}</td>
                <td style={{ padding: '6px 8px', color: invite.used ? 'var(--muted)' : 'var(--accent)' }}>
                  {invite.used ? 'USED' : 'AVAILABLE'}
                </td>
                <td style={{ padding: '6px 0 6px 8px', color: 'var(--muted)' }}>
                  {invite.created_at ? new Date(invite.created_at).toISOString().substring(0, 16).replace('T', ' ') : '-'}
                </td>
              </tr>
            ))}
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

  useEffect(() => { void load() }, [load])

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
                        <button onClick={() => void handleDelete(u)} style={{ ...actionBtnStyle, color: 'var(--danger)', borderColor: 'var(--danger)' }}>
                          YES
                        </button>
                        <button onClick={() => setConfirmDeleteId(null)} style={actionBtnStyle}>NO</button>
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

const actionBtnStyle: React.CSSProperties = {
  background: 'none',
  border: '1px solid var(--border)',
  color: 'var(--muted)',
  fontSize: '10px',
  padding: '2px 8px',
  fontFamily: 'inherit',
  cursor: 'pointer',
  letterSpacing: '0.05em',
}
