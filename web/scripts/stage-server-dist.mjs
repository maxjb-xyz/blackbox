import { cpSync, existsSync, mkdirSync, rmSync } from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const scriptDir = path.dirname(fileURLToPath(import.meta.url))
const webDir = path.resolve(scriptDir, '..')
const sourceDir = path.join(webDir, 'dist')
const targetDir = path.resolve(webDir, '..', 'server', 'web', 'dist')

if (!existsSync(sourceDir)) {
  throw new Error(`frontend build output not found at ${sourceDir}`)
}

rmSync(targetDir, { recursive: true, force: true })
mkdirSync(targetDir, { recursive: true })
cpSync(sourceDir, targetDir, { recursive: true })
