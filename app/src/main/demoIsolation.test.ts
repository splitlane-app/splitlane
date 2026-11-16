import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

const root = resolve(__dirname, '../..')
const read = (rel: string): string => readFileSync(resolve(root, rel), 'utf8')

describe('the demo stays out of the product', () => {
  it('has no demo input in electron.vite.config.ts', () => {
    const config = read('electron.vite.config.ts')
    expect(config).not.toMatch(/demo/i)
    expect(config).toContain("'src/main/index.ts'")
    expect(config).toContain("'src/preload/index.ts'")
    expect(config).toContain("'src/renderer/index.html'")
  })

  it('is built by its own config into out-demo, not out', () => {
    const config = read('vite.demo.config.ts')
    expect(config).toContain("'out-demo'")
    expect(config).toContain("'src/renderer/demo/demo.html'")
  })

  it('is not imported by the page', () => {
    expect(read('src/renderer/main.tsx')).not.toMatch(/demo/i)
    expect(read('src/renderer/App.tsx')).not.toMatch(/demo/i)
  })

  it('has the icon file and the output folder git-ignored', () => {
    const ignore = readFileSync(resolve(root, '../.gitignore'), 'utf8')
    expect(ignore).toContain('/app/out-demo/')
    expect(ignore).toContain('/app/src/renderer/demo/icons.local.json')
  })
})
