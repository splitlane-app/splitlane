import { afterEach, describe, expect, it } from 'vitest'
import { ERROR_CODES, PROXY_REJECT_CODES, REASON_CODES, type RejectReason } from '../../shared/protocol'
import { en } from './en'
import { ru, type Key } from './ru'
import { currentLang, setLang, t } from './t'

afterEach(() => setLang('ru'))

const keys = (d: Record<string, string>): string[] => Object.keys(d).sort()
const placeholders = (text: string): string[] => [...text.matchAll(/\{(\w+)\}/g)].map((m) => m[1]).sort()

describe('dictionary', () => {
  it('every reason, error and rejection code has a text', () => {
    const rejects: RejectReason[] = ['too_broad', 'protected', 'duplicate']
    const codes = [
      ...REASON_CODES.map((c) => `reason.${c}`),
      ...ERROR_CODES.map((c) => `error.${c}`),
      ...rejects.map((r) => `reject.${r}`)
    ]
    expect(codes).toHaveLength(19 + 28 + 3)
    for (const dict of [ru, en] as Record<string, string>[]) {
      for (const key of codes) {
        const text = dict[key]
        expect(typeof text, key).toBe('string')
        expect(text.trim().length, key).toBeGreaterThan(0)
      }
    }
  })

  it('every code a line of proxy.add may be rejected with has a text of its own', () => {
    for (const dict of [ru, en] as Record<string, string>[]) {
      for (const code of PROXY_REJECT_CODES) expect(dict[`error.${code}`]?.trim().length, code).toBeGreaterThan(0)
    }
    // no two codes read the same: each tells the user something different
    for (const dict of [ru, en] as Record<string, string>[]) {
      const texts = PROXY_REJECT_CODES.map((c) => dict[`error.${c}`])
      expect(new Set(texts).size).toBe(texts.length)
    }
  })

  it('the failures of the VPN lane speak of the VPN server, not of an exit node (either connector)', () => {
    const neutral: Key[] = [
      'reason.tunnel_not_proven',
      'reason.same_address',
      'reason.vpn_unreachable',
      'banner.degraded',
      'banner.degradedUnnamed'
    ]
    for (const key of neutral) {
      expect(ru[key], key).not.toMatch(/exit node/i)
      expect(ru[key], key).toContain('VPN-сервер')
      expect(en[key], key).not.toMatch(/exit node/i)
      expect(en[key], key).toMatch(/VPN server/)
    }
    expect(ru['lane.vpnSees']).not.toMatch(/exit node/i)
    expect(en['lane.vpnSees']).not.toMatch(/exit node/i)
  })

  it('en has every key of ru', () => {
    // the type Record<Key, string> of en already refuses a missing or an extra key; this guards the runtime object
    expect(keys(en)).toEqual(keys(ru))
    for (const [key, text] of Object.entries(en)) {
      expect(typeof text, key).toBe('string')
      expect(text.trim().length, key).toBeGreaterThan(0)
    }
  })

  it('en and ru have the same placeholders', () => {
    for (const key of Object.keys(ru) as Key[]) {
      expect(placeholders(en[key]), key).toEqual(placeholders(ru[key]))
    }
  })

  it('English texts are English and calm: no Cyrillic but the name of Russian, no exclamation marks', () => {
    for (const [key, text] of Object.entries(en)) {
      if (key !== 'language.ru') expect(text, key).not.toMatch(/[а-яё]/i)
      expect(text, key).not.toContain('!')
    }
  })

  it('English texts follow the glossary', () => {
    const glossary: Partial<Record<Key, string>> = {
      'lane.vpn': 'Via VPN',
      'lane.direct': 'Direct',
      'lane.main': 'main',
      'lane.makeMain': 'make main',
      'move.vpn': 'Send via VPN',
      'move.direct': 'Send direct',
      unpin: 'Unpin',
      'card.quiet': 'quiet',
      'header.on': 'On',
      'header.off': 'Off',
      'connection.title': 'Connection',
      'setup.title': 'First-time setup',
      'setup.install': 'Install the service',
      'setup.update': 'Update the service',
      'update.check': 'Check for updates',
      'update.available': 'Version {v} is available',
      'update.action.install': 'Update',
      'update.action.notes': "What's new",
      'update.action.later': 'Later',
      'update.action.dismiss': 'Close',
      'update.action.retry': 'Retry',
      'update.action.cancel': 'Cancel',
      'action.retry': 'Retry',
      'action.cancel': 'Cancel',
      'add.close': 'Close',
      'connection.language': 'Language',
      'connection.languageSystem': 'Same as Windows',
      'connect.key': 'Key or subscription',
      'connect.file': 'Configuration file',
      'proxy.refresh': 'Refresh',
      'proxy.subscriptions': 'Subscriptions',
      'proxy.servers': 'Servers',
      // stage 3 part 2
      'connect.vps': 'Your own VPS',
      'vps.setUp': 'Set up',
      'vps.cancel': 'Cancel'
    }
    for (const [key, text] of Object.entries(glossary)) expect(en[key as Key], key).toBe(text)
  })

  it('Russian texts follow the glossary', () => {
    const glossary: Partial<Record<Key, string>> = {
      'connect.key': 'Ключ или подписка',
      'connect.file': 'Файл настроек',
      'proxy.refresh': 'Обновить',
      'proxy.subscriptions': 'Подписки',
      'proxy.servers': 'Серверы',
      'connect.vps': 'Свой VPS',
      'vps.setUp': 'Настроить',
      'vps.cancel': 'Отменить'
    }
    for (const [key, text] of Object.entries(glossary)) expect(ru[key as Key], key).toBe(text)
  })

  it('the names of the languages are in their own language in both dictionaries', () => {
    for (const dict of [ru, en]) {
      expect(dict['language.ru']).toBe('Русский')
      expect(dict['language.en']).toBe('English')
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

  it('t uses the current language', () => {
    expect(currentLang()).toBe('ru')
    setLang('en')
    expect(currentLang()).toBe('en')
    expect(t('lane.vpn')).toBe('Via VPN')
    expect(t('update.available', { v: '1.2.0' })).toBe('Version 1.2.0 is available')
    expect(() => t('banner.error')).toThrow(/reason/)
    setLang('ru')
    expect(t('lane.vpn')).toBe('Через VPN')
    expect(t('update.available', { v: '1.2.0' })).toBe('Доступна версия 1.2.0')
  })
})
