import test from 'node:test'
import assert from 'node:assert/strict'

import {
  ADMIN_GROUPS,
  getAdminTabNavigationKey,
  getWrappedAdminTabIndex,
} from '../src/pages/adminNavigation.ts'

test('access group keeps users as its default first tab', () => {
  assert.equal(ADMIN_GROUPS.access.tabs[0], 'users')
})

test('desktop vertical submenu maps arrow keys to previous and next', () => {
  assert.equal(getAdminTabNavigationKey('ArrowUp', true), 'previous')
  assert.equal(getAdminTabNavigationKey('ArrowDown', true), 'next')
  assert.equal(getAdminTabNavigationKey('ArrowLeft', true), null)
  assert.equal(getAdminTabNavigationKey('ArrowRight', true), null)
})

test('mobile horizontal submenu maps arrow keys to previous and next', () => {
  assert.equal(getAdminTabNavigationKey('ArrowLeft', false), 'previous')
  assert.equal(getAdminTabNavigationKey('ArrowRight', false), 'next')
  assert.equal(getAdminTabNavigationKey('ArrowDown', false), null)
  assert.equal(getAdminTabNavigationKey('ArrowUp', false), null)
})

test('wrapped tab selection loops around the available tabs', () => {
  assert.equal(getWrappedAdminTabIndex(-1, 4), 3)
  assert.equal(getWrappedAdminTabIndex(-5, 4), 3)
  assert.equal(getWrappedAdminTabIndex(4, 4), 0)
  assert.equal(getWrappedAdminTabIndex(5, 4), 1)
})
