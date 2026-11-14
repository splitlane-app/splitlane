import { describe, expect, it } from 'vitest'
import { ERROR_CODES, REASON_CODES, type RejectReason } from '../../shared/protocol'
import { ru, type Key } from './ru'
import { t } from './t'

describe('dictionary', () => {
  it('every reason, error and rejection code has a text', () => {
    const rejects: RejectReason[] = ['too_broad', 'protected', 'duplicate']
    const keys = [
      ...REASON_CODES.map((c) => `reason.${c}`),
      ...ERROR_CODES.map((c) => `error.${c}`),
      ...rejects.map((r) => `reject.${r}`)
    ]
    expect(keys).toHaveLength(18 + 9 + 3)
    for (const key of keys) {
      const text = (ru as Record<string, string>)[key]
      expect(typeof text, key).toBe('string')
      expect(text.trim().length, key).toBeGreaterThan(0)
    }
  })

  it('t substitutes params', () => {
    expect(t('banner.degraded', { name: 'node-a' })).toContain('node-a')
    expect(t('banner.degraded', { name: 'node-a' })).not.toContain('{')
    expect(() => t('banner.degraded')).toThrow(Error)
    expect(() => t('banner.error', { other: 1 })).toThrow(/reason/)
    expect(t('lane.vpn')).toBe('Через VPN')
    const k: Key = 'banner.error'
    expect(t(k, { reason: 5 })).toContain('5')
  })
})
