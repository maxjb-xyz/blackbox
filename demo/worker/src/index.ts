import { Hono } from 'hono'

import { demoData } from './seed-data'
import { createAdminRouter } from './routes/admin'
import { createEntriesRouter } from './routes/entries'
import { createIncidentsRouter } from './routes/incidents'
import { createNodesRouter } from './routes/nodes'
import indexHtml from '../public/index.html'

const app = new Hono()

app.get('/api/setup/status', c => c.json(demoData.setupStatus))
app.get('/api/setup/health', c => c.json(demoData.healthStatus))

app.get('/api/auth/status', c => c.json({
  id: demoData.sessionUser.user_id,
  username: demoData.sessionUser.username,
  email: demoData.sessionUser.email,
  is_admin: demoData.sessionUser.is_admin,
  token_version: 8,
}))
app.get('/api/auth/me', c => c.json({ user: demoData.sessionUser }))
app.get('/api/auth/oidc/providers', c => c.json({
  providers: demoData.oidcProviders
    .filter(provider => provider.enabled)
    .map(provider => ({ id: provider.id, name: provider.name })),
}))
app.get('/api/auth/invite', c => c.json(demoData.invites))

app.post('/api/auth/login', c => c.json({ user: demoData.sessionUser }))
app.post('/api/auth/logout', c => c.body(null, 204))

app.get('/api/webhooks', c => c.json(demoData.webhooks))
app.get('/api/ws', c => c.json({ error: 'WebSocket disabled in demo mode' }, 404))

app.route('/api', createEntriesRouter(demoData))
app.route('/api', createIncidentsRouter(demoData))
app.route('/api', createNodesRouter(demoData))
app.route('/api', createAdminRouter(demoData))

app.on(['POST', 'PUT', 'PATCH', 'DELETE'], '/api/*', c => c.json({ error: 'Disabled in demo mode' }, 403))
app.all('/api/*', c => c.json({ error: 'Not found' }, 404))

app.get('*', c => c.html(indexHtml))

export default app
