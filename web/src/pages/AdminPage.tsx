import { useEffect, useState } from 'react'

interface InviteCode {
  code: string
  used: boolean
  created_at: string
}

function authHeaders(): Record<string, string> {
  const token = localStorage.getItem('token')
  return token ? { Authorization: `Bearer ${token}` } : {}
}

// Normalize invite payloads defensively because the server returns mixed shapes:
// CreateInvite uses a snake_case map, while ListInvites serializes the model with
// exported field names. Keep the code/Code, used/Used/used_by/UsedBy, and
// created_at/CreatedAt mappings aligned unless server responses are unified.
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
    if (typeof parsed.error === 'string' && parsed.error) {
      return parsed.error
    }
  } catch {
    /* fall back to the raw response text */
  }

  return text
}

export default function AdminPage() {
  const [invites, setInvites] = useState<InviteCode[]>([])
  const [loading, setLoading] = useState(false)
  const [creating, setCreating] = useState(false)
  const [error, setError] = useState<string | null>(null)

  async function loadInvites() {
    setLoading(true)
    setError(null)
    try {
      const res = await fetch('/api/auth/invite', { headers: authHeaders() })
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
  }

  async function createInvite() {
    setCreating(true)
    setError(null)
    try {
      const res = await fetch('/api/auth/invite', { method: 'POST', headers: authHeaders() })
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
  }

  useEffect(() => {
    void loadInvites()
  }, [])

  return (
    <div>
      <div style={{ padding: '12px 16px', borderBottom: '1px solid var(--border)', background: 'var(--surface)' }}>
        <span style={{ color: 'var(--muted)', fontSize: '11px', letterSpacing: '0.1em' }}>ADMIN / INVITE CODES</span>
      </div>
      <div style={{ padding: 16 }}>
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
              {invites.map(invite => (
                <tr key={invite.code}>
                  <td style={{ padding: '6px 8px 6px 0', color: 'var(--text)', fontFamily: 'inherit' }}>{invite.code}</td>
                  <td style={{ padding: '6px 8px', color: invite.used ? 'var(--muted)' : 'var(--accent)' }}>
                    {invite.used ? 'USED' : 'AVAILABLE'}
                  </td>
                  <td style={{ padding: '6px 0 6px 8px', color: 'var(--muted)' }}>
                    {invite.created_at ? new Date(invite.created_at).toISOString().substring(0, 16).replace('T', ' ') : '—'}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </div>
  )
}
