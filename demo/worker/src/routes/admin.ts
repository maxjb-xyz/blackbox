import { Hono } from 'hono'

import type { AuditLogEntry, DemoData, WebhookDelivery, WorkerBindings } from '../types'

function paginate<T>(items: readonly T[], page: number, perPage: number) {
  const safePage = Math.max(1, page)
  const safePerPage = Math.max(1, Math.min(200, perPage))
  const start = (safePage - 1) * safePerPage
  return {
    total: items.length,
    page: safePage,
    per_page: safePerPage,
    items: items.slice(start, start + safePerPage),
  }
}

function compareByCreatedAtDesc<T extends { created_at?: string; received_at?: string }>(left: T, right: T) {
  const leftValue = left.created_at ?? left.received_at ?? ''
  const rightValue = right.created_at ?? right.received_at ?? ''
  return Date.parse(rightValue) - Date.parse(leftValue)
}

function filterAuditLogs(items: readonly AuditLogEntry[], action?: string | null) {
  if (!action) return [...items].sort(compareByCreatedAtDesc)
  return items.filter(item => item.action.includes(action)).sort(compareByCreatedAtDesc)
}

function filterWebhookDeliveries(items: readonly WebhookDelivery[], source?: string | null, status?: string | null) {
  return items
    .filter(item => {
      if (source && item.source !== source) return false
      if (status && item.status !== status) return false
      return true
    })
    .sort(compareByCreatedAtDesc)
}

export function createAdminRouter(data: DemoData) {
  const app = new Hono<{ Bindings: WorkerBindings }>()

  app.get('/admin/users', c => c.json(data.users))
  app.get('/admin/config', c => c.json(data.adminConfig))
  app.get('/admin/settings', c => c.json(data.adminConfig))
  app.get('/admin/settings/systemd', c => {
    const units = Object.fromEntries(
      data.dataSourceInstances
        .filter(source => source.type === 'systemd' && source.node_id)
        .map(source => [source.node_id!, JSON.parse(source.config) as { units?: string[] }])
        .map(([nodeName, config]) => [nodeName, Array.isArray(config.units) ? config.units.map(value => String(value)) : []]),
    )
    return c.json(units)
  })
  app.get('/admin/audit-logs', c => {
    const page = Number(c.req.query('page') ?? '1')
    const perPage = Number(c.req.query('per_page') ?? '50')
    return c.json(paginate(filterAuditLogs(data.auditLogs, c.req.query('action')), page, perPage))
  })
  app.get('/admin/webhook-deliveries', c => {
    const page = Number(c.req.query('page') ?? '1')
    const perPage = Number(c.req.query('per_page') ?? '50')
    return c.json(paginate(filterWebhookDeliveries(data.webhookDeliveries, c.req.query('source'), c.req.query('status')), page, perPage))
  })
  app.get('/admin/oidc', c => c.json({ providers: data.oidcProviders, policy: data.oidcPolicy }))
  app.get('/admin/oidc/providers', c => c.json(data.oidcProviders))
  app.get('/admin/oidc/policy', c => c.json({ policy: data.oidcPolicy }))
  app.get('/admin/notifications', c => c.json(data.notificationDests))
  app.get('/admin/sources', c => c.json(data.sources))
  app.get('/admin/sources/types', c => c.json(data.sourceTypes))
  app.get('/admin/excluded-targets', c => c.json(data.excludedTargets))
  app.get('/admin/github/releases', c => c.json([]))

  return app
}
