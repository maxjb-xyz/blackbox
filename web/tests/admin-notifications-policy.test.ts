import test from 'node:test'
import assert from 'node:assert/strict'

import { validatePolicyForm } from '../src/pages/notificationPolicy.ts'

test('rejects quiet hours enabled without valid times', () => {
  assert.ok(validatePolicyForm({
    quiet_hours_enabled: true,
    quiet_hours_start: '9am',
    quiet_hours_end: '07:00',
    quiet_hours_mode: 'drop',
    rate_limit_enabled: false,
    rate_limit_count: 0,
    rate_limit_unit: 'hour',
  }))
})

test('rejects rate limit count below 1', () => {
  assert.ok(validatePolicyForm({
    quiet_hours_enabled: false,
    quiet_hours_start: '22:00',
    quiet_hours_end: '07:00',
    quiet_hours_mode: 'drop',
    rate_limit_enabled: true,
    rate_limit_count: 0,
    rate_limit_unit: 'hour',
  }))
})

test('accepts a valid policy', () => {
  assert.equal(validatePolicyForm({
    quiet_hours_enabled: true,
    quiet_hours_start: '22:00',
    quiet_hours_end: '07:00',
    quiet_hours_mode: 'defer',
    rate_limit_enabled: true,
    rate_limit_count: 5,
    rate_limit_unit: 'day',
  }), null)
})

test('accepts all-disabled policy', () => {
  assert.equal(validatePolicyForm({
    quiet_hours_enabled: false,
    quiet_hours_start: '',
    quiet_hours_end: '',
    quiet_hours_mode: 'drop',
    rate_limit_enabled: false,
    rate_limit_count: 0,
    rate_limit_unit: 'hour',
  }), null)
})
