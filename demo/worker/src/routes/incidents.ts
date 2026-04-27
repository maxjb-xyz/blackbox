import { Hono } from 'hono'

import type { DemoData, Entry, Incident, IncidentLinkRecord, WorkerBindings } from '../types'

interface IncidentFilters {
  status?: string | null
  confidence?: string | null
  service?: string | null
  limit?: number
}

function compareIncidentsDesc(left: Incident, right: Incident) {
  const timeDiff = Date.parse(right.opened_at) - Date.parse(left.opened_at)
  if (timeDiff !== 0) return timeDiff
  return right.id.localeCompare(left.id)
}

export function filterIncidents(incidents: readonly Incident[], filters: IncidentFilters) {
  const limit = Math.max(1, Math.min(100, filters.limit ?? incidents.length))
  return [...incidents]
    .filter(incident => {
      if (filters.status && incident.status !== filters.status) return false
      if (filters.confidence && incident.confidence !== filters.confidence) return false
      if (filters.service && !incident.services.includes(filters.service)) return false
      return true
    })
    .sort(compareIncidentsDesc)
    .slice(0, limit)
}

function compareEntriesAsc(left: Pick<Entry, 'timestamp' | 'id'>, right: Pick<Entry, 'timestamp' | 'id'>) {
  const timeDiff = Date.parse(left.timestamp) - Date.parse(right.timestamp)
  if (timeDiff !== 0) return timeDiff
  return left.id.localeCompare(right.id)
}

export function buildIncidentMembership(
  incidents: readonly Incident[],
  links: readonly IncidentLinkRecord[],
  entryIds: readonly string[],
) {
  const incidentsById = new Map(incidents.map(incident => [incident.id, incident]))
  const memberships: Record<string, { id: string; confidence: Incident['confidence'] }> = {}
  for (const link of links) {
    if (!entryIds.includes(link.entry_id)) continue
    const incident = incidentsById.get(link.incident_id)
    if (!incident) continue
    memberships[link.entry_id] = { id: incident.id, confidence: incident.confidence }
  }
  return memberships
}

function buildIncidentEntries(data: DemoData, incidentId: string) {
  const entriesById = new Map(data.entries.map(entry => [entry.id, entry]))
  return data.incidentLinks
    .filter(link => link.incident_id === incidentId)
    .map(link => {
      const entry = entriesById.get(link.entry_id)
      if (!entry) return null
      return {
        link,
        entry,
      }
    })
    .filter((item): item is { link: IncidentLinkRecord; entry: Entry } => item !== null)
    .sort((left, right) => compareEntriesAsc(left.entry, right.entry))
}

function escapePdfText(value: string) {
  return value.replace(/\\/g, '\\\\').replace(/\(/g, '\\(').replace(/\)/g, '\\)')
}

function buildSimplePdf(lines: string[]) {
  const content = ['BT', '/F1 12 Tf', '40 760 Td']
  lines.forEach((line, index) => {
    if (index > 0) content.push('0 -18 Td')
    content.push(`(${escapePdfText(line)}) Tj`)
  })
  content.push('ET')
  const stream = content.join('\n')
  const objects = [
    '1 0 obj << /Type /Catalog /Pages 2 0 R >> endobj',
    '2 0 obj << /Type /Pages /Count 1 /Kids [3 0 R] >> endobj',
    '3 0 obj << /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >> endobj',
    `4 0 obj << /Length ${stream.length} >> stream\n${stream}\nendstream endobj`,
    '5 0 obj << /Type /Font /Subtype /Type1 /BaseFont /Courier >> endobj',
  ]

  let pdf = '%PDF-1.4\n'
  const offsets: number[] = []
  for (const object of objects) {
    offsets.push(pdf.length)
    pdf += `${object}\n`
  }
  const xrefOffset = pdf.length
  pdf += `xref\n0 ${objects.length + 1}\n0000000000 65535 f \n`
  for (const offset of offsets) {
    pdf += `${String(offset).padStart(10, '0')} 00000 n \n`
  }
  pdf += `trailer << /Size ${objects.length + 1} /Root 1 0 R >>\nstartxref\n${xrefOffset}\n%%EOF`
  return new TextEncoder().encode(pdf)
}

export function createIncidentsRouter(data: DemoData) {
  const app = new Hono<{ Bindings: WorkerBindings }>()

  app.get('/incidents', c => {
    const limit = Number(c.req.query('limit') ?? '100')
    const incidents = filterIncidents(data.incidents, {
      status: c.req.query('status'),
      confidence: c.req.query('confidence'),
      service: c.req.query('service'),
      limit: Number.isFinite(limit) ? limit : data.incidents.length,
    })
    return c.json({ incidents, has_more: false })
  })

  app.get('/incidents/summary', c => {
    const openIncidents = data.incidents.filter(incident => incident.status === 'open')
    return c.json({
      open_count: openIncidents.length,
      has_confirmed_open: openIncidents.some(incident => incident.confidence === 'confirmed'),
    })
  })

  app.post('/incidents/membership', async c => {
    const body = await c.req.json().catch(() => ({})) as { entry_ids?: unknown }
    const entryIds = Array.isArray(body.entry_ids) ? body.entry_ids.map(value => String(value)) : []
    return c.json({ memberships: buildIncidentMembership(data.incidents, data.incidentLinks, entryIds) })
  })

  app.get('/incidents/:id/report.pdf', c => {
    const incident = data.incidents.find(candidate => candidate.id === c.req.param('id'))
    if (!incident) return c.json({ error: 'Incident not found' }, 404)

    const lines = [
      `Blackbox Incident Report: ${incident.id}`,
      `Title: ${incident.title}`,
      `Status: ${incident.status}`,
      `Confidence: ${incident.confidence}`,
      `Opened: ${incident.opened_at}`,
      `Resolved: ${incident.resolved_at ?? 'still open'}`,
      `Services: ${incident.services.join(', ')}`,
      `Nodes: ${incident.node_names.join(', ')}`,
    ]

    return new Response(buildSimplePdf(lines), {
      headers: {
        'content-type': 'application/pdf',
        'content-disposition': `attachment; filename="incident-${incident.id}-report.pdf"`,
      },
    })
  })

  app.get('/incidents/:id', c => {
    const incident = data.incidents.find(candidate => candidate.id === c.req.param('id'))
    if (!incident) return c.json({ error: 'Incident not found' }, 404)
    return c.json({
      incident,
      entries: buildIncidentEntries(data, incident.id),
    })
  })

  return app
}
