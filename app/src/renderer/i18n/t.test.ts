import { afterEach, describe, expect, it } from 'vitest'
import { ERROR_CODES, REASON_CODES, type RejectReason } from '../../shared/protocol'
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
    expect(codes).toHaveLength(19 + 27 + 3)
    for (const dict of [ru, en] as Record<string, string>[]) {
      for (const key of codes) {
        const text = dict[key]
        expect(typeof text, key).toBe('string')
        expect(text.trim().length, key).toBeGreaterThan(0)
      }
    }
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
      'connection.languageSystem': 'Same as Windows'
    }
    for (const [key, text] of Object.entries(glossary)) expect(en[key as Key], key).toBe(text)
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
