/**
 * The window's update model (stage 2 part 2): pure functions with no electron, fs or network.
 * The main process fetches and installs; this module decides what is acceptable.
 */

export type Version = readonly [number, number, number]

const REPO = 'splitlane-app/splitlane'
const GITHUB_HOSTS: readonly string[] = [
  'api.github.com',
  'github.com',
  'objects.githubusercontent.com',
  'release-assets.githubusercontent.com'
]
const MAX_INSTALLER_BYTES = 400 * 1024 * 1024

/** Exactly x.y.z: unsigned integers, no leading zeros except "0" itself. */
export function parseVersion(s: string): Version | null {
  const m = /^(0|[1-9]\d{0,8})\.(0|[1-9]\d{0,8})\.(0|[1-9]\d{0,8})$/.exec(s)
  if (!m) return null
  return [Number(m[1]), Number(m[2]), Number(m[3])]
}

export function compareVersions(a: Version, b: Version): number {
  for (let i = 0; i < 3; i++) {
    if (a[i] !== b[i]) return a[i] - b[i]
  }
  return 0
}

/** True only when both are x.y.z and `version` is greater than `than`. */
export function isNewer(version: string, than: string): boolean {
  const v = parseVersion(version)
  const t = parseVersion(than)
  return v !== null && t !== null && compareVersions(v, t) > 0
}

export interface Feed {
  /** Full address of releases/latest. */
  api: string
  /** Expected start of asset addresses: <origin>/splitlane-app/splitlane/releases/download/ */
  assetPrefix: string
  origin: 'github' | 'test'
  /** http://127.0.0.1:<port>, only for 'test'. */
  testOrigin?: string
}

const GITHUB_FEED: Feed = {
  api: `https://api.github.com/repos/${REPO}/releases/latest`,
  assetPrefix: `https://github.com/${REPO}/releases/download/`,
  origin: 'github'
}

/** `refused` is the log text when the variable is set but not an allowed test origin. */
export function feedFrom(env: string | undefined): { feed: Feed; refused?: string } {
  if (env === undefined || env === '') return { feed: { ...GITHUB_FEED } }
  const m = /^http:\/\/127\.0\.0\.1:(\d{1,5})\/?$/.exec(env)
  const port = m ? Number(m[1]) : 0
  if (!m || port < 1 || port > 65535 || port === 80) {
    return {
      feed: { ...GITHUB_FEED },
      refused: 'SPLITLANE_UPDATE_FEED ignored: only http://127.0.0.1:<port> is allowed'
    }
  }
  const origin = new URL(`http://127.0.0.1:${port}`).origin
  return {
    feed: {
      api: `${origin}/repos/${REPO}/releases/latest`,
      assetPrefix: `${origin}/${REPO}/releases/download/`,
      origin: 'test',
      testOrigin: origin
    }
  }
}

export function notesUrlFor(version: string): string {
  return `https://github.com/${REPO}/releases/tag/v${version}`
}

export type ReleasePick =
  | { kind: 'newer'; version: string; installerUrl: string; shaUrl: string; notesUrl: string; name: string }
  | { kind: 'notNewer'; version: string }
  | { kind: 'invalid'; reason: string }

function isRecord(x: unknown): x is Record<string, unknown> {
  return typeof x === 'object' && x !== null && !Array.isArray(x)
}

export function pickRelease(json: unknown, current: string, feed: Feed): ReleasePick {
  const bad = (reason: string): ReleasePick => ({ kind: 'invalid', reason })
  if (!isRecord(json)) return bad('release is not an object')
  if (json.draft === true || json.prerelease === true) return bad('release is a draft or a prerelease')
  const tag = json.tag_name
  if (typeof tag !== 'string' || !tag.startsWith('v')) return bad('bad release tag')
  const latest = parseVersion(tag.slice(1))
  if (!latest) return bad('bad release tag')
  const cur = parseVersion(current)
  if (!cur) return bad('bad current version')
  const version = tag.slice(1)
  if (compareVersions(latest, cur) <= 0) return { kind: 'notNewer', version }

  const assets = Array.isArray(json.assets) ? (json.assets as unknown[]) : []
  const name = `SplitLane-Setup-${version}.exe`
  const find = (n: string): Record<string, unknown> | string => {
    const hits = assets.filter((a): a is Record<string, unknown> => isRecord(a) && a.name === n)
    if (hits.length !== 1) return `release must have exactly one file ${n}`
    return hits[0]
  }
  const exe = find(name)
  if (typeof exe === 'string') return bad(exe)
  const sha = find(`${name}.sha256`)
  if (typeof sha === 'string') return bad(sha)

  const installerUrl = `${feed.assetPrefix}v${version}/${name}`
  const shaUrl = `${installerUrl}.sha256`
  if (exe.browser_download_url !== installerUrl || sha.browser_download_url !== shaUrl) {
    return bad('release file address is not the expected one')
  }
  const size = exe.size
  if (typeof size !== 'number' || !Number.isFinite(size) || !Number.isInteger(size) || size < 1 || size > MAX_INSTALLER_BYTES) {
    return bad('installer size is out of range')
  }
  return { kind: 'newer', version, installerUrl, shaUrl, notesUrl: notesUrlFor(version), name }
}

/** May a request or redirect go to this address? */
export function allowedHop(url: string, feed: Feed): boolean {
  let u: URL
  try {
    u = new URL(url)
  } catch {
    return false
  }
  if (feed.origin === 'test') return feed.testOrigin !== undefined && u.origin === feed.testOrigin
  return (
    u.protocol === 'https:' &&
    GITHUB_HOSTS.includes(u.hostname) &&
    u.port === '' &&
    u.username === '' &&
    u.password === ''
  )
}

/** First non-empty line: "<64 hex><1-2 spaces or a tab><name>"; returns the lowercase hash. */
export function parseSha256File(text: string, fileName: string): string | null {
  const line = text
    .replace(/^﻿/, '')
    .split(/\r?\n/)
    .find((l) => l.trim() !== '')
    ?.trimEnd()
  if (line === undefined) return null
  const m = /^([0-9a-fA-F]{64})(?: {1,2}|\t)(.+)$/.exec(line)
  if (!m || m[2] !== fileName) return null
  return m[1].toLowerCase()
}

export interface LastInstall {
  version: string
  result: 'ok' | 'refused'
  reason: string
}

/** last-install.txt: version, ok|refused, optional reason. */
export function parseLastInstall(text: string): LastInstall | null {
  const lines = text.replace(/^﻿/, '').split(/\r?\n/)
  if (parseVersion(lines[0] ?? '') === null) return null
  const result = lines[1]
  if (result !== 'ok' && result !== 'refused') return null
  return { version: lines[0], result, reason: (lines[2] ?? '').slice(0, 1000) }
}

export type UpdateState =
  | { kind: 'idle' }
  | { kind: 'checking'; manual: boolean }
  | { kind: 'upToDate' } // only after a manual check
  | { kind: 'checkFailed'; reason: string } // only after a manual check
  | { kind: 'available'; version: string; notesUrl: string; hidden: boolean }
  | { kind: 'downloading'; version: string; notesUrl: string; percent: number }
  | { kind: 'verifying'; version: string; notesUrl: string }
  | { kind: 'launching'; version: string; notesUrl: string }
  | { kind: 'cancelled'; version: string; notesUrl: string }
  | { kind: 'downloadFailed'; version: string; notesUrl: string; reason: string }
  | { kind: 'installed'; version: string }
  // reason may be ''; canRetry: the version is newer than the window's, so a retry finds it to install again
  | { kind: 'installRefused'; version: string; notesUrl: string; reason: string; canRetry: boolean }

/**
 * The outcome of an install started before this window started; the result file counts only for the same version.
 * A refusal of the pending version wins over the window's version: when `install` refuses, the files of that
 * version are already in place, so the window runs it although the setup failed.
 */
export function outcomeAfterRestart(
  pending: string | null,
  appVersion: string,
  last: LastInstall | null
): UpdateState | null {
  if (pending === null) return null
  const notesUrl = notesUrlFor(pending)
  const canRetry = isNewer(pending, appVersion)
  if (last !== null && last.version === pending && last.result === 'refused') {
    return { kind: 'installRefused', version: pending, notesUrl, reason: last.reason, canRetry }
  }
  const p = parseVersion(pending)
  const a = parseVersion(appVersion)
  if (pending === appVersion || (p !== null && a !== null && compareVersions(a, p) >= 0)) {
    return { kind: 'installed', version: appVersion }
  }
  return { kind: 'installRefused', version: pending, notesUrl, reason: '', canRetry }
}
