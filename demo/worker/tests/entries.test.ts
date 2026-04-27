import assert from 'node:assert/strict'
import test from 'node:test'

import { filterEntries, paginateEntries } from '../src/routes/entries.ts'

const SAMPLE_ENTRIES = [
  {
    id: 'entry-1',
    timestamp: '2026-04-27T11:00:00.000Z',
    node_name: 'mikoshi',
    source: 'docker',
    service: 'vaultwarden',
    event: 'start',
    content: 'Vaultwarden started cleanly',
    metadata: {},
  },
  {
    id: 'entry-2',
    timestamp: '2026-04-27T10:30:00.000Z',
    node_name: 'atlas',
    source: 'webhook',
    service: 'caddy',
    event: 'down',
    content: 'Atlas edge probe failed with 502',
    metadata: { monitor: 'edge-proxy' },
  },
  {
    id: 'entry-3',
    timestamp: '2026-04-27T10:00:00.000Z',
    node_name: 'hexcore',
    source: 'docker',
    service: 'ollama',
    event: 'die',
    content: 'Ollama exited with code 137',
    metadata: {},
  },
  {
    id: 'entry-4',
    timestamp: '2026-04-27T09:30:00.000Z',
    node_name: 'mikoshi',
    source: 'agent',
    service: 'blackbox-agent',
    event: 'heartbeat',
    content: 'agent heartbeat',
    metadata: {},
  },
] as const

test('filterEntries applies node, source, service, q, hideHeartbeat, and time window filters', () => {
  const filtered = filterEntries(SAMPLE_ENTRIES, {
    node: 'atlas',
    source: 'webhook',
    service: 'caddy',
    q: '502',
    hideHeartbeat: true,
    timeStart: '2026-04-27T10:15:00.000Z',
    timeEnd: '2026-04-27T10:45:00.000Z',
  })

  assert.deepEqual(filtered.map(entry => entry.id), ['entry-2'])
})

test('paginateEntries returns a stable next cursor for descending pages', () => {
  const page = paginateEntries(SAMPLE_ENTRIES, {
    limit: 2,
  })

  assert.deepEqual(page.entries.map(entry => entry.id), ['entry-1', 'entry-2'])
  assert.equal(page.nextCursor, '2026-04-27T10:30:00.000Z::entry-2')
})

test('paginateEntries supports cursor/before pagination and after pagination', () => {
  const olderPage = paginateEntries(SAMPLE_ENTRIES, {
    limit: 2,
    cursor: '2026-04-27T10:30:00.000Z::entry-2',
  })
  const newerPage = paginateEntries(SAMPLE_ENTRIES, {
    limit: 10,
    after: '2026-04-27T10:00:00.000Z::entry-3',
  })

  assert.deepEqual(olderPage.entries.map(entry => entry.id), ['entry-3', 'entry-4'])
  assert.equal(olderPage.nextCursor, undefined)
  assert.deepEqual(newerPage.entries.map(entry => entry.id), ['entry-1', 'entry-2'])
})
