import { describe, expect, it } from 'vitest'
import { activityAt, PHONE_ADDRESS, PROGRAMS, SCENES, sceneOf } from './scenes'

const ALLOWED_PROGRAMS = [
  'Steam',
  'Epic Games Launcher',
  'Discord',
  'Firefox',
  'Counter-Strike 2',
  'Fortnite',
  'Microsoft Edge'
]
const ALLOWED_HOSTS = ['203-0-113-10.sslip.io', 'vps.example.com', '127.0.0.1']

/** Every string of a value, keys included, for the scans below. */
function strings(x: unknown, out: string[] = []): string[] {
  if (typeof x === 'string') out.push(x)
  else if (Array.isArray(x)) x.forEach((v) => strings(v, out))
  else if (x !== null && typeof x === 'object') {
    for (const [k, v] of Object.entries(x)) {
      out.push(k)
      strings(v, out)
    }
  }
  return out
}

function everything(): string[] {
  const out: string[] = [...strings(PROGRAMS), PHONE_ADDRESS]
  for (const name of SCENES) {
    const scene = sceneOf(name)
    out.push(...strings(scene.snapshot))
    if (scene.vps !== null) out.push(...strings(scene.vps(0)))
  }
  out.push(...strings(activityAt(3)))
  return out
}

describe('the data of the demo', () => {
  it('holds the listed programs and no others', () => {
    expect(PROGRAMS.map((p) => p.name).sort()).toEqual([...ALLOWED_PROGRAMS].sort())
    for (const scene of SCENES) {
      for (const rule of sceneOf(scene).snapshot.state?.apps ?? []) expect(ALLOWED_PROGRAMS).toContain(rule.name)
    }
  })

  it('has only documentation addresses and the allowed hosts', () => {
    const text = everything().join('\n')
    for (const ip of text.match(/\b\d{1,3}(?:\.\d{1,3}){3}\b/g) ?? []) {
      // the helper/core versions are not addresses: 3 numbers; four are TEST-NET-3 (203.0.113.x) or loopback
      expect(ip.startsWith('203.0.113.') || ip === '127.0.0.1', ip).toBe(true)
    }
    for (const host of text.match(/[a-z0-9-]+(?:\.[a-z0-9-]+)*\.(?:io|com|net|org|ru)\b/gi) ?? []) {
      expect(ALLOWED_HOSTS, host).toContain(host.toLowerCase())
    }
  })

  it('has no user, PC or tailnet names in paths', () => {
    for (const s of everything()) {
      expect(s).not.toMatch(/\\Users\\(?!Public\\)/i)
      expect(s).not.toMatch(/ts\.net|tailscale\.com/i)
    }
  })

  it('shows the phone address of the spec', () => {
    expect(PHONE_ADDRESS).toMatch(/^https:\/\/203-0-113-10\.sslip\.io:8443\/[0-9a-f]{32}\/sub$/)
  })
})
