import test from 'node:test'
import assert from 'node:assert/strict'

import {
  getSourceCardColors,
  getSourceIconSpec,
} from '../src/components/sourceIcons.ts'

test('brand-backed sources resolve to branded icon specs', () => {
  assert.deepEqual(getSourceIconSpec('docker'), { kind: 'brand', name: 'docker' })
  assert.deepEqual(getSourceIconSpec('webhook_uptime_kuma'), { kind: 'brand', name: 'uptime-kuma' })
  assert.deepEqual(getSourceIconSpec('webhook_watchtower'), { kind: 'brand', name: 'watchtower' })
})

test('non-branded sources resolve to generic icon specs', () => {
  assert.deepEqual(getSourceIconSpec('systemd'), { kind: 'generic', name: 'systemd' })
  assert.deepEqual(getSourceIconSpec('filewatcher'), { kind: 'generic', name: 'filewatcher' })
  assert.deepEqual(getSourceIconSpec('unknown_source'), { kind: 'generic', name: 'fallback' })
})

test('watchtower keeps its own color identity distinct from uptime kuma', () => {
  const kuma = getSourceCardColors('webhook_uptime_kuma')
  const watchtower = getSourceCardColors('webhook_watchtower')

  assert.notDeepEqual(watchtower, kuma)
  assert.equal(watchtower.text, '#57b8d9')
})
