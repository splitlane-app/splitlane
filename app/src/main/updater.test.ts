import { createHash } from 'node:crypto'
import { mkdtemp, mkdir, readdir, rm, writeFile } from 'node:fs/promises'
import { createServer, type IncomingMessage, type Server, type ServerResponse } from 'node:http'
import type { AddressInfo } from 'node:net'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'
import { DEFAULT_SETTINGS, type WindowSettings } from '../shared/settings'
import { feedFrom, type Feed, type UpdateState } from '../shared/update'
import { abortable, nodeFs, runInstallerScript, Updater, type HopResponse, type UpdaterDeps } from './updater'

type Handler = (req: IncomingMessage, res: ServerResponse) => void | Promise<void>

const API = '/repos/splitlane-app/splitlane/releases/latest'
const NAME = 'SplitLane-Setup-9.9.9.exe'
const EXE = `/splitlane-app/splitlane/releases/download/v9.9.9/${NAME}`
const SHA = `${EXE}.sha256`
const PAYLOAD = Buffer.from(Array.from({ length: 1000 }, (_, i) => i % 251))
const GOOD_SHA = `${createHash('sha256').update(PAYLOAD).digest('hex')}  ${NAME}\n`

function releaseJson(feed: Feed, version = '9.9.9', size = PAYLOAD.length): object {
  const name = `SplitLane-Setup-${version}.exe`
  const base = `${feed.assetPrefix}v${version}/${name}`
  return {
    tag_name: `v${version}`,
    draft: false,
    prerelease: false,
    assets: [
      { name, size, browser_download_url: base },
      { name: `${name}.sha256`, size: 100, browser_download_url: `${base}.sha256` }
    ]
  }
}

let server: Server
let origin = ''
let feed: Feed
let routes: Record<string, Handler> = {}
let hits: string[] = []
let root = ''

beforeAll(async () => {
  server = createServer((req, res) => {
    const path = req.url ?? ''
    hits.push(path)
    const h = routes[path]
    if (!h) {
      res.writeHead(404).end()
      return
    }
    void Promise.resolve(h(req, res)).catch(() => res.destroy())
  })
  await new Promise<void>((r) => server.listen(0, '127.0.0.1', r))
  origin = `http://127.0.0.1:${(server.address() as AddressInfo).port}`
  feed = feedFrom(origin).feed
})

afterAll(async () => {
  server.closeAllConnections()
  await new Promise((r) => server.close(r))
})

beforeEach(async () => {
  routes = {}
  hits = []
  root = await mkdtemp(join(tmpdir(), 'splitlane-updater-test-'))
})

afterEach(async () => {
  await rm(root, { recursive: true, force: true })
})

const json = (o: object): Handler => (_q, res) => {
  res.writeHead(200, { 'content-type': 'application/json' }).end(JSON.stringify(o))
}
const text = (s: string): Handler => (_q, res) => {
  res.writeHead(200).end(s)
}
const status = (code: number): Handler => (_q, res) => {
  res.writeHead(code).end('x')
}
function serveGood(): void {
  routes[API] = json(releaseJson(feed))
  routes[SHA] = text(GOOD_SHA)
  routes[EXE] = (_q, res) => {
    res.writeHead(200, { 'content-length': String(PAYLOAD.length) })
    res.end(PAYLOAD)
  }
}

async function fetchHop(url: string, signal: AbortSignal): Promise<HopResponse> {
  const r = await fetch(url, { redirect: 'manual', signal })
  const len = r.headers.get('content-length')
  return {
    status: r.status,
    location: r.headers.get('location'),
    body: r.body as unknown as AsyncIterable<Uint8Array> | null,
    contentLength: len === null ? null : Number(len)
  }
}

function fakeClock() {
  let now = 1_000
  let seq = 0
  const timers = new Map<number, { at: number; fn: () => void }>()
  return {
    now: () => now,
    setTimeout: (fn: () => void, ms: number): unknown => {
      const id = ++seq
      timers.set(id, { at: now + ms, fn })
      return id
    },
    clearTimeout: (h: unknown): void => {
      timers.delete(h as number)
    },
    advance(ms: number): void {
      const target = now + ms
      for (;;) {
        const due = [...timers.entries()].filter(([, t]) => t.at <= target).sort((a, b) => a[1].at - b[1].at)[0]
        if (!due) break
        timers.delete(due[0])
        now = Math.max(now, due[1].at)
        due[1].fn()
      }
      now = target
    }
  }
}

interface Rig {
  u: Updater
  states: UpdateState[]
  logs: string[]
  sets: Partial<WindowSettings>[]
  settings: WindowSettings
  clock: ReturnType<typeof fakeClock>
  runInstaller: ReturnType<typeof vi.fn>
  tempDir: string
  lastInstallFile: string
}

function rig(o: Partial<Omit<UpdaterDeps, 'settings'>> & { settings?: Partial<WindowSettings>; installer?: 'started' | 'cancelled' | { error: string }; setHook?: () => Promise<void> } = {}): Rig {
  const states: UpdateState[] = []
  const logs: string[] = []
  const sets: Partial<WindowSettings>[] = []
  const { settings: initial, installer, setHook, ...over } = o
  const settings: WindowSettings = { ...DEFAULT_SETTINGS, ...initial }
  const clock = fakeClock()
  const runInstaller = vi.fn(async () => installer ?? 'started')
  const tempDir = join(root, 'SplitLane-update')
  const lastInstallFile = join(root, 'last-install.txt')
  const deps: UpdaterDeps = {
    appVersion: '1.0.0',
    feed,
    canUpdate: true,
    hop: fetchHop,
    tempDir,
    fs: nodeFs(),
    runInstaller,
    settings: {
      get: () => ({ ...settings }),
      set: async (p) => {
        if (p.pendingUpdate !== null && p.pendingUpdate !== undefined) await setHook?.()
        sets.push(p)
        Object.assign(settings, p)
      }
    },
    lastInstallFile,
    clock,
    log: (l) => logs.push(l),
    changed: (s) => states.push(s),
    ...over
  }
  return { u: new Updater(deps), states, logs, sets, settings, clock, runInstaller, tempDir, lastInstallFile }
}

async function files(dir: string): Promise<string[]> {
  try {
    return await readdir(dir)
  } catch {
    return []
  }
}

const flush = (): Promise<void> => new Promise((r) => setTimeout(r, 30))

/** A hop that answers from a script (no network); records the requested addresses. */
function scripted(answers: Record<string, HopResponse[] | HopResponse>): { hop: UpdaterDeps['hop']; urls: string[] } {
  const urls: string[] = []
  return {
    urls,
    hop: async (url) => {
      urls.push(url)
      const a = answers[url]
      if (a === undefined) throw new Error(`unexpected request to ${url}`)
      return Array.isArray(a) ? (a.shift() as HopResponse) : a
    }
  }
}
const bodyOf = (b: Uint8Array | string): AsyncIterable<Uint8Array> =>
  (async function* () {
    yield typeof b === 'string' ? Buffer.from(b) : b
  })()
const ok = (b: Uint8Array | string): HopResponse => ({
  status: 200,
  location: null,
  body: bodyOf(b),
  contentLength: null
})
const redirect = (location: string, code = 302): HopResponse => ({ status: code, location, body: null, contentLength: null })

const GH = feedFrom(undefined).feed
const GH_ROOT = 'https://github.com/splitlane-app/splitlane/releases/download/v9.9.9'
const GH_API = GH.api
const ghApi = (): HopResponse => ok(JSON.stringify(releaseJson(GH)))

describe('check', () => {
  it('finds a newer release', async () => {
    serveGood()
    const r = rig()
    await r.u.check(false)
    expect(r.u.state()).toEqual({
      kind: 'available',
      version: '9.9.9',
      notesUrl: 'https://github.com/splitlane-app/splitlane/releases/tag/v9.9.9',
      hidden: false
    })
  })

  it('manual check without a newer release says up to date', async () => {
    routes[API] = json(releaseJson(feed, '1.0.0'))
    const r = rig()
    await r.u.check(true)
    expect(r.u.state()).toEqual({ kind: 'upToDate' })
    await r.u.check(false)
    r.u.dismiss()
    expect(r.u.state()).toEqual({ kind: 'idle' })
  })

  it('automatic check failure shows nothing', async () => {
    routes[API] = status(403)
    const r = rig()
    await r.u.check(false)
    expect(r.u.state()).toEqual({ kind: 'idle' })
    expect(r.states.some((s) => s.kind === 'checkFailed')).toBe(false)
    expect(r.logs.join('\n')).toContain('HTTP 403')
  })

  it('an automatic failure keeps an available release', async () => {
    serveGood()
    const r = rig()
    await r.u.check(false)
    routes[API] = status(500)
    await r.u.check(false)
    expect(r.u.state().kind).toBe('available')
  })

  it('manual check failure shows the reason', async () => {
    routes[API] = status(403)
    const r = rig()
    await r.u.check(true)
    expect(r.u.state()).toEqual({ kind: 'checkFailed', reason: 'HTTP 403 (GitHub request limit?)' })
    r.u.dismiss()
    expect(r.u.state()).toEqual({ kind: 'idle' })
    routes[API] = text('not json')
    await r.u.check(true)
    expect(r.u.state().kind).toBe('checkFailed')
  })

  it('does nothing when the copy is not installed, and says so on a manual check', async () => {
    serveGood()
    const r = rig({ canUpdate: false })
    await r.u.check(false)
    expect(r.u.state()).toEqual({ kind: 'idle' })
    await r.u.check(true)
    expect(r.u.state().kind).toBe('checkFailed')
    expect(hits).toEqual([])
  })

  it('later hides the offer; an automatic check of the same version keeps it hidden', async () => {
    serveGood()
    const r = rig()
    await r.u.check(false)
    r.u.later()
    expect(r.u.state()).toMatchObject({ kind: 'available', hidden: true })
    await r.u.check(false)
    expect(r.u.state()).toMatchObject({ kind: 'available', hidden: true })
    await r.u.check(true)
    expect(r.u.state()).toMatchObject({ kind: 'available', hidden: false })
  })

  it('an automatic check does not replace a result banner', async () => {
    serveGood()
    for (const pendingUpdate of ['1.2.0', '1.0.0']) {
      const r = rig({ settings: { pendingUpdate } })
      await r.u.start()
      r.u.stop()
      const before = r.u.state()
      expect(['installRefused', 'installed']).toContain(before.kind)
      const n = hits.length
      await r.u.check(false)
      expect(r.u.state()).toEqual(before)
      expect(hits.length).toBe(n)
      expect(r.logs.join('\n')).toContain('skipped')
    }
    // a failed download is a result too
    routes[SHA] = text('garbage')
    const f = rig()
    await f.u.check(false)
    await f.u.update()
    const failed = f.u.state()
    expect(failed.kind).toBe('downloadFailed')
    await f.u.check(false)
    expect(f.u.state()).toEqual(failed)
  })

  it('an automatic check is silent while it runs', async () => {
    let release!: () => void
    const gate = new Promise<void>((res) => (release = res))
    routes[API] = async (_q, res) => {
      await gate
      res.writeHead(200).end(JSON.stringify(releaseJson(feed)))
    }
    const r = rig()
    const p = r.u.check(false)
    await vi.waitFor(() => expect(hits).toContain(API))
    expect(r.u.state()).toEqual({ kind: 'idle' })
    expect(r.states).toEqual([])
    release()
    await p
    expect(r.states.map((x) => x.kind)).toEqual(['available'])
    // a failure leaves the state alone and only logs
    routes[API] = status(500)
    await r.u.check(false)
    expect(r.u.state().kind).toBe('available')
    expect(r.states.map((x) => x.kind)).toEqual(['available'])
    expect(r.logs.join('\n')).toContain('HTTP 500')
  })

  it('a check refuses an answer over 1 MB', async () => {
    routes[API] = (_q, res) => void res.writeHead(200).end(Buffer.alloc(1024 * 1024 + 1, 0x20))
    const r = rig()
    await r.u.check(true)
    expect(r.u.state()).toEqual({ kind: 'checkFailed', reason: 'the release answer is larger than 1 MB' })
  })

  it('a first address that is not allowed is refused before any request', async () => {
    const hop = vi.fn(fetchHop)
    const r = rig({ hop, feed: { ...feed, api: 'http://127.0.0.1:1/repos/x' } })
    await r.u.check(true)
    expect(r.u.state()).toEqual({ kind: 'checkFailed', reason: 'the address is not allowed' })
    expect(hop).not.toHaveBeenCalled()
  })

  it('a listener that throws does not leave the updater stuck', async () => {
    serveGood()
    const r = rig({
      changed: () => {
        throw new Error('listener bug')
      }
    })
    await r.u.check(false)
    expect(r.u.state().kind).toBe('available')
    await r.u.check(true)
    expect(r.u.state().kind).toBe('available')
    expect(r.logs.join('\n')).toContain('listener bug')
  })
})

describe('abortable', () => {
  it('removes its abort listener when the promise settles', async () => {
    const added: unknown[] = []
    const removed: unknown[] = []
    const signal = {
      aborted: false,
      addEventListener: (_t: string, f: unknown) => added.push(f),
      removeEventListener: (_t: string, f: unknown) => removed.push(f)
    } as unknown as AbortSignal
    for (let i = 0; i < 50; i++) await abortable(Promise.resolve(new Uint8Array(10)), signal)
    await expect(abortable(Promise.reject(new Error('x')), signal)).rejects.toThrow('x')
    expect(added).toHaveLength(51)
    expect(removed).toEqual(added)
  })

  it('answers aborted when the signal fires first', async () => {
    const c = new AbortController()
    const p = abortable(new Promise<number>(() => undefined), c.signal)
    c.abort()
    await expect(p).resolves.toBe('aborted')
    await expect(abortable(Promise.resolve(1), c.signal)).resolves.toBe('aborted')
  })
})

describe('update details', () => {
  const ghAnswers = (body: HopResponse): Record<string, HopResponse> => ({
    [GH_API]: ghApi(),
    [`${GH_ROOT}/${NAME}.sha256`]: ok(GOOD_SHA),
    [`${GH_ROOT}/${NAME}`]: body
  })
  const chunks = (clock: ReturnType<typeof fakeClock> | null): AsyncIterable<Uint8Array> =>
    (async function* () {
      for (let i = 0; i < 10; i++) {
        if (clock) clock.advance(250)
        yield PAYLOAD.subarray(i * 100, (i + 1) * 100)
      }
    })()
  const percents = (r: Rig): number[] =>
    r.states.filter((x) => x.kind === 'downloading').map((x) => (x as { percent: number }).percent)

  it('a checksum mismatch says so in English', async () => {
    serveGood()
    routes[SHA] = text(`${'0'.repeat(64)}  ${NAME}\n`)
    const r = rig()
    await r.u.check(false)
    await r.u.update()
    expect(r.u.state()).toMatchObject({ reason: 'the file does not match its checksum' })
  })

  it('a content-length above the announced size is refused before reading', async () => {
    let read = false
    let returned = false
    const body: AsyncIterable<Uint8Array> = {
      [Symbol.asyncIterator]: () => ({
        next: () => {
          read = true
          return Promise.resolve({ done: true, value: undefined })
        },
        return: () => {
          returned = true
          return Promise.resolve({ done: true, value: undefined })
        }
      })
    }
    const s = scripted(ghAnswers({ status: 200, location: null, body, contentLength: 5000 }))
    const r = rig({ feed: GH, hop: s.hop })
    await r.u.check(false)
    await r.u.update()
    expect(r.u.state()).toMatchObject({ kind: 'downloadFailed', reason: 'the file is larger than the release says' })
    expect(read).toBe(false)
    expect(returned).toBe(true)
  })

  it('progress changes at most once per 250 ms, and 100% always shows', async () => {
    const s = scripted(ghAnswers({ status: 200, location: null, body: chunks(null), contentLength: null }))
    const r = rig({ feed: GH, hop: s.hop })
    await r.u.check(false)
    await r.u.update()
    expect(percents(r)).toEqual([0, 10, 100])
  })

  it('progress follows every whole percent when 250 ms pass', async () => {
    let clock: ReturnType<typeof fakeClock> | null = null
    const answers = scripted(ghAnswers(ok('')))
    const r = rig({
      feed: GH,
      hop: async (url, sig) =>
        url.endsWith('.exe') ? { status: 200, location: null, body: chunks(clock), contentLength: null } : answers.hop(url, sig)
    })
    clock = r.clock
    await r.u.check(false)
    await r.u.update()
    expect(percents(r)).toEqual([0, 10, 20, 30, 40, 50, 60, 70, 80, 90, 100])
  })

  it('stop() before the installer starts prevents runInstaller', async () => {
    serveGood()
    let entered!: () => void
    const inSet = new Promise<void>((res) => (entered = res))
    let release!: () => void
    const gate = new Promise<void>((res) => (release = res))
    const r = rig({
      setHook: async () => {
        entered()
        await gate
      }
    })
    await r.u.check(false)
    const p = r.u.update()
    await inSet
    r.u.stop()
    release()
    await p
    expect(r.runInstaller).not.toHaveBeenCalled()
    expect(r.settings.pendingUpdate).toBeNull()
  })
})

describe('update', () => {
  it('downloads, verifies and starts the installer', async () => {
    serveGood()
    const r = rig()
    await r.u.check(false)
    await r.u.update()
    expect(r.u.state()).toMatchObject({ kind: 'launching', version: '9.9.9' })
    expect(r.runInstaller).toHaveBeenCalledWith(join(r.tempDir, NAME))
    expect(r.settings.pendingUpdate).toBe('9.9.9')
    expect(await files(r.tempDir)).toEqual([NAME])
    expect(r.states.some((s) => s.kind === 'downloading' && s.percent === 100)).toBe(true)
    expect(r.states.some((s) => s.kind === 'verifying')).toBe(true)
  })

  it('a checksum mismatch fails and removes the file', async () => {
    serveGood()
    routes[SHA] = text(`${'0'.repeat(64)}  ${NAME}\n`)
    const r = rig()
    await r.u.check(false)
    await r.u.update()
    expect(r.u.state()).toMatchObject({ kind: 'downloadFailed', version: '9.9.9' })
    expect(await files(r.tempDir)).toEqual([])
    expect(r.runInstaller).not.toHaveBeenCalled()
    expect(r.settings.pendingUpdate).toBeNull()
  })

  it('a bad or oversized checksum file fails', async () => {
    serveGood()
    routes[SHA] = text('x'.repeat(2000))
    const r = rig()
    await r.u.check(false)
    await r.u.update()
    expect(r.u.state().kind).toBe('downloadFailed')
    routes[SHA] = text('garbage\n')
    await r.u.update()
    expect(r.u.state()).toMatchObject({ kind: 'downloadFailed', reason: 'the checksum file is not valid' })
  })

  it('an oversized download fails', async () => {
    serveGood()
    routes[EXE] = (_q, res) => {
      res.writeHead(200) // no content-length: the bytes are counted
      res.write(PAYLOAD)
      res.end(PAYLOAD)
    }
    const r = rig()
    await r.u.check(false)
    await r.u.update()
    expect(r.u.state()).toMatchObject({ kind: 'downloadFailed', reason: 'the file is larger than the release says' })
    expect(await files(r.tempDir)).toEqual([])
    expect(r.runInstaller).not.toHaveBeenCalled()
  })

  it('a stalled download times out', async () => {
    serveGood()
    let stalled!: () => void
    const reached = new Promise<void>((res) => (stalled = res))
    const r = rig({
      hop: async (url, signal) => {
        if (!url.endsWith('.exe')) return fetchHop(url, signal)
        stalled()
        return { status: 200, location: null, contentLength: null, body: { [Symbol.asyncIterator]: () => ({ next: () => new Promise<never>(() => undefined) }) } }
      }
    })
    await r.u.check(false)
    const p = r.u.update()
    await reached
    await flush()
    r.clock.advance(59_000)
    await flush()
    expect(r.u.state().kind).toBe('downloading')
    r.clock.advance(1_000)
    await p
    expect(r.u.state()).toMatchObject({ kind: 'downloadFailed', reason: 'no data for 60 seconds' })
    expect(await files(r.tempDir)).toEqual([])
  })

  it('refuses a redirect to another host', async () => {
    const s = scripted({
      [GH_API]: ghApi(),
      [`${GH_ROOT}/${NAME}.sha256`]: ok(GOOD_SHA),
      [`${GH_ROOT}/${NAME}`]: redirect('https://evil.example/file.exe')
    })
    const r = rig({ feed: GH, hop: s.hop })
    await r.u.check(false)
    await r.u.update()
    expect(r.u.state()).toMatchObject({ kind: 'downloadFailed', reason: 'redirect to a host that is not allowed' })
    expect(s.urls.some((u) => u.includes('evil'))).toBe(false)
    expect(await files(r.tempDir)).toEqual([])
    expect(r.runInstaller).not.toHaveBeenCalled()
  })

  it('refuses a redirect to http', async () => {
    const s = scripted({
      [GH_API]: ghApi(),
      [`${GH_ROOT}/${NAME}.sha256`]: ok(GOOD_SHA),
      [`${GH_ROOT}/${NAME}`]: redirect('http://objects.githubusercontent.com/file.exe')
    })
    const r = rig({ feed: GH, hop: s.hop })
    await r.u.check(false)
    await r.u.update()
    expect(r.u.state()).toMatchObject({ kind: 'downloadFailed', reason: 'redirect to a host that is not allowed' })
    expect(s.urls).not.toContain('http://objects.githubusercontent.com/file.exe')
    expect(await files(r.tempDir)).toEqual([])
  })

  it('follows an allowed redirect', async () => {
    const s = scripted({
      [GH_API]: ghApi(),
      [`${GH_ROOT}/${NAME}.sha256`]: ok(GOOD_SHA),
      [`${GH_ROOT}/${NAME}`]: redirect('/splitlane-app/splitlane/releases/download/v9.9.9/mirror', 301),
      [`${GH_ROOT}/mirror`]: redirect('https://release-assets.githubusercontent.com/blob', 307),
      'https://release-assets.githubusercontent.com/blob': ok(PAYLOAD)
    })
    const r = rig({ feed: GH, hop: s.hop })
    await r.u.check(false)
    await r.u.update()
    expect(r.u.state()).toMatchObject({ kind: 'launching' })
    expect(s.urls).toContain('https://release-assets.githubusercontent.com/blob')
  })

  it('too many redirects fail', async () => {
    const answers: Record<string, HopResponse[]> = { [GH_API]: [ghApi()], [`${GH_ROOT}/${NAME}.sha256`]: [ok(GOOD_SHA)] }
    for (let i = 0; i < 10; i++) answers[i === 0 ? `${GH_ROOT}/${NAME}` : `https://github.com/r${i}`] = [redirect(`https://github.com/r${i + 1}`)]
    const s = scripted(answers)
    const r = rig({ feed: GH, hop: s.hop })
    await r.u.check(false)
    await r.u.update()
    expect(r.u.state()).toMatchObject({ kind: 'downloadFailed', reason: 'too many redirects' })
    expect(s.urls.slice(s.urls.indexOf(`${GH_ROOT}/${NAME}`))).toHaveLength(5)
  })

  it('cancel stops the download', async () => {
    serveGood()
    routes[EXE] = (_q, res) => {
      res.writeHead(200, { 'content-length': String(PAYLOAD.length) })
      res.write(PAYLOAD.subarray(0, 10))
      // never finished
    }
    const r = rig()
    await r.u.check(false)
    const p = r.u.update()
    await vi.waitFor(() => expect(hits).toContain(EXE))
    await flush()
    r.u.cancel()
    await p
    expect(r.u.state()).toMatchObject({ kind: 'available', version: '9.9.9', hidden: false })
    expect(await files(r.tempDir)).toEqual([])
    expect(r.runInstaller).not.toHaveBeenCalled()
    // can be started again
    routes[EXE] = (_q, res) => void res.writeHead(200).end(PAYLOAD)
    await r.u.update()
    expect(r.u.state().kind).toBe('launching')
  })

  it('a second update while downloading is ignored', async () => {
    serveGood()
    let release!: () => void
    const gate = new Promise<void>((res) => (release = res))
    routes[EXE] = async (_q, res) => {
      res.writeHead(200, { 'content-length': String(PAYLOAD.length) })
      res.write(PAYLOAD.subarray(0, 10))
      await gate
      res.end(PAYLOAD.subarray(10))
    }
    const r = rig()
    await r.u.check(false)
    const p = r.u.update()
    await vi.waitFor(() => expect(hits).toContain(EXE))
    await r.u.update()
    release()
    await p
    expect(hits.filter((h) => h === EXE)).toHaveLength(1)
    expect(r.runInstaller).toHaveBeenCalledTimes(1)
  })

  it('a check during a download is ignored', async () => {
    serveGood()
    let release!: () => void
    const gate = new Promise<void>((res) => (release = res))
    routes[EXE] = async (_q, res) => {
      res.writeHead(200, { 'content-length': String(PAYLOAD.length) })
      res.write(PAYLOAD.subarray(0, 10))
      await gate
      res.end(PAYLOAD.subarray(10))
    }
    const r = rig()
    await r.u.check(false)
    const p = r.u.update()
    await vi.waitFor(() => expect(hits).toContain(EXE))
    const apiHits = hits.filter((h) => h === API).length
    await r.u.check(true)
    await r.u.check(false)
    expect(hits.filter((h) => h === API).length).toBe(apiHits)
    expect(r.u.state().kind).toBe('downloading')
    release()
    await p
    expect(r.u.state().kind).toBe('launching')
  })

  it('a write error fails the download', async () => {
    serveGood()
    const real = nodeFs()
    const r = rig({
      fs: {
        ...real,
        open: async (p) => {
          const h = await real.open(p)
          return {
            write: () => Promise.reject(new Error('ENOSPC: no space left on device')),
            close: () => h.close()
          }
        }
      }
    })
    await r.u.check(false)
    await r.u.update()
    expect(r.u.state()).toMatchObject({ kind: 'downloadFailed', version: '9.9.9' })
    expect((r.u.state() as { reason: string }).reason).toContain('ENOSPC')
    expect(r.runInstaller).not.toHaveBeenCalled()
    expect(await files(r.tempDir)).toEqual([])
  })

  it('a failure to remove the partial file is only logged', async () => {
    serveGood()
    const real = nodeFs()
    const r = rig({
      fs: { ...real, unlink: () => Promise.reject(new Error('EBUSY')) }
    })
    routes[SHA] = text(`${'0'.repeat(64)}  ${NAME}\n`)
    await r.u.check(false)
    await r.u.update()
    expect(r.u.state().kind).toBe('downloadFailed')
    expect(r.logs.join('\n')).toContain('EBUSY')
  })

  it('cancelled UAC clears pendingUpdate', async () => {
    serveGood()
    const r = rig({ installer: 'cancelled' })
    await r.u.check(false)
    await r.u.update()
    expect(r.u.state()).toMatchObject({ kind: 'cancelled', version: '9.9.9' })
    expect(r.sets).toEqual([{ pendingUpdate: '9.9.9' }, { pendingUpdate: null }])
    expect(r.settings.pendingUpdate).toBeNull()
    // retry from the cancelled state
    r.runInstaller.mockResolvedValueOnce({ error: 'cannot start' })
    await r.u.update()
    expect(r.u.state()).toMatchObject({ kind: 'downloadFailed', reason: 'cannot start' })
    expect(r.settings.pendingUpdate).toBeNull()
  })
})

describe('start', () => {
  it('start reports installed and clears pendingUpdate', async () => {
    const r = rig({ appVersion: '1.1.0', settings: { pendingUpdate: '1.1.0' } })
    await r.u.start()
    r.u.stop()
    expect(r.u.state()).toEqual({ kind: 'installed', version: '1.1.0' })
    expect(r.settings.pendingUpdate).toBeNull()
    expect(r.states).toEqual([{ kind: 'installed', version: '1.1.0' }])
  })

  it('start reports a refusal with its reason', async () => {
    const r = rig({ settings: { pendingUpdate: '1.2.0' } })
    await mkdir(root, { recursive: true })
    await writeFile(r.lastInstallFile, '1.2.0\r\nrefused\r\nthe tunnel is on\r\n', 'utf8')
    await r.u.start()
    r.u.stop()
    expect(r.u.state()).toEqual({
      kind: 'installRefused',
      version: '1.2.0',
      notesUrl: 'https://github.com/splitlane-app/splitlane/releases/tag/v1.2.0',
      reason: 'the tunnel is on'
    })
    expect(r.settings.pendingUpdate).toBeNull()
  })

  it('start without a pending update leaves the state idle', async () => {
    const r = rig()
    await r.u.start()
    r.u.stop()
    expect(r.u.state()).toEqual({ kind: 'idle' })
    expect(r.states).toEqual([])
  })

  it('an installer refused after a restart can be retried', async () => {
    serveGood()
    const r = rig({ settings: { pendingUpdate: '9.9.9' } })
    await r.u.start()
    expect(r.u.state().kind).toBe('installRefused')
    await r.u.update()
    expect(r.u.state()).toMatchObject({ kind: 'launching', version: '9.9.9' })
    r.u.stop()
  })

  it('schedule: first check after 30 s, then every 24 h, none when checkUpdates is off', async () => {
    routes[API] = json(releaseJson(feed, '1.0.0'))
    const apiHits = (): number => hits.filter((h) => h === API).length
    const r = rig()
    await r.u.start()
    r.clock.advance(29_999)
    await flush()
    expect(apiHits()).toBe(0)
    r.clock.advance(1)
    await vi.waitFor(() => expect(apiHits()).toBe(1))
    r.clock.advance(24 * 3600 * 1000 - 1)
    await flush()
    expect(apiHits()).toBe(1)
    r.clock.advance(1)
    await vi.waitFor(() => expect(apiHits()).toBe(2))
    r.u.stop()
    r.clock.advance(48 * 3600 * 1000)
    await flush()
    expect(apiHits()).toBe(2)

    hits = []
    const off = rig({ settings: { checkUpdates: false } })
    await off.u.start()
    off.clock.advance(72 * 3600 * 1000)
    await flush()
    expect(apiHits()).toBe(0)
    // a manual check still works
    await off.u.check(true)
    expect(apiHits()).toBe(1)
    off.u.stop()
  })
})

describe('runInstallerScript', () => {
  it('runInstallerScript quotes the path and maps a cancelled UAC to 1223', () => {
    const s = runInstallerScript('C:\\Temp\\SplitLane-update\\SplitLane-Setup-1.2.3.exe')
    expect(s).toContain("-FilePath 'C:\\Temp\\SplitLane-update\\SplitLane-Setup-1.2.3.exe'")
    expect(s).toContain("-ArgumentList '/S','--force-run' -Verb RunAs -ErrorAction Stop")
    expect(s).not.toContain('-Wait')
    expect(s).toContain('exit 1223')
    expect(s).toMatch(/exit 0\r\n$/)
    expect(runInstallerScript("C:\\O'Brien\\a.exe")).toContain("-FilePath 'C:\\O''Brien\\a.exe'")
    expect(runInstallerScript('C:\\O\u2019Brien\\a.exe')).toContain("-FilePath 'C:\\O\u2019\u2019Brien\\a.exe'")
    expect(() => runInstallerScript('C:\\a\u0001b.exe')).toThrow()
    expect(() => runInstallerScript('C:\\a"b.exe')).toThrow()
    expect(() => runInstallerScript('relative\\a.exe')).toThrow()
    expect(() => runInstallerScript('C:\\%TEMP%\\a.exe')).toThrow()
  })
})
