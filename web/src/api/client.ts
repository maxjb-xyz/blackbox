export interface SetupStatus {
  bootstrapped: boolean
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

function authHeaders(): Record<string, string> {
  const token = localStorage.getItem('token')
  return token ? { Authorization: `Bearer ${token}` } : {}
}

export async function checkSetupStatus(): Promise<SetupStatus> {
  const res = await fetch('/api/setup/status')
  if (!res.ok) throw new Error('Failed to check setup status')
  return res.json()
}

export async function checkHealth(): Promise<HealthStatus> {
  const res = await fetch('/api/setup/health')
  if (!res.ok) throw new Error('Failed to check health')
  return res.json()
}

export async function bootstrap(username: string, password: string): Promise<string> {
  const res = await fetch('/api/auth/bootstrap', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username, password }),
  })
  if (!res.ok) {
    const data = await res.json().catch(() => ({}))
    throw new Error((data as { error?: string }).error ?? 'Bootstrap failed')
  }
  const data = await res.json()
  return (data as { token: string }).token
}

export async function login(username: string, password: string): Promise<string> {
  const res = await fetch('/api/auth/login', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username, password }),
  })
  if (!res.ok) {
    const data = await res.json().catch(() => ({}))
    throw new Error((data as { error?: string }).error ?? 'Login failed')
  }
  const data = await res.json()
  return (data as { token: string }).token
}

export async function fetchNodes(): Promise<Node[]> {
  const res = await fetch('/api/nodes', { headers: authHeaders() })
  if (!res.ok) throw new Error('Failed to fetch nodes')
  return res.json()
}

export async function fetchEntries(params: {
  cursor?: string
  limit?: number
  node?: string
  source?: string
  q?: string
}): Promise<EntriesPage> {
  const url = new URL('/api/entries', window.location.origin)
  if (params.cursor) url.searchParams.set('cursor', params.cursor)
  if (params.limit) url.searchParams.set('limit', String(params.limit))
  if (params.node) url.searchParams.set('node', params.node)
  if (params.source) url.searchParams.set('source', params.source)
  if (params.q) url.searchParams.set('q', params.q)

  const res = await fetch(url.toString(), { headers: authHeaders() })
  if (!res.ok) throw new Error('Failed to fetch entries')
  return res.json()
}

export async function fetchEntry(id: string): Promise<Entry> {
  const res = await fetch(`/api/entries/${id}`, { headers: authHeaders() })
  if (!res.ok) throw new Error('Entry not found')
  return res.json()
}

export async function fetchNotes(entryId: string): Promise<EntryNote[]> {
  const res = await fetch(`/api/entries/${entryId}/notes`, { headers: authHeaders() })
  if (!res.ok) throw new Error('Failed to fetch notes')
  return res.json()
}

export async function createNote(entryId: string, content: string): Promise<EntryNote> {
  const res = await fetch(`/api/entries/${entryId}/notes`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', ...authHeaders() },
    body: JSON.stringify({ content }),
  })
  if (!res.ok) throw new Error('Failed to create note')
  return res.json()
}

export async function deleteNote(noteId: string): Promise<void> {
  const res = await fetch(`/api/notes/${noteId}`, {
    method: 'DELETE',
    headers: authHeaders(),
  })
  if (!res.ok) throw new Error('Failed to delete note')
}
