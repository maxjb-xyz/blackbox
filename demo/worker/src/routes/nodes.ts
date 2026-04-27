import { Hono } from 'hono'

import type { DemoData, WorkerBindings } from '../types'

export function createNodesRouter(data: DemoData) {
  const app = new Hono<{ Bindings: WorkerBindings }>()

  app.get('/nodes', c => c.json(data.nodes))

  return app
}
