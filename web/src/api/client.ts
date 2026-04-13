import { readErrorMessage } from './errorUtils'

export interface SetupStatus {
  bootstrapped: boolean
}

export interface SessionUser {
  user_id: string
  username: string
  is_admin: boolean
  email: string
  oidc_linked: boolean
}

export interface HealthStatus {
  database: 'ok' | 'error'
  oidc: 'ok' | 'unavailable' | 'disabled'
  oidc_enabled: boolean
}

export interface Entry {
  id: string
  timestamp: string
  node_name: string
  source: string
  service: string
  event: string
  content: string
  metadata: string
  correlated_id?: string
}

export interface EntriesPage {
  entries: Entry[]
  next_cursor?: string
}

export interface Node {
  id: string
  name: string
  last_seen: string
  agent_version: string
  ip_address: string
  os_info: string
  status: 'online' | 'offline'
}

export interface EntryNote {
  id: string
  entry_id: string
  user_id: string
  username: string
  content: string
  created_at: string
}

export interface EntryNotesPage {
  notes: EntryNote[]
  has_more: boolean
  next_offset?: number
}

export interface AdminUser {
  id: string
  username: string
  is_admin: boolean
  token_version: number
  created_at: string
}

function apiFetch(input: RequestInfo | URL, init?: RequestInit) {
  return fetch(input, { credentials: 'same-origin', ...init })
}

export async function checkSetupStatus(): Promise<SetupStatus> {
  const res = await apiFetch('/api/setup/status')
  if (!res.ok) throw new Error('Failed to check setup status')
  return res.json()
}

export async function checkHealth(): Promise<HealthStatus> {
  const res = await apiFetch('/api/setup/health')
  if (!res.ok) throw new Error('Failed to check health')
  return res.json()
}

export async function bootstrap(username: string, email: string, password: string): Promise<SessionUser> {
  const res = await apiFetch('/api/auth/bootstrap', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username, email, password }),
  })
  if (!res.ok) throw new Error(await readErrorMessage(res, 'Bootstrap failed'))
  const data = (await res.json()) as { user: SessionUser }
  return data.user
}

export async function login(username: string, password: string): Promise<SessionUser> {
  const res = await apiFetch('/api/auth/login', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username, password }),
  })
  if (!res.ok) throw new Error(await readErrorMessage(res, 'Login failed'))
  const data = (await res.json()) as { user: SessionUser }
  return data.user
}

export async function register(
  username: string,
  email: string,
  password: string,
  inviteCode: string,
): Promise<SessionUser> {
  const res = await apiFetch('/api/auth/register', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username, email, password, invite_code: inviteCode }),
  })
  if (!res.ok) throw new Error(await readErrorMessage(res, 'Registration failed'))
  const data = (await res.json()) as { user: SessionUser }
  return data.user
}

export interface PublicOIDCProvider {
  id: string
  name: string
}

export async function fetchOIDCProviders(): Promise<{ providers: PublicOIDCProvider[] }> {
  const res = await apiFetch('/api/auth/oidc/providers')
  if (!res.ok) throw new Error(await readErrorMessage(res, 'Failed to fetch OIDC providers'))
  return res.json()
}

export async function fetchCurrentUser(): Promise<SessionUser> {
  const res = await apiFetch('/api/auth/me')
  if (!res.ok) throw new Error('Failed to fetch current user')
  const data = (await res.json()) as { user: SessionUser }
  return data.user
}

export async function updateAccountEmail(email: string): Promise<SessionUser> {
  const res = await apiFetch('/api/auth/me', {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ email }),
  })
  if (!res.ok) {
    const data = await res.json().catch(() => ({}))
    throw new Error((data as { error?: string }).error ?? 'Failed to update email')
  }
  const data = (await res.json()) as { user: SessionUser }
  return data.user
}

export async function logout(): Promise<void> {
  const res = await apiFetch('/api/auth/logout', { method: 'POST' })
  if (!res.ok) throw new Error('Logout failed')
}

export async function fetchNodes(): Promise<Node[]> {
  const res = await apiFetch('/api/nodes')
  if (!res.ok) throw new Error('Failed to fetch nodes')
  return res.json()
}

export async function fetchEntries(params: {
  cursor?: string
  limit?: number
  node?: string
  source?: string
  service?: string
  q?: string
  hideHeartbeat?: boolean
  timeStart?: Date | null
  timeEnd?: Date | null
}): Promise<EntriesPage> {
  const url = new URL('/api/entries', window.location.origin)
  if (params.cursor) url.searchParams.set('cursor', params.cursor)
  if (params.limit) url.searchParams.set('limit', String(params.limit))
  if (params.node) url.searchParams.set('node', params.node)
  if (params.source) url.searchParams.set('source', params.source)
  if (params.service) url.searchParams.set('service', params.service)
  if (params.q) url.searchParams.set('q', params.q)
  if (params.hideHeartbeat) url.searchParams.set('hide_heartbeat', 'true')
  if (params.timeStart) url.searchParams.set('time_start', params.timeStart.toISOString())
  if (params.timeEnd) url.searchParams.set('time_end', params.timeEnd.toISOString())

  const res = await apiFetch(url.toString())
  if (!res.ok) throw new Error('Failed to fetch entries')
  return res.json()
}

export async function fetchEntryServices(): Promise<{ services: string[] }> {
  const res = await apiFetch('/api/entries/services')
  if (!res.ok) throw new Error(await readErrorMessage(res, 'Failed to fetch entry services'))
  return res.json()
}

export async function fetchEntry(id: string): Promise<Entry> {
  const res = await apiFetch(`/api/entries/${id}`)
  if (!res.ok) throw new Error('Entry not found')
  return res.json()
}

export async function fetchNotes(entryId: string): Promise<EntryNote[]> {
  const res = await apiFetch(`/api/entries/${entryId}/notes`)
  if (!res.ok) throw new Error('Failed to fetch notes')
  const data = (await res.json()) as EntryNotesPage
  return data.notes
}

export async function createNote(entryId: string, content: string): Promise<EntryNote> {
  const res = await apiFetch(`/api/entries/${entryId}/notes`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ content }),
  })
  if (!res.ok) throw new Error('Failed to create note')
  return res.json()
}

export async function deleteNote(noteId: string): Promise<void> {
  const res = await apiFetch(`/api/notes/${noteId}`, {
    method: 'DELETE',
  })
  if (!res.ok) throw new Error('Failed to delete note')
}

export async function listAdminUsers(): Promise<AdminUser[]> {
  const res = await apiFetch('/api/admin/users')
  if (!res.ok) throw new Error('Failed to list users')
  return res.json()
}

export async function updateAdminUser(id: string, isAdmin: boolean): Promise<AdminUser> {
  const res = await apiFetch(`/api/admin/users/${id}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ is_admin: isAdmin }),
  })
  if (!res.ok) {
    const data = await res.json().catch(() => ({}))
    throw new Error((data as { error?: string }).error ?? 'Failed to update user')
  }
  return res.json()
}

export async function forceLogoutUser(id: string): Promise<void> {
  const res = await apiFetch(`/api/admin/users/${id}/force-logout`, { method: 'POST' })
  if (!res.ok) throw new Error('Failed to force logout user')
}

export async function deleteAdminUser(id: string): Promise<void> {
  const res = await apiFetch(`/api/admin/users/${id}`, { method: 'DELETE' })
  if (!res.ok) throw new Error(await readErrorMessage(res, 'Failed to delete user'))
}

export interface AdminConfig {
  webhook_secret: string
  file_watcher_redact_secrets: boolean
  ai_provider: 'ollama' | 'openai_compat'
  ai_url: string
  ai_model: string
  ai_api_key_set: boolean
  ai_mode: 'analysis' | 'enhanced'
}

export async function fetchAdminConfig(): Promise<AdminConfig> {
  const res = await apiFetch('/api/admin/config')
  if (!res.ok) throw new Error('Failed to fetch admin config')
  return res.json()
}

export async function updateFileWatcherSettings(redactSecrets: boolean): Promise<{ redact_secrets: boolean }> {
  const res = await apiFetch('/api/admin/settings/file-watcher', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ redact_secrets: redactSecrets }),
  })
  if (!res.ok) throw new Error(await readErrorMessage(res, 'Failed to update file watcher settings'))
  return res.json()
}

export async function updateAISettings(
  provider: 'ollama' | 'openai_compat',
  url: string,
  model: string,
  apiKey: string,
  clearAPIKey: boolean,
  mode: 'analysis' | 'enhanced',
): Promise<void> {
  const res = await apiFetch('/api/admin/settings/ai', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ ai_provider: provider, ai_url: url, ai_model: model, ai_api_key: apiKey, ai_clear_api_key: clearAPIKey, ai_mode: mode }),
  })
  if (!res.ok) throw new Error(await readErrorMessage(res, 'Failed to update AI settings'))
}

export async function fetchSystemdSettings(): Promise<Record<string, string[]>> {
  const res = await apiFetch('/api/admin/settings/systemd')
  if (!res.ok) throw new Error(await readErrorMessage(res, 'Failed to fetch systemd settings'))
  return res.json() as Promise<Record<string, string[]>>
}

export async function updateSystemdSettings(nodeName: string, units: string[]): Promise<void> {
  const res = await apiFetch(`/api/admin/settings/systemd/${encodeURIComponent(nodeName)}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ units }),
  })
  if (!res.ok) throw new Error(await readErrorMessage(res, 'Failed to update systemd settings'))
}

export async function revokeInvite(id: string): Promise<void> {
  const res = await apiFetch(`/api/auth/invite/${encodeURIComponent(id)}`, { method: 'DELETE' })
  if (!res.ok) throw new Error(await readErrorMessage(res, 'Failed to revoke invite'))
}

export interface OIDCProviderConfig {
  id: string
  name: string
  issuer: string
  client_id: string
  client_secret: string
  redirect_url: string
  require_verified_email: boolean
  enabled: boolean
  created_at: string
  updated_at: string
}

export interface AdminOIDCProviderInput {
  id: string
  name: string
  issuer: string
  client_id: string
  client_secret?: string
  redirect_url: string
  require_verified_email: boolean
  enabled: boolean
}

export async function listAdminOIDCProviders(): Promise<OIDCProviderConfig[]> {
  const res = await apiFetch('/api/admin/oidc/providers')
  if (!res.ok) throw new Error(await readErrorMessage(res, 'Failed to list OIDC providers'))
  return res.json()
}

export async function createAdminOIDCProvider(
  data: AdminOIDCProviderInput,
): Promise<OIDCProviderConfig> {
  const res = await apiFetch('/api/admin/oidc/providers', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  })
  if (!res.ok) throw new Error(await readErrorMessage(res, 'Failed to create OIDC provider'))
  return res.json()
}

export async function updateAdminOIDCProvider(
  id: string,
  data: Partial<AdminOIDCProviderInput>,
): Promise<OIDCProviderConfig> {
  const res = await apiFetch(`/api/admin/oidc/providers/${encodeURIComponent(id)}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  })
  if (!res.ok) throw new Error(await readErrorMessage(res, 'Failed to update OIDC provider'))
  return res.json()
}

export async function deleteAdminOIDCProvider(id: string): Promise<void> {
  const res = await apiFetch(`/api/admin/oidc/providers/${encodeURIComponent(id)}`, {
    method: 'DELETE',
  })
  if (!res.ok) throw new Error(await readErrorMessage(res, 'Failed to delete OIDC provider'))
}

export async function getOIDCPolicy(): Promise<{ policy: string }> {
  const res = await apiFetch('/api/admin/oidc/policy')
  if (!res.ok) throw new Error(await readErrorMessage(res, 'Failed to fetch OIDC policy'))
  return res.json()
}

export async function setOIDCPolicy(policy: string): Promise<void> {
  const res = await apiFetch('/api/admin/oidc/policy', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ policy }),
  })
  if (!res.ok) throw new Error(await readErrorMessage(res, 'Failed to update OIDC policy'))
}

export interface Incident {
  id: string
  opened_at: string
  resolved_at?: string | null
  status: 'open' | 'resolved'
  confidence: 'confirmed' | 'suspected'
  title: string
  services: string
  root_cause_id?: string
  trigger_id?: string
  node_names: string
  metadata: string
}

export interface IncidentEntryLink {
  link: {
    incident_id: string
    entry_id: string
    role: 'trigger' | 'cause' | 'evidence' | 'recovery' | 'ai_cause'
    score: number
    reason?: string
  }
  entry: Entry
}

export interface IncidentDetail {
  incident: Incident
  entries: IncidentEntryLink[]
}

export interface IncidentsPage {
  incidents: Incident[]
  has_more: boolean
}

export interface IncidentMembership {
  id: string
  confidence: Incident['confidence']
}

export interface IncidentSummary {
  openCount: number
  hasConfirmedOpen: boolean
}

export interface NotificationDest {
  id: string
  name: string
  type: 'discord' | 'slack' | 'ntfy'
  url: string
  events: string[]
  enabled: boolean
  created_at: string
  updated_at: string
}

export interface NotificationDestInput {
  name: string
  type: 'discord' | 'slack' | 'ntfy'
  url: string
  events: string[]
  enabled: boolean
}

export function parseIncidentServices(inc: Incident): string[] {
  try { return JSON.parse(inc.services) as string[] } catch { return [] }
}

export function parseIncidentNodes(inc: Incident): string[] {
  try { return JSON.parse(inc.node_names) as string[] } catch { return [] }
}

export function parseIncidentMetadata(inc: Incident): Record<string, unknown> {
  try { return JSON.parse(inc.metadata) as Record<string, unknown> } catch { return {} }
}

export async function fetchIncidents(params?: {
  status?: string
  confidence?: string
  service?: string
  limit?: number
}): Promise<IncidentsPage> {
  const qs = new URLSearchParams()
  if (params?.status) qs.set('status', params.status)
  if (params?.confidence) qs.set('confidence', params.confidence)
  if (params?.service) qs.set('service', params.service)
  if (params?.limit) qs.set('limit', String(params.limit))
  const url = '/api/incidents' + (qs.toString() ? '?' + qs.toString() : '')
  const res = await apiFetch(url)
  if (!res.ok) throw new Error(await readErrorMessage(res, 'Failed to fetch incidents'))
  return res.json() as Promise<IncidentsPage>
}

export async function fetchIncidentSummary(): Promise<IncidentSummary> {
  const res = await apiFetch('/api/incidents/summary')
  if (!res.ok) throw new Error(await readErrorMessage(res, 'Failed to fetch incident summary'))
  const data = await res.json() as { open_count: number, has_confirmed_open: boolean }
  return {
    openCount: data.open_count ?? 0,
    hasConfirmedOpen: data.has_confirmed_open === true,
  }
}

export async function fetchIncident(id: string): Promise<IncidentDetail> {
  const res = await apiFetch(`/api/incidents/${id}`)
  if (!res.ok) throw new Error(await readErrorMessage(res, 'Failed to fetch incident'))
  return res.json() as Promise<IncidentDetail>
}

export async function fetchIncidentsForEntryIds(entryIds: string[]): Promise<Record<string, IncidentMembership>> {
  if (entryIds.length === 0) return {}
  const res = await apiFetch('/api/incidents/membership', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ entry_ids: entryIds }),
  })
  if (!res.ok) throw new Error(await readErrorMessage(res, 'Failed to fetch incident membership'))
  const data = await res.json() as { memberships?: Record<string, IncidentMembership> }
  return data.memberships ?? {}
}

function normalizeNotificationDest(data: Record<string, unknown>): NotificationDest {
  const rawEvents = data.events
  const events = Array.isArray(rawEvents)
    ? rawEvents.map(event => String(event))
    : []

  return {
    id: String(data.id ?? ''),
    name: String(data.name ?? ''),
    type: (data.type === 'slack' || data.type === 'ntfy' ? data.type : 'discord'),
    url: String(data.url ?? ''),
    events,
    enabled: data.enabled === true,
    created_at: String(data.created_at ?? ''),
    updated_at: String(data.updated_at ?? ''),
  }
}

export async function listNotificationDests(): Promise<NotificationDest[]> {
  const res = await apiFetch('/api/admin/notifications')
  if (!res.ok) throw new Error(await readErrorMessage(res, 'Failed to list notification destinations'))
  const data = await res.json() as Record<string, unknown>[]
  return data.map(normalizeNotificationDest)
}

export async function createNotificationDest(data: NotificationDestInput): Promise<NotificationDest> {
  const res = await apiFetch('/api/admin/notifications', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  })
  if (!res.ok) throw new Error(await readErrorMessage(res, 'Failed to create notification destination'))
  const created = await res.json() as Record<string, unknown>
  return normalizeNotificationDest(created)
}

export async function updateNotificationDest(id: string, data: NotificationDestInput): Promise<NotificationDest> {
  const res = await apiFetch(`/api/admin/notifications/${encodeURIComponent(id)}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  })
  if (!res.ok) throw new Error(await readErrorMessage(res, 'Failed to update notification destination'))
  const updated = await res.json() as Record<string, unknown>
  return normalizeNotificationDest(updated)
}

export async function deleteNotificationDest(id: string): Promise<void> {
  const res = await apiFetch(`/api/admin/notifications/${encodeURIComponent(id)}`, {
    method: 'DELETE',
  })
  if (!res.ok) throw new Error(await readErrorMessage(res, 'Failed to delete notification destination'))
}

export async function testNotificationDest(id: string): Promise<{ ok: boolean; error?: string }> {
  const res = await apiFetch(`/api/admin/notifications/${encodeURIComponent(id)}/test`, {
    method: 'POST',
  })
  if (!res.ok) throw new Error(await readErrorMessage(res, 'Failed to test notification destination'))
  return res.json() as Promise<{ ok: boolean; error?: string }>
}
