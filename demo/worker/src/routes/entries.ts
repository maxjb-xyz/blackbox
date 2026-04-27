import { Hono } from 'hono'

import type { DemoData, Entry, WorkerBindings } from '../types'

interface EntryFilters {
  node?: string | null
  source?: string | null
  service?: string | null
  q?: string | null
  hideHeartbeat?: boolean
  timeStart?: string | null
  timeEnd?: string | null
}

interface EntryPagination {
  before?: string | null
  after?: string | null
  cursor?: string | null
  limit?: number
}

function entryCursor(entry: Pick<Entry, 'timestamp' | 'id'>): string {
  return `${entry.timestamp}::${entry.id}`
}

function parseCursor(raw: string | null | undefined): { timestamp: string; id: string } | null {
  if (!raw) return null
  const [timestamp, id] = raw.split('::')
  if (!timestamp) return null
  return { timestamp, id: id ?? '' }
}

function compareEntriesDesc(left: Pick<Entry, 'timestamp' | 'id'>, right: Pick<Entry, 'timestamp' | 'id'>) {
  const timeDiff = Date.parse(right.timestamp) - Date.parse(left.timestamp)
  if (timeDiff !== 0) return timeDiff
  return right.id.localeCompare(left.id)
}

function isBeforeCursor(entry: Pick<Entry, 'timestamp' | 'id'>, cursor: { timestamp: string; id: string }) {
  if (entry.timestamp < cursor.timestamp) return true
  if (entry.timestamp > cursor.timestamp) return false
  return entry.id < cursor.id
}

function isAfterCursor(entry: Pick<Entry, 'timestamp' | 'id'>, cursor: { timestamp: string; id: string }) {
  if (entry.timestamp > cursor.timestamp) return true
  if (entry.timestamp < cursor.timestamp) return false
  return entry.id > cursor.id
}

export function filterEntries(entries: readonly Entry[], filters: EntryFilters): Entry[] {
  const query = filters.q?.trim().toLowerCase()
  const timeStartMs = filters.timeStart ? Date.parse(filters.timeStart) : Number.NaN
  const timeEndMs = filters.timeEnd ? Date.parse(filters.timeEnd) : Number.NaN

  return entries.filter(entry => {
    if (filters.hideHeartbeat && entry.source === 'agent' && entry.event === 'heartbeat') return false
    if (filters.node && entry.node_name !== filters.node) return false
    if (filters.source && entry.source !== filters.source) return false
    if (filters.service && entry.service !== filters.service) return false

    const timestampMs = Date.parse(entry.timestamp)
    if (!Number.isNaN(timeStartMs) && timestampMs < timeStartMs) return false
    if (!Number.isNaN(timeEndMs) && timestampMs > timeEndMs) return false

    if (query) {
      const haystack = [
        entry.node_name,
        entry.source,
        entry.service,
        entry.compose_service ?? '',
        entry.event,
        entry.content,
        JSON.stringify(entry.metadata),
      ].join(' ').toLowerCase()
      if (!haystack.includes(query)) return false
    }

    return true
  })
}

export function paginateEntries(entries: readonly Entry[], options: EntryPagination) {
  const limit = Math.max(1, Math.min(200, options.limit ?? 50))
  const beforeCursor = parseCursor(options.cursor ?? options.before)
  const afterCursor = parseCursor(options.after)

  const sorted = [...entries].sort(compareEntriesDesc)
  const windowed = sorted.filter(entry => {
    if (beforeCursor && !isBeforeCursor(entry, beforeCursor)) return false
    if (afterCursor && !isAfterCursor(entry, afterCursor)) return false
    return true
  })

  const pageEntries = windowed.slice(0, limit)
  const hasMore = windowed.length > limit

  return {
    entries: pageEntries,
    nextCursor: hasMore && pageEntries.length > 0 ? entryCursor(pageEntries[pageEntries.length - 1]) : undefined,
  }
}

export function createEntriesRouter(data: DemoData) {
  const app = new Hono<{ Bindings: WorkerBindings }>()

  app.get('/entries', c => {
    const limit = Number(c.req.query('limit') ?? '50')
    const filtered = filterEntries(data.entries, {
      node: c.req.query('node'),
      source: c.req.query('source'),
      service: c.req.query('service'),
      q: c.req.query('q'),
      hideHeartbeat: c.req.query('hide_heartbeat') === 'true',
    })
    const page = paginateEntries(filtered, {
      before: c.req.query('before'),
      after: c.req.query('after'),
      cursor: c.req.query('cursor'),
      limit: Number.isFinite(limit) ? limit : 50,
    })

    return c.json({
      entries: page.entries,
      ...(page.nextCursor ? { next_cursor: page.nextCursor } : {}),
    })
  })

  app.get('/entries/services', c => {
    const services = [...new Set(data.entries.map(entry => entry.service).filter(Boolean))].sort((left, right) => left.localeCompare(right))
    return c.json({ services })
  })

  app.get('/entries/:id/notes', c => {
    const entry = data.entries.find(candidate => candidate.id === c.req.param('id'))
    if (!entry) return c.json({ error: 'Entry not found' }, 404)
    return c.json({ notes: [], has_more: false })
  })

  app.get('/entries/:id', c => {
    const entry = data.entries.find(candidate => candidate.id === c.req.param('id'))
    if (!entry) return c.json({ error: 'Entry not found' }, 404)
    return c.json(entry)
  })

  return app
}
