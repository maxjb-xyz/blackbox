import assert from 'node:assert/strict'
import test from 'node:test'

import { createDemoData } from '../src/seed-data.ts'
import { buildIncidentMembership, filterIncidents } from '../src/routes/incidents.ts'

const demoData = createDemoData(Date.parse('2026-04-27T12:00:00.000Z'))

test('createDemoData yields the expected demo scale and recent timestamps', () => {
  assert.equal(demoData.nodes.length, 3)
  assert.equal(demoData.incidents.length, 15)
  assert.equal(demoData.users.length, 4)
  assert.ok(demoData.entries.length >= 390)
  assert.ok(demoData.entries.length <= 430)

  const newestTimestamp = Date.parse(demoData.entries[0].timestamp)
  const oldestTimestamp = Date.parse(demoData.entries[demoData.entries.length - 1].timestamp)
  assert.ok(newestTimestamp <= Date.parse('2026-04-27T12:00:00.000Z'))
  assert.ok(oldestTimestamp >= Date.parse('2026-04-13T00:00:00.000Z'))
})

test('filterIncidents applies status, service, and limit filters', () => {
  const open = filterIncidents(demoData.incidents, {
    status: 'open',
  })
  const vaultwarden = filterIncidents(demoData.incidents, {
    service: 'vaultwarden',
    limit: 3,
  })

  assert.ok(open.every(incident => incident.status === 'open'))
  assert.ok(vaultwarden.length >= 1)
  assert.ok(vaultwarden.every(incident => incident.services.includes('vaultwarden')))
  assert.ok(vaultwarden.length <= 3)
})

test('buildIncidentMembership maps entry ids to incident ids and confidence', () => {
  const linkedEntries = demoData.incidentLinks.slice(0, 3).map(link => link.entry_id)
  const memberships = buildIncidentMembership(demoData.incidents, demoData.incidentLinks, linkedEntries)

  assert.deepEqual(Object.keys(memberships).sort(), linkedEntries.slice().sort())
  for (const entryId of linkedEntries) {
    assert.ok(memberships[entryId])
    assert.ok(memberships[entryId]?.id)
    assert.ok(memberships[entryId]?.confidence === 'confirmed' || memberships[entryId]?.confidence === 'suspected')
  }
})
