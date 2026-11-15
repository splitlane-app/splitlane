import { createHash } from 'node:crypto'
import { open, mkdir, readFile, rename, rm, unlink } from 'node:fs/promises'
import { join } from 'node:path'
import type { WindowSettings } from '../shared/settings'
import {
  allowedHop,
  notesUrlFor,
  outcomeAfterRestart,
  parseLastInstall,
  parseSha256File,
  pickRelease,
  type Feed,
  type UpdateState
} from '../shared/update'
import { checkPath, psSingleQuoted } from './setup'

/** One HTTP response; redirects are not followed by the transport. */
export interface HopResponse {
  status: number
  location: string | null
  body: AsyncIterable<Uint8Array> | null
  contentLength: number | null
}

export interface UpdaterDeps {
  appVersion: string
  feed: Feed
  /** canInstall of part 1, or a test feed in development. */
  canUpdate: boolean
  /** One request WITHOUT following redirects. */
  hop(url: string, signal: AbortSignal): Promise<HopResponse>
  /** <TEMP>\SplitLane-update */
  tempDir: string
  fs: {
    rmDir(p: string): Promise<void>
    mkdir(p: string): Promise<void>
    open(p: string): Promise<{ write(b: Uint8Array): Promise<void>; close(): Promise<void> }>
    rename(a: string, b: string): Promise<void>
    unlink(p: string): Promise<void>
    readText(p: string): Promise<string | null>
  }
  runInstaller(exe: string): Promise<'started' | 'cancelled' | { error: string }>
  settings: { get(): WindowSettings; set(patch: Partial<WindowSettings>): Promise<void> }
  /** %ProgramData%\SplitLane\export\last-install.txt */
  lastInstallFile: string
  clock: { now(): number; setTimeout(fn: () => void, ms: number): unknown; clearTimeout(h: unknown): void }
  log(line: string): void
  changed(state: UpdateState): void
}

const API_LIMIT = 1024 * 1024
const SHA_LIMIT = 1024
const INSTALLER_LIMIT = 400 * 1024 * 1024
const MAX_STEPS = 5
const STALL_MS = 60_000
const FIRST_CHECK_MS = 30_000
const CHECK_EVERY_MS = 24 * 60 * 60 * 1000
const PROGRESS_EVERY_MS = 250
const UAC_CANCELLED = 1223

const AUTO_CHECK_STATES: readonly UpdateState['kind'][] = ['idle', 'upToDate', 'checkFailed', 'available']

const NOT_INSTALLED = 'updates are available only for an installed SplitLane'

type Newer = Extract<ReturnType<typeof pickRelease>, { kind: 'newer' }> & { size: number }
type Pick = Exclude<ReturnType<typeof pickRelease>, { kind: 'newer' }> | Newer

class Cancelled extends Error {
  constructor() {
    super('cancelled')
  }
}

function errText(e: unknown): string {
  return e instanceof Error ? e.message : String(e)
}

/** The filesystem operations of the updater on node:fs. */
export function nodeFs(): UpdaterDeps['fs'] {
  return {
    rmDir: (p) => rm(p, { recursive: true, force: true }),
    mkdir: async (p) => {
      await mkdir(p, { recursive: true })
    },
    open: async (p) => {
      const fh = await open(p, 'w')
      return {
        write: async (b) => {
          let off = 0
          while (off < b.length) {
            const { bytesWritten } = await fh.write(b, off, b.length - off)
            off += bytesWritten
          }
        },
        close: () => fh.close()
      }
    },
    rename: (a, b) => rename(a, b),
    unlink: (p) => unlink(p),
    readText: async (p) => {
      try {
        return await readFile(p, 'utf8')
      } catch {
        return null
      }
    }
  }
}

/** PowerShell text that starts the installer elevated without waiting; a cancelled UAC prompt exits 1223. */
export function runInstallerScript(exe: string): string {
  checkPath(exe)
  return [
    'try {',
    `  Start-Process -FilePath ${psSingleQuoted(exe)} -ArgumentList '/S','--force-run' -Verb RunAs -ErrorAction Stop`,
    '} catch {',
    '  $e = $_.Exception',
    '  while ($null -ne $e) {',
    `    if (($e -is [System.ComponentModel.Win32Exception]) -and ($e.NativeErrorCode -eq ${UAC_CANCELLED})) { exit ${UAC_CANCELLED} }`,
    '    $e = $e.InnerException',
    '  }',
    '  [Console]::Error.WriteLine($_.Exception.Message)',
    '  exit 1',
    '}',
    'exit 0',
    ''
  ].join('\r\n')
}

/** powershell.exe's exit code of runInstallerScript as the result of runInstaller. */
export function installerOutcome(code: number): 'started' | 'cancelled' | { error: string } {
  if (code === 0) return 'started'
  if (code === UAC_CANCELLED) return 'cancelled'
  return { error: `powershell exit ${code}` }
}

export class Updater {
  private s: UpdateState = { kind: 'idle' }
  private busy = false
  private ctl: AbortController | null = null
  private stalled = false
  private cancelling = false
  private stopped = false
  private timer: unknown = null
  private stallTimer: unknown = null
  private pick: Newer | null = null
  private checkRun: Promise<void> | null = null
  /** Whether the running check reports as a manual one (an automatic check turns manual on a press). */
  private checkManual = false

  constructor(private readonly d: UpdaterDeps) {}

  state(): UpdateState {
    return this.s
  }

  async start(): Promise<void> {
    const pending = this.d.settings.get().pendingUpdate
    if (pending !== null) {
      let last = null
      try {
        last = parseLastInstall((await this.d.fs.readText(this.d.lastInstallFile)) ?? '')
      } catch (e) {
        this.d.log(`cannot read the install result: ${errText(e)}`)
      }
      const outcome = outcomeAfterRestart(pending, this.d.appVersion, last)
      if (outcome !== null) {
        this.set(outcome)
        try {
          await this.d.settings.set({ pendingUpdate: null })
        } catch (e) {
          this.d.log(`cannot clear pendingUpdate: ${errText(e)}`)
        }
      }
    }
    if (this.stopped) return
    this.timer = this.d.clock.setTimeout(() => this.tick(), FIRST_CHECK_MS)
  }

  stop(): void {
    this.stopped = true
    if (this.timer !== null) this.d.clock.clearTimeout(this.timer)
    this.timer = null
    this.disarm()
    this.ctl?.abort()
  }

  check(manual: boolean): Promise<void> {
    if (this.checkRun !== null) {
      // a manual press during a silent automatic check: that check becomes the manual one
      if (manual && !this.checkManual) {
        this.checkManual = true
        this.set({ kind: 'checking', manual: true })
      }
      return this.checkRun
    }
    if (this.busy || this.inProgress()) return Promise.resolve()
    const run = this.runCheck(manual).finally(() => {
      this.checkRun = null
    })
    this.checkRun = run
    return run
  }

  private async runCheck(manualAtStart: boolean): Promise<void> {
    const manual = manualAtStart
    this.checkManual = manual
    if (!this.d.canUpdate) {
      if (manual) this.set({ kind: 'checkFailed', reason: NOT_INSTALLED })
      return
    }
    if (!manual && !AUTO_CHECK_STATES.includes(this.s.kind)) {
      this.d.log(`automatic update check skipped: state is ${this.s.kind}`)
      return
    }
    this.begin()
    // An automatic check is silent: no `checking` state, it never disturbs what is on screen.
    if (manual) this.set({ kind: 'checking', manual })
    try {
      const pick = await this.fetchPick()
      if (pick.kind === 'newer') {
        this.pick = pick
        const cur = this.s
        if (this.checkManual || !(cur.kind === 'available' && cur.version === pick.version)) {
          this.set({ kind: 'available', version: pick.version, notesUrl: pick.notesUrl, hidden: false })
        }
      } else if (pick.kind === 'notNewer') {
        if (this.checkManual) this.set({ kind: 'upToDate' })
      } else {
        throw new Error(pick.reason)
      }
    } catch (e) {
      const reason = errText(e)
      if (this.checkManual) {
        this.set({ kind: 'checkFailed', reason })
      } else {
        this.d.log(`update check failed: ${reason}`)
      }
    } finally {
      this.busy = false
      this.ctl = null
    }
  }

  async update(): Promise<void> {
    const s = this.s
    if (this.busy) return
    if (s.kind !== 'available' && s.kind !== 'cancelled' && s.kind !== 'downloadFailed' && s.kind !== 'installRefused') return
    this.begin()
    let version = s.version
    let notesUrl = s.notesUrl
    this.set({ kind: 'downloading', version, notesUrl, percent: 0 })
    let part: string | null = null
    let handle: { write(b: Uint8Array): Promise<void>; close(): Promise<void> } | null = null
    try {
      let pick = this.pick !== null && this.pick.version === version ? this.pick : null
      if (pick === null) {
        const fresh = await this.fetchPick()
        if (fresh.kind !== 'newer') throw new Error('the release is not available')
        pick = fresh
        this.pick = fresh
        version = fresh.version
        notesUrl = fresh.notesUrl
        this.set({ kind: 'downloading', version, notesUrl, percent: 0 })
      }
      const release = pick
      const fs = this.d.fs
      await fs.rmDir(this.d.tempDir)
      await fs.mkdir(this.d.tempDir)

      const shaText = await this.fetchText(release.shaUrl, SHA_LIMIT, 'the checksum file is larger than 1 KB')
      const expected = parseSha256File(shaText, release.name)
      if (expected === null) throw new Error('the checksum file is not valid')

      const finalPath = join(this.d.tempDir, release.name)
      part = `${finalPath}.part`
      handle = await fs.open(part)
      const out = handle
      const hash = createHash('sha256')
      let received = 0
      let lastPercent = 0
      let lastEmit = Number.NEGATIVE_INFINITY
      const announced = release.size <= INSTALLER_LIMIT
      await this.request(release.installerUrl, Math.min(INSTALLER_LIMIT, release.size), announced ? 'the file is larger than the release says' : 'the file is larger than 400 MB', async (chunk) => {
        hash.update(chunk)
        await out.write(chunk)
        received += chunk.length
        const percent = Math.min(100, Math.floor((received * 100) / release.size))
        const now = this.d.clock.now()
        if (percent !== lastPercent && (percent === 100 || now - lastEmit >= PROGRESS_EVERY_MS)) {
          lastPercent = percent
          lastEmit = now
          this.set({ kind: 'downloading', version, notesUrl, percent })
        }
      })
      handle = null
      await out.close()
      if (this.ctl?.signal.aborted) throw new Cancelled()

      this.set({ kind: 'verifying', version, notesUrl })
      if (hash.digest('hex') !== expected) throw new Error('the file does not match its checksum')
      await fs.rename(part, finalPath)
      part = null

      if (this.stopped) throw new Cancelled()
      await this.d.settings.set({ pendingUpdate: version })
      this.set({ kind: 'launching', version, notesUrl })
      if (this.stopped) {
        await this.clearPending()
        throw new Cancelled()
      }
      let result: Awaited<ReturnType<UpdaterDeps['runInstaller']>>
      try {
        result = await this.d.runInstaller(finalPath)
      } catch (e) {
        result = { error: errText(e) }
      }
      if (result === 'started') return
      await this.clearPending()
      if (result === 'cancelled') this.set({ kind: 'cancelled', version, notesUrl })
      else this.set({ kind: 'downloadFailed', version, notesUrl, reason: result.error })
    } catch (e) {
      if (handle !== null) {
        try {
          await handle.close()
        } catch (ce) {
          this.d.log(`cannot close the partial file: ${errText(ce)}`)
        }
      }
      if (part !== null) {
        try {
          await this.d.fs.unlink(part)
        } catch (ue) {
          this.d.log(`cannot remove the partial file: ${errText(ue)}`)
        }
      }
      if (this.cancelling && !this.stalled) {
        this.set({ kind: 'available', version, notesUrl: notesUrlFor(version), hidden: false })
      } else {
        const reason = errText(e)
        this.d.log(`download of ${version} failed: ${reason}`)
        this.set({ kind: 'downloadFailed', version, notesUrl, reason })
      }
    } finally {
      this.busy = false
      this.ctl = null
    }
  }

  cancel(): void {
    if (this.s.kind !== 'downloading' || !this.busy) return
    this.cancelling = true
    this.ctl?.abort()
  }

  later(): void {
    if (this.s.kind === 'available') this.set({ ...this.s, hidden: true })
  }

  dismiss(): void {
    const k = this.s.kind
    if (k === 'installed' || k === 'upToDate' || k === 'checkFailed') this.set({ kind: 'idle' })
  }

  // --- internals ---

  private begin(): void {
    this.busy = true
    this.cancelling = false
    this.stalled = false
    this.ctl = new AbortController()
  }

  private inProgress(): boolean {
    const k = this.s.kind
    return k === 'downloading' || k === 'verifying' || k === 'launching' || k === 'checking'
  }

  private set(s: UpdateState): void {
    this.s = s
    if (this.stopped) return
    try {
      this.d.changed(s)
    } catch (e) {
      this.d.log(`state listener failed: ${errText(e)}`)
    }
  }

  private tick(): void {
    this.timer = this.d.clock.setTimeout(() => this.tick(), CHECK_EVERY_MS)
    if (this.stopped || !this.d.settings.get().checkUpdates) return
    this.check(false).catch((e) => this.d.log(`update check crashed: ${errText(e)}`))
  }

  private async clearPending(): Promise<void> {
    try {
      await this.d.settings.set({ pendingUpdate: null })
    } catch (e) {
      this.d.log(`cannot clear pendingUpdate: ${errText(e)}`)
    }
  }

  private async fetchPick(): Promise<Pick> {
    const text = await this.fetchText(this.d.feed.api, API_LIMIT, 'the release answer is larger than 1 MB')
    let json: unknown
    try {
      json = JSON.parse(text)
    } catch {
      throw new Error('the release answer is not valid JSON')
    }
    const pick = pickRelease(json, this.d.appVersion, this.d.feed)
    if (pick.kind !== 'newer') return pick
    // pickRelease validated the installer entry: its size is a positive integer.
    const assets = (json as { assets: { name: string; size: number }[] }).assets
    const size = assets.find((a) => a.name === pick.name)?.size ?? 0
    return { ...pick, size }
  }

  private async fetchText(url: string, limit: number, tooLarge: string): Promise<string> {
    const parts: Buffer[] = []
    await this.request(url, limit, tooLarge, async (c) => {
      parts.push(Buffer.from(c))
    })
    return Buffer.concat(parts).toString('utf8')
  }

  private arm(): void {
    this.disarm()
    this.stallTimer = this.d.clock.setTimeout(() => {
      this.stalled = true
      this.ctl?.abort()
    }, STALL_MS)
  }

  private disarm(): void {
    if (this.stallTimer !== null) this.d.clock.clearTimeout(this.stallTimer)
    this.stallTimer = null
  }

  /** GET with manual redirects; the body goes to `onChunk`. Throws on any refusal. */
  private async request(url: string, limit: number, tooLarge: string, onChunk: (c: Uint8Array) => Promise<void>): Promise<void> {
    const signal = (this.ctl ??= new AbortController()).signal
    const fail = (): Error => (this.stalled ? new Error('no data for 60 seconds') : new Cancelled())
    let current = url
    try {
      for (let step = 1; ; step++) {
        if (!allowedHop(current, this.d.feed)) {
          throw new Error(step === 1 ? 'the address is not allowed' : 'redirect to a host that is not allowed')
        }
        this.arm()
        let res: HopResponse
        try {
          res = await abortable(this.d.hop(current, signal), signal)
            .then((r) => {
              if (r === 'aborted') throw fail()
              return r
            })
        } catch (e) {
          if (signal.aborted) throw fail()
          throw e
        }
        if ([301, 302, 303, 307, 308].includes(res.status)) {
          void discard(res.body)
          if (step >= MAX_STEPS) throw new Error('too many redirects')
          if (res.location === null || res.location === '') throw new Error('redirect without a location')
          try {
            current = new URL(res.location, current).toString()
          } catch {
            throw new Error('redirect to a bad address')
          }
          continue
        }
        if (res.status !== 200) {
          void discard(res.body)
          throw new Error(res.status === 403 ? 'HTTP 403 (GitHub request limit?)' : `HTTP ${res.status}`)
        }
        if (res.contentLength !== null && res.contentLength > limit) {
          void discard(res.body)
          throw new Error(tooLarge)
        }
        if (res.body === null) throw new Error('the answer has no body')
        const it = res.body[Symbol.asyncIterator]()
        let total = 0
        for (;;) {
          this.arm()
          let r: IteratorResult<Uint8Array> | 'aborted'
          try {
            r = await abortable(it.next(), signal)
          } catch (e) {
            if (signal.aborted) throw fail()
            throw e
          }
          if (r === 'aborted') {
            void Promise.resolve(it.return?.()).catch(() => undefined)
            throw fail()
          }
          if (r.done) return
          total += r.value.length
          if (total > limit) {
            void Promise.resolve(it.return?.()).catch(() => undefined)
            throw new Error(tooLarge)
          }
          await onChunk(r.value)
        }
      }
    } finally {
      this.disarm()
    }
  }
}

/** Resolves with 'aborted' when the signal fires first; its abort listener is removed as soon as `p` settles. */
export function abortable<T>(p: Promise<T>, signal: AbortSignal): Promise<T | 'aborted'> {
  return new Promise((resolve, reject) => {
    if (signal.aborted) {
      p.catch(() => undefined)
      resolve('aborted')
      return
    }
    const on = (): void => resolve('aborted')
    signal.addEventListener('abort', on, { once: true })
    p.then(
      (v) => {
        signal.removeEventListener('abort', on)
        resolve(v)
      },
      (e: unknown) => {
        signal.removeEventListener('abort', on)
        reject(e)
      }
    )
  })
}

function discard(body: AsyncIterable<Uint8Array> | null): Promise<void> {
  if (body === null) return Promise.resolve()
  try {
    return Promise.resolve(body[Symbol.asyncIterator]().return?.()).then(
      () => undefined,
      () => undefined
    )
  } catch {
    return Promise.resolve()
  }
}
