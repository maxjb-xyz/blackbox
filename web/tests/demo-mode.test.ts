import assert from 'node:assert/strict'
import test from 'node:test'

import {
  DEMO_INTRO_DISMISSED_KEY,
  DEMO_SESSION_USER,
  isDemoIntroDismissed,
  isDemoModeEnabled,
} from '../src/demoMode.ts'

test('isDemoModeEnabled only enables the explicit true flag', () => {
  assert.equal(isDemoModeEnabled('true'), true)
  assert.equal(isDemoModeEnabled('false'), false)
  assert.equal(isDemoModeEnabled(undefined), false)
})

test('isDemoIntroDismissed only accepts the persisted true marker', () => {
  assert.equal(DEMO_INTRO_DISMISSED_KEY, 'blackbox_demo_intro_dismissed')
  assert.equal(isDemoIntroDismissed('true'), true)
  assert.equal(isDemoIntroDismissed('false'), false)
  assert.equal(isDemoIntroDismissed(null), false)
})

test('DEMO_SESSION_USER is a fixed admin identity for the public demo', () => {
  assert.equal(DEMO_SESSION_USER.is_admin, true)
  assert.equal(DEMO_SESSION_USER.oidc_linked, false)
  assert.ok(DEMO_SESSION_USER.username.length > 0)
  assert.ok(DEMO_SESSION_USER.email.includes('@'))
})
