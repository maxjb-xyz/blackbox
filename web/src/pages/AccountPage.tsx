import { useEffect, useState, type CSSProperties, type FormEvent } from 'react'
import { updateAccountEmail } from '../api/client'
import { useSession } from '../session'
import PageHeader from '../components/PageHeader'

const fontFamily = 'JetBrains Mono, Fira Code, Cascadia Code, ui-monospace, monospace'

const pageStyle: CSSProperties = {
  minHeight: '100%',
  background: '#0B0B0B',
  fontFamily,
}

const contentStyle: CSSProperties = {
  padding: '14px 16px 24px',
  maxWidth: 960,
  margin: '0 auto',
}

const panelStyle: CSSProperties = {
  border: '1px solid var(--border)',
  background: '#0B0B0B',
}

const rowStyle: CSSProperties = {
  display: 'flex',
  alignItems: 'flex-start',
  gap: 24,
  flexWrap: 'wrap',
  padding: '14px 16px',
  borderBottom: '1px solid var(--border)',
}

const lastRowStyle: CSSProperties = {
  ...rowStyle,
  borderBottom: 'none',
}

const labelStyle: CSSProperties = {
  width: 120,
  color: 'var(--muted)',
  fontSize: '11px',
  letterSpacing: '0.08em',
  flexShrink: 0,
}

const valueStyle: CSSProperties = {
  color: 'var(--text)',
  fontSize: '13px',
  lineHeight: 1.5,
}

const fieldStyle: CSSProperties = {
  flex: 1,
  minWidth: 0,
  display: 'flex',
  flexDirection: 'column',
  gap: 6,
}

const buttonStyle: CSSProperties = {
  background: 'transparent',
  border: '1px solid var(--border)',
  color: 'var(--text)',
  padding: '5px 10px',
  fontFamily,
  fontSize: '11px',
  letterSpacing: '0.08em',
  cursor: 'pointer',
}

const mutedNoteStyle: CSSProperties = {
  color: 'var(--muted)',
  fontSize: '11px',
}

const errorStyle: CSSProperties = {
  color: 'var(--danger)',
  fontSize: '12px',
}

const inputStyle: CSSProperties = {
  width: '100%',
  maxWidth: 360,
  background: 'var(--surface)',
  border: '1px solid var(--border)',
  color: 'var(--text)',
  padding: '8px 10px',
  fontFamily,
  fontSize: '13px',
  boxSizing: 'border-box',
}

export default function AccountPage() {
  const { user, updateSession } = useSession()
  const [editingEmail, setEditingEmail] = useState(false)
  const [emailInput, setEmailInput] = useState('')
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const username = user?.username ?? '—'
  const currentEmail = user?.email ?? ''
  const displayEmail = currentEmail.trim() || '—'
  const role = user ? (user.is_admin ? 'ADMIN' : 'USER') : '—'
  const oidcLinked = user?.oidc_linked === true
  const canEditEmail = user !== null && !oidcLinked

  useEffect(() => {
    if (!editingEmail) {
      setEmailInput(currentEmail)
    }
  }, [currentEmail, editingEmail])

  useEffect(() => {
    if (oidcLinked) {
      setEditingEmail(false)
      setSaving(false)
      setError(null)
    }
  }, [oidcLinked])

  function handleStartEdit() {
    setEmailInput(currentEmail)
    setError(null)
    setEditingEmail(true)
  }

  function handleCancelEdit() {
    setEmailInput(currentEmail)
    setError(null)
    setEditingEmail(false)
  }

  async function handleSave(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!user || oidcLinked) {
      return
    }

    const nextEmail = emailInput.trim()
    if (nextEmail === currentEmail) {
      setError(null)
      setEditingEmail(false)
      return
    }

    setSaving(true)
    setError(null)
    try {
      const updatedUser = await updateAccountEmail(nextEmail)
      updateSession(updatedUser)
      setEditingEmail(false)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to update email')
    } finally {
      setSaving(false)
    }
  }

  return (
    <div style={pageStyle}>
      <PageHeader title="ACCOUNT" />

      <div style={contentStyle}>
        <div style={panelStyle}>
          <div style={rowStyle}>
            <div style={labelStyle}>USERNAME</div>
            <div style={fieldStyle}>
              <span style={valueStyle}>{username}</span>
            </div>
          </div>

          <div style={rowStyle}>
            <div style={labelStyle}>EMAIL</div>
            <div style={fieldStyle}>
              {oidcLinked ? (
                <>
                  <span style={valueStyle}>{displayEmail}</span>
                  <span style={mutedNoteStyle}>managed by SSO provider</span>
                </>
              ) : canEditEmail && editingEmail ? (
                <form onSubmit={handleSave} style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
                  <input
                    aria-label="Email address"
                    type="email"
                    value={emailInput}
                    onChange={event => setEmailInput(event.target.value)}
                    disabled={saving}
                    autoFocus
                    style={inputStyle}
                  />
                  <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
                    <button
                      type="submit"
                      disabled={saving}
                      style={{ ...buttonStyle, color: saving ? 'var(--muted)' : 'var(--accent)' }}
                    >
                      {saving ? '[SAVING]' : '[SAVE]'}
                    </button>
                    <button type="button" onClick={handleCancelEdit} disabled={saving} style={buttonStyle}>
                      [CANCEL]
                    </button>
                  </div>
                  {error && <div style={errorStyle}>{error}</div>}
                </form>
              ) : (
                <>
                  <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}>
                    <span style={valueStyle}>{displayEmail}</span>
                    {canEditEmail && (
                      <button type="button" onClick={handleStartEdit} style={buttonStyle}>
                        [EDIT]
                      </button>
                    )}
                  </div>
                  {error && <div style={errorStyle}>{error}</div>}
                </>
              )}
            </div>
          </div>

          <div style={oidcLinked ? rowStyle : lastRowStyle}>
            <div style={labelStyle}>ROLE</div>
            <div style={fieldStyle}>
              <span style={valueStyle}>{role}</span>
            </div>
          </div>

          {oidcLinked && (
            <div style={lastRowStyle}>
              <div style={labelStyle}>SSO</div>
              <div style={fieldStyle}>
                <span style={valueStyle}>account linked via OIDC</span>
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
