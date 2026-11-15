import { describe, expect, it } from 'vitest'
import {
  allowedHop,
  compareVersions,
  feedFrom,
  notesUrlFor,
  outcomeAfterRestart,
  parseLastInstall,
  parseSha256File,
  parseVersion,
  pickRelease,
  type Feed
} from './update'

const GH = feedFrom(undefined).feed
const TEST = feedFrom('http://127.0.0.1:8123').feed
const PREFIX = 'https://github.com/splitlane-app/splitlane/releases/download/'
const HEX = 'ab'.repeat(32)

function release(version: string, over: Record<string, unknown> = {}, prefix = PREFIX): Record<string, unknown> {
  const exe = `SplitLane-Setup-${version}.exe`
  return {
    tag_name: `v${version}`,
    draft: false,
    prerelease: false,
    assets: [
      { name: exe, browser_download_url: `${prefix}v${version}/${exe}`, size: 90_000_000 },
      { name: `${exe}.sha256`, browser_download_url: `${prefix}v${version}/${exe}.sha256`, size: 100 },
      { name: 'other.txt', browser_download_url: `${prefix}v${version}/other.txt`, size: 5 }
    ],
    ...over
  }
}

function invalid(r: ReturnType<typeof pickRelease>): string {
  expect(r.kind).toBe('invalid')
  return r.kind === 'invalid' ? r.reason : ''
}

describe('update', () => {
  it('parseVersion accepts x.y.z only', () => {
    expect(parseVersion('1.2.3')).toEqual([1, 2, 3])
    expect(parseVersion('0.0.0')).toEqual([0, 0, 0])
    expect(parseVersion('10.20.30')).toEqual([10, 20, 30])
    for (const bad of ['', '1.2', '1.2.3.4', 'v1.2.3', '01.2.3', '1.02.3', '1.2.03', '-1.2.3', '1.2.-3', '1.2.3-beta', ' 1.2.3', '1.2.3\n', '1.a.3', '+1.2.3']) {
      expect(parseVersion(bad), bad).toBeNull()
    }
  })

  it('compareVersions orders', () => {
    expect(compareVersions([1, 2, 3], [1, 2, 3])).toBe(0)
    expect(compareVersions([1, 2, 3], [1, 2, 4])).toBeLessThan(0)
    expect(compareVersions([1, 3, 0], [1, 2, 9])).toBeGreaterThan(0)
    expect(compareVersions([2, 0, 0], [1, 99, 99])).toBeGreaterThan(0)
    expect(compareVersions([1, 10, 0], [1, 9, 0])).toBeGreaterThan(0)
  })

  it('feedFrom github by default', () => {
    for (const env of [undefined, '']) {
      const r = feedFrom(env)
      expect(r.refused).toBeUndefined()
      expect(r.feed).toEqual({
        api: 'https://api.github.com/repos/splitlane-app/splitlane/releases/latest',
        assetPrefix: PREFIX,
        origin: 'github'
      })
    }
  })

  it('feedFrom accepts only http 127.0.0.1 with a port', () => {
    for (const env of ['http://127.0.0.1:8123', 'http://127.0.0.1:8123/']) {
      const r = feedFrom(env)
      expect(r.refused).toBeUndefined()
      expect(r.feed).toEqual({
        api: 'http://127.0.0.1:8123/repos/splitlane-app/splitlane/releases/latest',
        assetPrefix: 'http://127.0.0.1:8123/splitlane-app/splitlane/releases/download/',
        origin: 'test',
        testOrigin: 'http://127.0.0.1:8123'
      })
    }
    for (const env of [
      'https://127.0.0.1:8123',
      'http://localhost:8123',
      'http://127.0.0.2:8123',
      'http://127.0.0.1',
      'http://127.0.0.1:8123/x',
      'http://user@127.0.0.1:8123',
      'http://user:pw@127.0.0.1:8123',
      'http://127.0.0.1:0',
      'http://127.0.0.1:99999',
      'https://example.com',
      'garbage'
    ]) {
      const r = feedFrom(env)
      expect(r.feed, env).toEqual(GH)
      expect(typeof r.refused, env).toBe('string')
      expect(r.refused).not.toBe('')
    }
  })

  it('pickRelease newer', () => {
    const r = pickRelease(release('1.2.0'), '1.1.9', GH)
    expect(r).toEqual({
      kind: 'newer',
      version: '1.2.0',
      installerUrl: `${PREFIX}v1.2.0/SplitLane-Setup-1.2.0.exe`,
      shaUrl: `${PREFIX}v1.2.0/SplitLane-Setup-1.2.0.exe.sha256`,
      notesUrl: 'https://github.com/splitlane-app/splitlane/releases/tag/v1.2.0',
      name: 'SplitLane-Setup-1.2.0.exe'
    })
    const t = pickRelease(release('1.2.0', {}, TEST.assetPrefix), '1.0.0', TEST)
    expect(t.kind).toBe('newer')
  })

  it('pickRelease not newer', () => {
    expect(pickRelease(release('1.2.0'), '1.2.0', GH)).toEqual({ kind: 'notNewer', version: '1.2.0' })
    expect(pickRelease(release('1.2.0'), '1.3.0', GH)).toEqual({ kind: 'notNewer', version: '1.2.0' })
    expect(invalid(pickRelease(release('1.2.0'), 'junk', GH))).not.toBe('')
  })

  it('pickRelease refuses a draft and a prerelease', () => {
    expect(invalid(pickRelease(release('1.2.0', { draft: true }), '1.0.0', GH))).not.toBe('')
    expect(invalid(pickRelease(release('1.2.0', { prerelease: true }), '1.0.0', GH))).not.toBe('')
  })

  it('pickRelease refuses a bad tag', () => {
    for (const tag of ['1.2.0', 'v1.2', 'v1.2.0-rc1', 'v01.2.0', '', 'latest', 5, null]) {
      expect(invalid(pickRelease(release('1.2.0', { tag_name: tag }), '1.0.0', GH)), String(tag)).not.toBe('')
    }
    for (const junk of [null, undefined, 5, 'x', [], {}]) {
      expect(invalid(pickRelease(junk, '1.0.0', GH))).not.toBe('')
    }
  })

  it('pickRelease refuses missing or duplicate assets', () => {
    const base = release('1.2.0')
    const assets = base.assets as Array<Record<string, unknown>>
    expect(invalid(pickRelease({ ...base, assets: assets.slice(1) }, '1.0.0', GH))).not.toBe('')
    expect(invalid(pickRelease({ ...base, assets: [assets[0], assets[2]] }, '1.0.0', GH))).not.toBe('')
    expect(invalid(pickRelease({ ...base, assets: [...assets, assets[0]] }, '1.0.0', GH))).not.toBe('')
    expect(invalid(pickRelease({ ...base, assets: [...assets, assets[1]] }, '1.0.0', GH))).not.toBe('')
    expect(invalid(pickRelease({ ...base, assets: 'x' }, '1.0.0', GH))).not.toBe('')
    expect(invalid(pickRelease({ ...base, assets: [null, 5, ...assets.slice(1)] }, '1.0.0', GH))).not.toBe('')
  })

  it('pickRelease refuses a foreign asset url', () => {
    const base = release('1.2.0')
    const assets = base.assets as Array<Record<string, unknown>>
    const swap = (i: number, url: unknown): unknown => ({
      ...base,
      assets: assets.map((a, j) => (j === i ? { ...a, browser_download_url: url } : a))
    })
    for (const i of [0, 1]) {
      for (const url of [
        'https://evil.example/splitlane-app/splitlane/releases/download/v1.2.0/' + (assets[i].name as string),
        `${PREFIX}v1.1.0/${assets[i].name as string}`,
        `http://github.com/splitlane-app/splitlane/releases/download/v1.2.0/${assets[i].name as string}`,
        `${PREFIX}v1.2.0/${assets[i].name as string}?x=1`,
        42
      ]) {
        expect(invalid(pickRelease(swap(i, url), '1.0.0', GH)), String(url)).not.toBe('')
      }
    }
    // a GitHub-shaped release is foreign to a test feed
    expect(invalid(pickRelease(release('1.2.0'), '1.0.0', TEST))).not.toBe('')
  })

  it('pickRelease refuses an oversized installer', () => {
    const base = release('1.2.0')
    const assets = base.assets as Array<Record<string, unknown>>
    const withSize = (size: unknown): unknown => ({ ...base, assets: [{ ...assets[0], size }, ...assets.slice(1)] })
    expect(pickRelease(withSize(400 * 1024 * 1024), '1.0.0', GH).kind).toBe('newer')
    expect(invalid(pickRelease(withSize(400 * 1024 * 1024 + 1), '1.0.0', GH))).not.toBe('')
    expect(invalid(pickRelease(withSize('big'), '1.0.0', GH))).not.toBe('')
    expect(invalid(pickRelease(withSize(-1), '1.0.0', GH))).not.toBe('')
  })

  it('allowedHop github hosts', () => {
    for (const h of ['api.github.com', 'github.com', 'objects.githubusercontent.com', 'release-assets.githubusercontent.com']) {
      expect(allowedHop(`https://${h}/x/y?z=1`, GH), h).toBe(true)
    }
    expect(allowedHop('https://GitHub.com/x', GH)).toBe(true)
    expect(allowedHop('https://github.com:443/x', GH)).toBe(true) // URL drops the default port
  })

  it('allowedHop refuses http, other hosts, ports, credentials', () => {
    for (const u of [
      'http://github.com/x',
      'https://evil.example/x',
      'https://github.com.evil.example/x',
      'https://sub.github.com/x',
      'https://githubusercontent.com/x',
      'https://github.com:8443/x',
      'https://user@github.com/x',
      'https://user:pw@github.com/x',
      'http://127.0.0.1:8123/x',
      'ftp://github.com/x',
      'not a url',
      ''
    ]) {
      expect(allowedHop(u, GH), u).toBe(false)
    }
  })

  it('allowedHop test origin only', () => {
    expect(allowedHop('http://127.0.0.1:8123/a/b', TEST)).toBe(true)
    expect(allowedHop('http://127.0.0.1:8124/a', TEST)).toBe(false)
    expect(allowedHop('https://127.0.0.1:8123/a', TEST)).toBe(false)
    expect(allowedHop('https://github.com/x', TEST)).toBe(false)
    expect(allowedHop('http://localhost:8123/x', TEST)).toBe(false)
    const noOrigin: Feed = { ...TEST, testOrigin: undefined }
    expect(allowedHop('http://127.0.0.1:8123/a', noOrigin)).toBe(false)
  })

  it('parseSha256File', () => {
    const name = 'SplitLane-Setup-1.2.0.exe'
    expect(parseSha256File(`${HEX}  ${name}\n`, name)).toBe(HEX)
    expect(parseSha256File(`${HEX.toUpperCase()} ${name}`, name)).toBe(HEX)
    expect(parseSha256File(`${HEX}\t${name}\r\n`, name)).toBe(HEX)
    expect(parseSha256File(`\n\r\n${HEX}  ${name}\nignored line`, name)).toBe(HEX)
    expect(parseSha256File(`${HEX}  other.exe`, name)).toBeNull()
    expect(parseSha256File(`${HEX}   ${name}`, name)).toBeNull()
    expect(parseSha256File(`${HEX}${name}`, name)).toBeNull()
    expect(parseSha256File(`${HEX.slice(1)}  ${name}`, name)).toBeNull()
    expect(parseSha256File(`${HEX}0  ${name}`, name)).toBeNull()
    expect(parseSha256File(`${'zz'.repeat(32)}  ${name}`, name)).toBeNull()
    expect(parseSha256File(`${HEX}  ${name}x`, name)).toBeNull()
    expect(parseSha256File('', name)).toBeNull()
    expect(parseSha256File('\n  \n', name)).toBeNull()
  })

  it('parseLastInstall', () => {
    expect(parseLastInstall('1.2.0\r\nok\r\n\r\n')).toEqual({ version: '1.2.0', result: 'ok', reason: '' })
    expect(parseLastInstall('1.2.0\nok')).toEqual({ version: '1.2.0', result: 'ok', reason: '' })
    expect(parseLastInstall('1.2.0\r\nrefused\r\nнет места на диске\r\n')).toEqual({
      version: '1.2.0',
      result: 'refused',
      reason: 'нет места на диске'
    })
    expect(parseLastInstall('1.2.0\nrefused')).toEqual({ version: '1.2.0', result: 'refused', reason: '' })
    expect(parseLastInstall('v1.2.0\nok\n')).toBeNull()
    expect(parseLastInstall('1.2\nok\n')).toBeNull()
    expect(parseLastInstall('1.2.0\nmaybe\n')).toBeNull()
    expect(parseLastInstall('1.2.0\n')).toBeNull()
    expect(parseLastInstall('')).toBeNull()
  })

  it('outcomeAfterRestart installed', () => {
    expect(outcomeAfterRestart('1.2.0', '1.2.0', null)).toEqual({ kind: 'installed', version: '1.2.0' })
    expect(outcomeAfterRestart('1.2.0', '1.2.0', { version: '1.2.0', result: 'refused', reason: 'x' })).toEqual({
      kind: 'installed',
      version: '1.2.0'
    })
  })

  it('outcomeAfterRestart refused with reason', () => {
    expect(outcomeAfterRestart('1.2.0', '1.1.0', { version: '1.2.0', result: 'refused', reason: 'busy' })).toEqual({
      kind: 'installRefused',
      version: '1.2.0',
      notesUrl: notesUrlFor('1.2.0'),
      reason: 'busy'
    })
    expect(outcomeAfterRestart('1.2.0', '1.1.0', null)).toEqual({
      kind: 'installRefused',
      version: '1.2.0',
      notesUrl: notesUrlFor('1.2.0'),
      reason: ''
    })
  })

  it('outcomeAfterRestart ignores a result of another version', () => {
    expect(outcomeAfterRestart('1.2.0', '1.1.0', { version: '1.0.5', result: 'refused', reason: 'old' })).toEqual({
      kind: 'installRefused',
      version: '1.2.0',
      notesUrl: notesUrlFor('1.2.0'),
      reason: ''
    })
    expect(outcomeAfterRestart('1.2.0', '1.1.0', { version: '1.2.0', result: 'ok', reason: '' })).toEqual({
      kind: 'installRefused',
      version: '1.2.0',
      notesUrl: notesUrlFor('1.2.0'),
      reason: ''
    })
  })

  it('outcomeAfterRestart without pending is null', () => {
    expect(outcomeAfterRestart(null, '1.2.0', null)).toBeNull()
    expect(outcomeAfterRestart(null, '1.2.0', { version: '1.2.0', result: 'refused', reason: 'x' })).toBeNull()
  })
})
