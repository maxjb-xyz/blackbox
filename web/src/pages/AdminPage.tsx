import { Fragment, useCallback, useEffect, useRef, useState } from 'react'
import type { CSSProperties } from 'react'
import { Bug, ChevronDown, ChevronUp, ExternalLink, Lightbulb } from 'lucide-react'
import { Navigate } from 'react-router-dom'
import {
  createAdminOIDCProvider,
  createNotificationDest,
  deleteAdminOIDCProvider,
  deleteNotificationDest,
  deleteAdminUser,
  fetchAdminConfig,
  fetchNodes,
  fetchSystemdSettings,
  forceLogoutUser,
  getOIDCPolicy,
  listAdminOIDCProviders,
  listAdminUsers,
  listNotificationDests,
  revokeInvite,
  setOIDCPolicy,
  testAISettings,
  testNotificationDest,
  updateAISettings,
  updateFileWatcherSettings,
  updateAdminOIDCProvider,
  updateAdminUser,
  updateNotificationDest,
  updateSystemdSettings,
} from '../api/client'
import type { AISettingsInput, AdminUser, Node, NotificationDest, NotificationDestInput, OIDCProviderConfig } from '../api/client'
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

interface GitHubRelease {
  id: number
  tag_name: string
  name: string
  published_at: string
  body: string
  html_url: string
}

type Tab = 'invites' | 'users' | 'oidc' | 'settings' | 'systemd' | 'notifications' | 'github'
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

const NOTIFICATION_EVENT_OPTIONS = [
  { value: 'incident_opened_confirmed', label: 'INCIDENT OPENED / CONFIRMED' },
  { value: 'incident_opened_suspected', label: 'INCIDENT OPENED / SUSPECTED' },
  { value: 'incident_confirmed', label: 'INCIDENT UPGRADED TO CONFIRMED' },
  { value: 'incident_resolved', label: 'INCIDENT RESOLVED' },
] as const
const ADMIN_TABS: Tab[] = ['invites', 'users', 'oidc', 'settings', 'systemd', 'notifications', 'github']

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

function truncateDisplayText(value: string, maxChars = 200): string {
  const chars = Array.from(value)
  if (chars.length <= maxChars) return value
  return chars.slice(0, maxChars).join('') + '...'
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

function emptyNotificationDestForm(): NotificationDestInput {
  return {
    name: '',
    type: 'discord',
    url: '',
    events: [],
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
  const [notificationDests, setNotificationDests] = useState<NotificationDest[]>([])
  const [notificationsLoading, setNotificationsLoading] = useState(false)
  const [notificationsError, setNotificationsError] = useState<string | null>(null)
  const [notificationForm, setNotificationForm] = useState<NotificationDestInput>(emptyNotificationDestForm)
  const [notificationFormOpen, setNotificationFormOpen] = useState(false)
  const [notificationEditingId, setNotificationEditingId] = useState<string | null>(null)
  const [notificationSaving, setNotificationSaving] = useState(false)
  const [notificationSaveError, setNotificationSaveError] = useState<string | null>(null)
  const [notificationTestResults, setNotificationTestResults] = useState<Record<string, { ok: boolean; error?: string } | undefined>>({})
  const notificationTestTimeoutsRef = useRef<Record<string, number>>({})
  const adminTabRefs = useRef<Array<HTMLButtonElement | null>>([])

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

  const loadNotificationDests = useCallback(async () => {
    setNotificationsLoading(true)
    setNotificationsError(null)
    try {
      setNotificationDests(await listNotificationDests())
    } catch (err) {
      setNotificationsError(err instanceof Error ? err.message : 'Failed to load notification destinations')
    } finally {
      setNotificationsLoading(false)
    }
  }, [])

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

  useEffect(() => {
    if (tab !== 'notifications') return
    void loadNotificationDests()
  }, [loadNotificationDests, tab])

  useEffect(() => {
    return () => {
      Object.values(notificationTestTimeoutsRef.current).forEach(timeoutID => {
        window.clearTimeout(timeoutID)
      })
    }
  }, [])

  if (!isAdmin) return <Navigate to="/timeline" replace />

  function selectAdminTabAt(index: number) {
    const normalizedIndex = (index + ADMIN_TABS.length) % ADMIN_TABS.length
    setTab(ADMIN_TABS[normalizedIndex])
    adminTabRefs.current[normalizedIndex]?.focus()
  }

  return (
    <div>
      <PageHeader
        title="ADMIN"
        titleActions={(
          <div className="admin-title-actions">
            <span className="admin-title-divider" aria-hidden="true" />
            <div className="admin-tab-list" role="tablist" aria-label="Admin sections">
              {ADMIN_TABS.map((t, index) => (
                <Fragment key={t}>
                  {index > 0 ? <span className="admin-tab-divider" aria-hidden="true">/</span> : null}
                  <button
                    ref={element => { adminTabRefs.current[index] = element }}
                    className="admin-tab-button"
                    id={`admin-tab-${t}`}
                    role="tab"
                    aria-selected={tab === t}
                    aria-controls={`admin-panel-${t}`}
                    tabIndex={tab === t ? 0 : -1}
                    onClick={() => setTab(t)}
                    onKeyDown={event => {
                      if (event.key === 'ArrowRight') {
                        event.preventDefault()
                        selectAdminTabAt(index + 1)
                        return
                      }
                      if (event.key === 'ArrowLeft') {
                        event.preventDefault()
                        selectAdminTabAt(index - 1)
                        return
                      }
                      if (event.key === 'Home') {
                        event.preventDefault()
                        selectAdminTabAt(0)
                        return
                      }
                      if (event.key === 'End') {
                        event.preventDefault()
                        selectAdminTabAt(ADMIN_TABS.length - 1)
                      }
                    }}
                    style={{
                      background: 'none',
                      border: 'none',
                      color: tab === t ? 'var(--accent)' : '#F0F0F0',
                      fontSize: '18px',
                      fontWeight: 700,
                      letterSpacing: '0.12em',
                      cursor: 'pointer',
                      fontFamily: 'inherit',
                      lineHeight: 1,
                      padding: 0,
                    }}
                  >
                    {t.toUpperCase()}
                  </button>
                </Fragment>
              ))}
            </div>
          </div>
        )}
      />

      <div
        className="admin-page-body"
        id={`admin-panel-${tab}`}
        role="tabpanel"
        aria-labelledby={`admin-tab-${tab}`}
        style={{ padding: 24, maxWidth: 960, margin: '0 auto' }}
      >
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
        {tab === 'notifications' && (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
            <section style={panelStyle}>
              <div style={panelHeaderStyle}>
                <span style={panelLabelStyle}>DESTINATIONS</span>
                <button
                  type="button"
                  onClick={() => {
                    setNotificationForm(emptyNotificationDestForm())
                    setNotificationFormOpen(true)
                    setNotificationEditingId(null)
                    setNotificationSaveError(null)
                  }}
                  style={actionBtnStyle}
                >
                  ADD DESTINATION
                </button>
              </div>

              {notificationsError && <div style={{ color: 'var(--danger)', fontSize: '12px', marginBottom: 12 }}>{notificationsError}</div>}
              {notificationSaveError && <div style={{ color: 'var(--danger)', fontSize: '12px', marginBottom: 12 }}>{notificationSaveError}</div>}

              {notificationFormOpen && (
                <form
                  onSubmit={async e => {
                    e.preventDefault()
                    setNotificationSaving(true)
                    setNotificationSaveError(null)
                    try {
                      if (notificationEditingId) {
                        await updateNotificationDest(notificationEditingId, notificationForm)
                      } else {
                        await createNotificationDest(notificationForm)
                      }
                      setNotificationForm(emptyNotificationDestForm())
                      setNotificationFormOpen(false)
                      setNotificationEditingId(null)
                      await loadNotificationDests()
                    } catch (err) {
                      setNotificationSaveError(err instanceof Error ? err.message : 'Failed to save notification destination')
                    } finally {
                      setNotificationSaving(false)
                    }
                  }}
                  style={{ border: '1px solid var(--border)', padding: 16, marginBottom: 16 }}
                >
                  <div style={{ display: 'grid', gap: 12, gridTemplateColumns: 'repeat(auto-fit, minmax(220px, 1fr))' }}>
                    <label style={fieldWrapperStyle}>
                      <span style={fieldLabelStyle}>NAME</span>
                      <input
                        type="text"
                        value={notificationForm.name}
                        onChange={e => setNotificationForm(prev => ({ ...prev, name: e.target.value }))}
                        required
                        style={inputStyle}
                      />
                    </label>

                    <label style={fieldWrapperStyle}>
                      <span style={fieldLabelStyle}>TYPE</span>
                      <select
                        value={notificationForm.type}
                        onChange={e =>
                          setNotificationForm(prev => ({
                            ...prev,
                            type: e.target.value as NotificationDestInput['type'],
                          }))
                        }
                        style={inputStyle}
                      >
                        <option value="discord">Discord</option>
                        <option value="slack">Slack</option>
                        <option value="ntfy">Ntfy</option>
                      </select>
                    </label>

                    <label style={{ ...fieldWrapperStyle, gridColumn: '1 / -1' }}>
                      <span style={fieldLabelStyle}>
                        {notificationForm.type === 'ntfy' ? 'TOPIC URL' : 'WEBHOOK URL'}
                      </span>
                      <input
                        type="url"
                        value={notificationForm.url}
                        onChange={e => setNotificationForm(prev => ({ ...prev, url: e.target.value }))}
                        required
                        placeholder={
                          notificationForm.type === 'ntfy'
                            ? 'https://ntfy.sh/my-topic'
                            : notificationForm.type === 'slack'
                              ? 'https://hooks.slack.com/services/...'
                              : 'https://discord.com/api/webhooks/...'
                        }
                        style={inputStyle}
                      />
                    </label>

                    <div style={{ ...fieldWrapperStyle, gridColumn: '1 / -1' }}>
                      <span style={fieldLabelStyle}>EVENTS</span>
                      <div style={{ display: 'grid', gap: 8 }}>
                        {NOTIFICATION_EVENT_OPTIONS.map(option => (
                          <label
                            key={option.value}
                            style={{
                              display: 'flex',
                              gap: 10,
                              alignItems: 'center',
                              border: '1px solid var(--border)',
                              padding: '10px 12px',
                              color: 'var(--muted)',
                              cursor: 'pointer',
                              background: notificationForm.events.includes(option.value) ? 'var(--surface)' : 'transparent',
                            }}
                          >
                            <input
                              type="checkbox"
                              checked={notificationForm.events.includes(option.value)}
                              onChange={e =>
                                setNotificationForm(prev => ({
                                  ...prev,
                                  events: e.target.checked
                                    ? [...prev.events, option.value]
                                    : prev.events.filter(event => event !== option.value),
                                }))
                              }
                            />
                            <span style={{ fontSize: '11px', letterSpacing: '0.05em' }}>{option.label}</span>
                          </label>
                        ))}
                      </div>
                    </div>

                    <label style={{ ...fieldWrapperStyle, gridColumn: '1 / -1', flexDirection: 'row', alignItems: 'center', gap: 8 }}>
                      <input
                        type="checkbox"
                        checked={notificationForm.enabled}
                        onChange={e => setNotificationForm(prev => ({ ...prev, enabled: e.target.checked }))}
                      />
                      <span style={{ color: 'var(--muted)', fontSize: '11px', letterSpacing: '0.05em' }}>ENABLED</span>
                    </label>
                  </div>

                  <div style={{ display: 'flex', gap: 8, marginTop: 16, flexWrap: 'wrap' }}>
                    <button
                      type="submit"
                      disabled={notificationSaving}
                      style={{
                        background: notificationSaving ? 'var(--border)' : 'var(--accent)',
                        color: '#000',
                        border: 'none',
                        padding: '8px 16px',
                        fontFamily: 'inherit',
                        fontSize: '12px',
                        fontWeight: 'bold',
                        letterSpacing: '0.1em',
                        cursor: notificationSaving ? 'not-allowed' : 'pointer',
                      }}
                    >
                      {notificationSaving ? 'SAVING...' : notificationEditingId ? 'SAVE CHANGES' : 'CREATE DESTINATION'}
                    </button>
                    <button
                      type="button"
                      onClick={() => {
                        setNotificationForm(emptyNotificationDestForm())
                        setNotificationFormOpen(false)
                        setNotificationEditingId(null)
                        setNotificationSaveError(null)
                      }}
                      style={actionBtnStyle}
                    >
                      CANCEL
                    </button>
                  </div>
                </form>
              )}

              {notificationsLoading ? (
                <div style={{ color: 'var(--muted)', fontSize: '12px' }}>loading...</div>
              ) : notificationDests.length === 0 ? (
                <div style={{ color: 'var(--muted)', fontSize: '12px' }}>no notification destinations configured</div>
              ) : (
                <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
                  {notificationDests.map(dest => (
                    <div key={dest.id} style={{ border: '1px solid var(--border)', padding: 12 }}>
                      <div
                        style={{
                          display: 'flex',
                          justifyContent: 'space-between',
                          alignItems: 'flex-start',
                          gap: 12,
                          flexWrap: 'wrap',
                        }}
                      >
                        <div style={{ minWidth: 0, flex: 1 }}>
                          <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap', marginBottom: 6 }}>
                            <span style={{ color: 'var(--text)', fontSize: '12px', fontWeight: 'bold' }}>{dest.name}</span>
                            <span style={{ color: 'var(--muted)', fontSize: '10px', letterSpacing: '0.1em' }}>
                              {dest.type.toUpperCase()}
                            </span>
                            <span style={{ color: dest.enabled ? 'var(--success)' : 'var(--muted)', fontSize: '10px', letterSpacing: '0.1em' }}>
                              {dest.enabled ? 'ENABLED' : 'DISABLED'}
                            </span>
                          </div>
                          <div style={{ color: 'var(--muted)', fontSize: '11px', wordBreak: 'break-all', marginBottom: 6 }}>
                            {dest.url}
                          </div>
                          <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap' }}>
                            {dest.events.map(event => (
                              <span
                                key={event}
                                style={{
                                  border: '1px solid var(--border)',
                                  padding: '2px 6px',
                                  color: 'var(--muted)',
                                  fontSize: '10px',
                                  letterSpacing: '0.05em',
                                }}
                              >
                                {event.toUpperCase()}
                              </span>
                            ))}
                          </div>
                        </div>

                        <div style={{ display: 'flex', gap: 8, alignItems: 'center', flexWrap: 'wrap', justifyContent: 'flex-end' }}>
                          {notificationTestResults[dest.id] && (
                            <span
                              style={{
                                color: notificationTestResults[dest.id]?.ok ? 'var(--success)' : 'var(--danger)',
                                fontSize: '11px',
                              }}
                            >
                              {notificationTestResults[dest.id]?.ok
                                ? 'TEST SENT'
                                : notificationTestResults[dest.id]?.error ?? 'TEST FAILED'}
                            </span>
                          )}
                          <button
                            type="button"
                            onClick={async () => {
                              const existingTimeout = notificationTestTimeoutsRef.current[dest.id]
                              if (existingTimeout) {
                                window.clearTimeout(existingTimeout)
                              }
                              setNotificationTestResults(prev => ({ ...prev, [dest.id]: undefined }))
                              try {
                                const result = await testNotificationDest(dest.id)
                                setNotificationTestResults(prev => ({ ...prev, [dest.id]: result }))
                              } catch (err) {
                                setNotificationTestResults(prev => ({
                                  ...prev,
                                  [dest.id]: {
                                    ok: false,
                                    error: err instanceof Error ? err.message : 'Failed to test notification destination',
                                  },
                                }))
                              }
                              notificationTestTimeoutsRef.current[dest.id] = window.setTimeout(() => {
                                setNotificationTestResults(prev => {
                                  const next = { ...prev }
                                  delete next[dest.id]
                                  return next
                                })
                                delete notificationTestTimeoutsRef.current[dest.id]
                              }, 5000)
                            }}
                            style={actionBtnStyle}
                          >
                            TEST
                          </button>
                          <button
                            type="button"
                            onClick={() => {
                              setNotificationForm({
                                name: dest.name,
                                type: dest.type,
                                url: dest.url,
                                events: [...dest.events],
                                enabled: dest.enabled,
                              })
                              setNotificationFormOpen(true)
                              setNotificationEditingId(dest.id)
                              setNotificationSaveError(null)
                            }}
                            style={actionBtnStyle}
                          >
                            EDIT
                          </button>
                          <button
                            type="button"
                            onClick={async () => {
                              if (!window.confirm(`Delete notification destination "${dest.name}"?`)) return
                              setNotificationsError(null)
                              try {
                                await deleteNotificationDest(dest.id)
                                if (notificationEditingId === dest.id) {
                                  setNotificationForm(emptyNotificationDestForm())
                                  setNotificationFormOpen(false)
                                  setNotificationEditingId(null)
                                  setNotificationSaveError(null)
                                }
                                await loadNotificationDests()
                              } catch (err) {
                                setNotificationsError(err instanceof Error ? err.message : 'Failed to delete notification destination')
                              }
                            }}
                            style={{ ...actionBtnStyle, color: 'var(--danger)', borderColor: 'var(--danger)' }}
                          >
                            DELETE
                          </button>
                        </div>
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </section>
          </div>
        )}
        {tab === 'github' && <GitHubTab />}
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
          <>
            <div className="oidc-provider-table-wrap">
              <table className="oidc-provider-table" style={{ width: '100%', borderCollapse: 'collapse', fontSize: '12px' }}>
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
            </div>

            <div className="oidc-provider-mobile-list">
              {providers.map(provider => (
                <div key={`${provider.id}-mobile`} className="oidc-provider-card">
                  <div className="oidc-provider-card-head">
                    <div>
                      <div className="oidc-provider-card-name">{provider.name}</div>
                      <div className="oidc-provider-card-id">{provider.id}</div>
                    </div>
                    <div style={{ color: provider.enabled ? 'var(--success)' : 'var(--muted)', fontSize: '11px', letterSpacing: '0.08em' }}>
                      {provider.enabled ? 'ENABLED' : 'DISABLED'}
                    </div>
                  </div>

                  <div className="oidc-provider-card-field">
                    <span className="oidc-provider-card-label">ISSUER</span>
                    <span className="oidc-provider-card-value">{provider.issuer}</span>
                  </div>
                  <div className="oidc-provider-card-field">
                    <span className="oidc-provider-card-label">CREATED</span>
                    <span className="oidc-provider-card-value">{formatInviteTimestamp(provider.created_at)}</span>
                  </div>

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
                </div>
              ))}
            </div>
          </>
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
  const [aiClearAPIKey, setAIClearAPIKey] = useState(false)
  const [aiMode, setAIMode] = useState<'analysis' | 'enhanced'>('analysis')
  const [aiSaving, setAISaving] = useState(false)
  const [aiTesting, setAITesting] = useState(false)
  const [aiError, setAIError] = useState<string | null>(null)
  const [aiSuccess, setAISuccess] = useState(false)
  const [aiTestResult, setAITestResult] = useState<{ ok: boolean; response?: string; error?: string } | null>(null)
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
      await updateAISettings(getAISettingsInput())
      setAIAPIKey('')
      setAIClearAPIKey(false)
      const saved = await fetchAdminConfig()
      setAIAPIKeySet(saved.ai_api_key_set ?? false)
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

  function getAISettingsInput(): AISettingsInput {
    return {
      provider: aiProvider,
      url: aiURL,
      model: aiModel,
      apiKey: aiAPIKey,
      clearAPIKey: aiClearAPIKey,
      mode: aiMode,
    }
  }

  async function runAITest() {
    if (!initialLoaded) return
    setAITesting(true)
    setAITestResult(null)
    try {
      const result = await testAISettings(getAISettingsInput())
      setAITestResult(result)
    } catch (err) {
      setAITestResult({
        ok: false,
        error: err instanceof Error ? err.message : 'Test failed',
      })
    } finally {
      setAITesting(false)
    }
  }

  const aiTestMessage = aiTestResult
    ? aiTestResult.ok
      ? `test ok${aiTestResult.response ? `: ${truncateDisplayText(aiTestResult.response)}` : ''}`
      : truncateDisplayText(aiTestResult.error ?? 'Test failed')
    : null

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
                    onClick={() => {
                      if (p !== 'openai_compat') { setAIAPIKey(''); setAIClearAPIKey(false) }
                      setAIProvider(p)
                      setAISuccess(false)
                      setAITestResult(null)
                    }}
                    disabled={aiSaving || aiTesting}
                    style={{
                      padding: '4px 12px',
                      fontSize: 12,
                      border: '1px solid',
                      borderColor: selected ? AI_SETTINGS_ACCENT_COLOR : 'var(--border)',
                      background: selected ? 'rgba(168, 85, 247, 0.1)' : 'transparent',
                      color: selected ? AI_SETTINGS_ACCENT_COLOR : 'var(--muted)',
                      cursor: aiSaving || aiTesting ? 'not-allowed' : 'pointer',
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
                onChange={e => { setAIURL(e.target.value); setAISuccess(false); setAITestResult(null) }}
                placeholder={aiProvider === 'openai_compat' ? 'https://api.openai.com' : 'http://192.168.1.10:11434'}
                disabled={!initialLoaded || aiSaving || aiTesting}
                style={{ display: 'block', width: '100%', marginTop: 4, ...FILTER_CONTROL_STYLE }}
              />
            </label>
            {aiProvider === 'openai_compat' && (
              <label style={{ fontSize: 11, color: 'var(--muted)', display: 'block', marginBottom: 8 }}>
                <span style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                  API KEY
                  {aiAPIKeySet && !aiClearAPIKey && (
                    <button
                      type="button"
                      onClick={() => { setAIClearAPIKey(true); setAIAPIKey(''); setAISuccess(false); setAITestResult(null) }}
                      disabled={aiSaving || aiTesting}
                      style={{ fontSize: 10, padding: '1px 6px', border: '1px solid var(--danger)', color: 'var(--danger)', background: 'transparent', cursor: 'pointer', fontFamily: 'inherit' }}
                    >
                      CLEAR KEY
                    </button>
                  )}
                  {aiClearAPIKey && (
                    <button
                      type="button"
                      onClick={() => { setAIClearAPIKey(false); setAISuccess(false); setAITestResult(null) }}
                      disabled={aiSaving || aiTesting}
                      style={{ fontSize: 10, padding: '1px 6px', border: '1px solid var(--muted)', color: 'var(--muted)', background: 'transparent', cursor: 'pointer', fontFamily: 'inherit' }}
                    >
                      CANCEL
                    </button>
                  )}
                </span>
                <input
                  type="password"
                  value={aiAPIKey}
                  onChange={e => { setAIAPIKey(e.target.value); if (aiClearAPIKey) setAIClearAPIKey(false); setAISuccess(false); setAITestResult(null) }}
                  placeholder={aiClearAPIKey ? '[key will be removed on save]' : aiAPIKeySet ? '[key set — leave blank to keep]' : 'sk-...'}
                  disabled={!initialLoaded || aiSaving || aiTesting}
                  style={{ display: 'block', width: '100%', marginTop: 4, ...FILTER_CONTROL_STYLE }}
                />
              </label>
            )}
            <label style={{ fontSize: 11, color: 'var(--muted)', display: 'block', marginBottom: 8 }}>
              MODEL
              <input
                type="text"
                value={aiModel}
                onChange={e => { setAIModel(e.target.value); setAISuccess(false); setAITestResult(null) }}
                placeholder={aiProvider === 'openai_compat' ? 'gpt-4o-mini' : 'llama3.2'}
                disabled={!initialLoaded || aiSaving || aiTesting}
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
                        onClick={() => { setAIMode(mode); setAISuccess(false); setAITestResult(null) }}
                        disabled={aiSaving || aiTesting}
                        style={{
                          padding: '4px 12px',
                          fontSize: 12,
                          border: '1px solid',
                          borderColor: selected ? AI_SETTINGS_ACCENT_COLOR : 'var(--border)',
                          background: selected ? 'rgba(168, 85, 247, 0.1)' : 'transparent',
                          color: selected ? AI_SETTINGS_ACCENT_COLOR : 'var(--muted)',
                          cursor: aiSaving || aiTesting ? 'not-allowed' : 'pointer',
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
            {aiTestMessage && (
              <div style={{ color: aiTestResult?.ok ? 'var(--success)' : 'var(--danger)', fontSize: 11, overflowWrap: 'anywhere' }}>
                {aiTestMessage}
              </div>
            )}
            {aiSuccess && <div style={{ color: 'var(--success)', fontSize: 11 }}>saved</div>}
            <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
              <button
                type="button"
                onClick={() => void runAITest()}
                disabled={!initialLoaded || aiSaving || aiTesting || aiURL.trim() === '' || aiModel.trim() === ''}
                style={{
                  alignSelf: 'flex-start',
                  background: 'transparent',
                  border: '1px solid var(--border)',
                  color: 'var(--muted)',
                  fontSize: 11,
                  padding: '4px 12px',
                  cursor: !initialLoaded || aiSaving || aiTesting || aiURL.trim() === '' || aiModel.trim() === '' ? 'not-allowed' : 'pointer',
                  fontFamily: 'inherit',
                }}
              >
                {aiTesting ? 'TESTING...' : 'TEST'}
              </button>
              <button
                type="submit"
                disabled={!initialLoaded || aiSaving || aiTesting}
                style={{
                  alignSelf: 'flex-start',
                  background: 'transparent',
                  border: '1px solid var(--border)',
                  color: 'var(--muted)',
                  fontSize: 11,
                  padding: '4px 12px',
                  cursor: !initialLoaded || aiSaving || aiTesting ? 'not-allowed' : 'pointer',
                  fontFamily: 'inherit',
                }}
              >
                {aiSaving ? 'SAVING...' : 'SAVE'}
              </button>
            </div>
          </form>
        )}
      </section>
    </div>
  )
}

function GitHubTab() {
  const [releases, setReleases] = useState<GitHubRelease[]>([])
  const [releasesLoading, setReleasesLoading] = useState(false)
  const [releasesError, setReleasesError] = useState<string | null>(null)
  const [expandedIds, setExpandedIds] = useState<Set<number>>(new Set())

  useEffect(() => {
    const controller = new AbortController()
    void (async () => {
      setReleasesLoading(true)
      setReleasesError(null)
      try {
        const res = await fetch('/api/admin/github/releases', { signal: controller.signal })
        if (!res.ok) throw new Error(`Failed to fetch releases (${res.status})`)
        const raw: unknown = await res.json()
        if (!Array.isArray(raw)) throw new Error('Unexpected response format from releases API')
        setReleases(raw as GitHubRelease[])
      } catch (err) {
        if (err instanceof Error && err.name === 'AbortError') return
        setReleasesError(err instanceof Error ? err.message : 'Failed to fetch releases')
      } finally {
        setReleasesLoading(false)
      }
    })()
    return () => controller.abort()
  }, [])

  function toggleExpanded(id: number) {
    setExpandedIds(prev => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
      <section style={panelStyle}>
        <div style={panelHeaderStyle}>
          <span style={panelLabelStyle}>COMMUNITY</span>
        </div>
        <div style={{ display: 'flex', gap: 12, flexWrap: 'wrap' }}>
          <a
            href="https://github.com/maxjb-xyz/blackbox/issues/new?template=feature_request.md&labels=enhancement"
            target="_blank"
            rel="noopener noreferrer"
            style={githubActionBtnStyle}
          >
            <Lightbulb size={14} />
            SUGGEST A FEATURE
          </a>
          <a
            href="https://github.com/maxjb-xyz/blackbox/issues/new?template=bug_report.md&labels=bug"
            target="_blank"
            rel="noopener noreferrer"
            style={githubActionBtnStyle}
          >
            <Bug size={14} />
            REPORT A BUG
          </a>
        </div>
      </section>

      <section style={panelStyle}>
        <div style={panelHeaderStyle}>
          <span style={panelLabelStyle}>RELEASES</span>
        </div>
        {releasesLoading && (
          <div style={{ color: 'var(--muted)', fontSize: 12 }}>Loading releases...</div>
        )}
        {releasesError && (
          <div style={{ color: 'var(--danger)', fontSize: 12 }}>
            {releasesError || 'Failed to load releases from GitHub. Check your network connection.'}
          </div>
        )}
        {!releasesLoading && !releasesError && releases.length === 0 && (
          <div style={{ color: 'var(--muted)', fontSize: 12 }}>No releases found.</div>
        )}
        {!releasesLoading && !releasesError && releases.map((release, index) => {
          const isExpanded = expandedIds.has(release.id)
          const hasBody = Boolean(release.body && release.body.trim().length > 0)
          const isLast = index === releases.length - 1

          return (
            <div
              key={release.id}
              style={{
                padding: '12px 0',
                borderBottom: isLast ? 'none' : '1px solid var(--border)',
              }}
            >
              <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}>
                <span style={{ color: 'var(--accent)', fontSize: 12, fontFamily: 'inherit' }}>
                  {release.tag_name}
                </span>
                {release.name && (
                  <span style={{ color: 'var(--text)', fontSize: 12 }}>
                    - {release.name}
                  </span>
                )}
                <span style={{ color: 'var(--muted)', fontSize: 11, marginLeft: 'auto' }}>
                  {formatLocalDate(release.published_at)}
                </span>
                <a
                  href={release.html_url}
                  target="_blank"
                  rel="noopener noreferrer"
                  style={{
                    color: 'var(--muted)',
                    fontSize: 11,
                    textDecoration: 'none',
                    display: 'inline-flex',
                    alignItems: 'center',
                    gap: 4,
                  }}
                >
                  <ExternalLink size={12} />
                  View on GitHub
                </a>
                {hasBody && (
                  <button
                    type="button"
                    onClick={() => toggleExpanded(release.id)}
                    style={{
                      background: 'none',
                      border: 'none',
                      cursor: 'pointer',
                      color: 'var(--muted)',
                      padding: 0,
                      display: 'inline-flex',
                      alignItems: 'center',
                    }}
                    aria-label={isExpanded ? 'Collapse release notes' : 'Expand release notes'}
                  >
                    {isExpanded ? <ChevronUp size={12} /> : <ChevronDown size={12} />}
                  </button>
                )}
              </div>

              {isExpanded && hasBody && (
                <pre
                  style={{
                    whiteSpace: 'pre-wrap',
                    fontSize: 11,
                    color: 'var(--muted)',
                    borderTop: '1px solid var(--border)',
                    paddingTop: 8,
                    fontFamily: 'inherit',
                    margin: '8px 0 0 0',
                  }}
                >
                  {release.body}
                </pre>
              )}
            </div>
          )
        })}
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

const AI_SETTINGS_ACCENT_COLOR = '#a855f7'

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

const githubActionBtnStyle: CSSProperties = {
  display: 'flex',
  alignItems: 'center',
  gap: 8,
  flex: 1,
  justifyContent: 'center',
  background: 'none',
  border: '1px solid var(--border)',
  color: 'var(--text)',
  fontSize: '11px',
  letterSpacing: '0.08em',
  padding: '10px 16px',
  fontFamily: 'inherit',
  cursor: 'pointer',
  textDecoration: 'none',
}
