import { afterEach, expect, test } from 'vitest'
import { PROXY_PROTOCOLS, PROXY_REJECT_CODES } from '../../shared/protocol'
import { setLang, t } from '../i18n/t'
import { proxyServer, proxyState } from '../testing'
import { protocolLabel, proxyCall, proxyErrorText, rejectionLine, rejectionText, sourceText, subscriptionErrorText, updatedText } from './proxy'

afterEach(() => setLang('ru'))

const SUB = { id: 'sub-0123456789abcdef', name: 'sub.example.com', updatedAt: '2026-11-16T10:00:00Z', count: 2, error: null }

test('protocol labels are the names of the projects, the same in both languages', () => {
  expect(PROXY_PROTOCOLS.map(protocolLabel)).toEqual(['VLESS', 'VMess', 'Trojan', 'Shadowsocks', 'Hysteria2', 'TUIC', 'WireGuard'])
  setLang('en')
  expect(protocolLabel('vmess')).toBe('VMess')
})

test('the source of a server: by hand, a file, or the name of its subscription', () => {
  const manual = proxyServer('0123456789abcdef', 'a')
  const file = proxyServer('0123456789abcde0', 'b', { source: 'file' })
  const fromSub = proxyServer('0123456789abcde1', 'c', { source: SUB.id })
  const orphan = proxyServer('0123456789abcde2', 'd', { source: 'sub-ffffffffffffffff' })
  const s = proxyState([manual, file, fromSub, orphan], null, [SUB])
  expect([manual, file, fromSub, orphan].map((x) => sourceText(s, x))).toEqual(['вручную', 'файл', 'sub.example.com', null])
  setLang('en')
  expect([manual, file].map((x) => sourceText(s, x))).toEqual(['added by hand', 'file'])
})

test('every rejection code has its own text; any other code a generic one', () => {
  for (const code of PROXY_REJECT_CODES) {
    expect(rejectionText(code), code).toBe(t(`error.${code}`))
    expect(rejectionText(code), code).not.toBe(t('proxy.unknownLine'))
  }
  expect(rejectionText('something_new')).toBe('Строка не распознана')
  // a code of the helper that is not a rejection is not taken for one
  expect(rejectionText('internal')).toBe('Строка не распознана')
  setLang('en')
  expect(rejectionText('something_new')).toBe('The line was not recognized')
})

test('a rejected line carries its number; the whole text (line 0) the reason alone', () => {
  expect(rejectionLine({ line: 3, code: 'unsupported_transport' })).toBe('Строка 3: Этот транспорт SplitLane пока не поддерживает')
  expect(rejectionLine({ line: 0, code: 'fetch_timeout' })).toBe('Сервер подписки не ответил за 20 секунд')
  expect(rejectionLine({ line: 7, code: 'nope' })).toBe('Строка 7: Строка не распознана')
  setLang('en')
  expect(rejectionLine({ line: 2, code: 'amneziawg' })).toBe("Line 2: AmneziaWG files aren't supported yet")
})

test('a failed add or refresh: the phase and busy have texts of their own, the rest the error texts', () => {
  expect(proxyErrorText({ code: 'wrong_phase', message: '' })).toBe(
    'Подписку выбранного сервера можно обновить, когда деление выключено'
  )
  expect(proxyErrorText({ code: 'busy', message: '' })).toBe('Уже идёт загрузка подписки')
  expect(proxyErrorText({ code: 'fetch_blocked_address', message: '' })).toBe(
    'Адрес подписки ведёт в местную сеть — такие адреса запрещены'
  )
  expect(proxyErrorText({ code: 'internal', message: 'link closed' })).toBe(t('error.internal'))
  setLang('en')
  expect(proxyErrorText({ code: 'busy', message: '' })).toBe('A subscription is already downloading')
  expect(proxyErrorText({ code: 'wrong_phase', message: '' })).toBe(
    'You can refresh the subscription of the selected server when split routing is off'
  )
})

test('the last error of a subscription; an unknown code reads as a failed download', () => {
  expect(subscriptionErrorText('fetch_http_status')).toBe('Сервер подписки ответил ошибкой. Проверьте адрес')
  expect(subscriptionErrorText('something_new')).toBe(t('error.fetch_failed'))
})

test('the time of the last update in the language of the window; nothing before the first one', () => {
  const at = '2026-11-16T10:00:00Z'
  const ru = new Intl.DateTimeFormat('ru', { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(at))
  expect(updatedText(at, 'ru')).toBe(`Обновлена: ${ru}`)
  expect(ru).toMatch(/2026/)
  setLang('en')
  const en = new Intl.DateTimeFormat('en', { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(at))
  expect(updatedText(at, 'en')).toBe(`Updated: ${en}`)
  expect(en).toMatch(/Nov/)
  expect(updatedText('', 'en')).toBeNull()
  expect(updatedText('not a time', 'en')).toBeNull()
})

test('a bridge call that throws is a lost link', async () => {
  expect(await proxyCall(async () => ({ kind: 'cancelled' }))).toEqual({ kind: 'cancelled' })
  expect(
    await proxyCall(async () => {
      throw new Error('gone')
    })
  ).toEqual({ kind: 'failed', error: { code: 'internal', message: 'gone' } })
})
