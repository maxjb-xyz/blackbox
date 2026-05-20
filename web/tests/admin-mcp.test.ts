import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import test from 'node:test'

import { createDemoData } from '../../demo/worker/src/seed-data.ts'

const adminMcpHelperPath = new URL('../src/pages/adminMcp.ts', import.meta.url)
const adminPagePath = new URL('../src/pages/AdminPage.tsx', import.meta.url)
const apiClientPath = new URL('../src/api/client.ts', import.meta.url)
const demoTypesPath = new URL('../../demo/worker/src/types.ts', import.meta.url)

test('resolveMcpEndpointUrl prefers the backend value and falls back to origin', async () => {
  const helperModule = await import('../src/pages/adminMcp.ts').catch(() => null)

  assert.ok(helperModule, `Expected helper module at ${adminMcpHelperPath.pathname}`)
  assert.equal(
    helperModule.resolveMcpEndpointUrl('https://admin.example.test/custom-mcp', 'https://app.example.test'),
    'https://admin.example.test/custom-mcp',
  )
  assert.equal(
    helperModule.resolveMcpEndpointUrl('', 'https://app.example.test'),
    'https://app.example.test/mcp',
  )
  assert.equal(
    helperModule.resolveMcpEndpointUrl('   ', 'https://app.example.test/root/'),
    'https://app.example.test/root/mcp',
  )
  assert.equal(
    helperModule.resolveMcpEndpointUrl('', 'https://app.example.test', 'https://configured.example.test?bad=1'),
    '',
  )
})

test('admin MCP contract uses endpoint url and warning fields instead of port', async () => {
  const [clientSource, adminPageSource, demoTypesSource] = await Promise.all([
    readFile(apiClientPath, 'utf8'),
    readFile(adminPagePath, 'utf8'),
    readFile(demoTypesPath, 'utf8'),
  ])

  assert.match(clientSource, /mcp_endpoint_url:\s*string/)
  assert.match(clientSource, /mcp_migration_warning:\s*boolean/)
  assert.doesNotMatch(clientSource, /mcp_port/)

  assert.match(adminPageSource, /mcp_endpoint_url/)
  assert.doesNotMatch(adminPageSource, /MCP PORT/)
  assert.doesNotMatch(adminPageSource, /mcp_port/)

  assert.match(demoTypesSource, /mcp_endpoint_url:\s*string/)
  assert.match(demoTypesSource, /mcp_migration_warning:\s*boolean/)
  assert.doesNotMatch(demoTypesSource, /mcp_port/)
})

test('client and admin page support acknowledge-only MCP migration warning updates', async () => {
  const [clientSource, adminPageSource] = await Promise.all([
    readFile(apiClientPath, 'utf8'),
    readFile(adminPagePath, 'utf8'),
  ])

  assert.match(clientSource, /mcp_enabled\?:\s*boolean/)
  assert.match(clientSource, /acknowledge_mcp_migration_warning\?:\s*true/)
  assert.match(adminPageSource, /updateMCPSettings\(\{\s*acknowledge_mcp_migration_warning:\s*true\s*\}\)/)
})

test('demo seed data does not surface an undismissable MCP migration warning', () => {
  const demoData = createDemoData(Date.parse('2026-04-27T12:00:00.000Z'))

  assert.equal(demoData.adminConfig.mcp_migration_warning, false)
})
