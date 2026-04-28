import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import test from 'node:test'

const adminPagePath = new URL('../src/pages/AdminPage.tsx', import.meta.url)

test('github community section includes the buy me a coffee link', async () => {
  const source = await readFile(adminPagePath, 'utf8')

  assert.match(source, /https:\/\/buymeacoffee\.com\/maxjb/)
  assert.match(source, /BUY ME A COFFEE/)
  assert.match(source, /style=\{buyMeACoffeeBtnStyle\}/)
  assert.match(source, /background: '#ffdd00'/)
})
