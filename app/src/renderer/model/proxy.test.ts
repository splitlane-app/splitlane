import { afterEach, expect, test } from 'vitest'
import { LIMITS, PROXY_PROTOCOLS, PROXY_REJECT_CODES } from '../../shared/protocol'
import { LIMIT_PARAMS, setLang, t } from '../i18n/t'
import { errorText } from '../store/appStore'
import { proxyServer, proxyState } from '../testing'
import {
  addRequest,
  commandErrorText,
  FILE_REQUEST,
  isSubscriptionAddress,
  outcomeNotice,
  protocolLabel,
  proxyCall,
  proxyErrorText,
  refreshRequest,
  rejectionLine,
  rejectionText,
  sourceText,
  subscriptionErrorText,
  updatedText
} from './proxy'

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
    expect(rejectionText(code), code).toBe(t(`error.${code}`, LIMIT_PARAMS))
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

test('a failed add or refresh: the phase and a download have texts of their own, the rest the error texts', () => {
  const sub = { subscription: true, phaseBusy: false }
  expect(proxyErrorText({ code: 'wrong_phase', message: '' }, sub)).toBe(
    'Подписку выбранного сервера можно обновить, когда деление выключено'
  )
  expect(proxyErrorText({ code: 'busy', message: '' }, sub)).toBe('Уже идёт загрузка подписки')
  expect(proxyErrorText({ code: 'fetch_blocked_address', message: '' }, sub)).toBe(
    'Адрес подписки ведёт в местную сеть — такие адреса запрещены'
  )
  expect(proxyErrorText({ code: 'internal', message: 'link closed' }, sub)).toBe(t('error.internal'))
  setLang('en')
  expect(proxyErrorText({ code: 'busy', message: '' }, sub)).toBe('A subscription is already downloading')
  expect(proxyErrorText({ code: 'wrong_phase', message: '' }, sub)).toBe(
    'You can refresh the subscription of the selected server when split routing is off'
  )
})

test('busy is another download only when there was one to make and the phase cannot be the cause', () => {
  const general = t('error.busy')
  // links or a file: nothing to download (a busy helper is one shutting down)
  expect(proxyErrorText({ code: 'busy', message: '' }, { subscription: false, phaseBusy: false })).toBe(general)
  expect(proxyErrorText({ code: 'busy', message: '' }, FILE_REQUEST)).toBe(general)
  expect(proxyErrorText({ code: 'wrong_phase', message: '' }, FILE_REQUEST)).toBe(t('error.wrong_phase'))
  // routing starting or stopping on the subscription
  expect(proxyErrorText({ code: 'busy', message: '' }, { subscription: true, phaseBusy: true })).toBe(general)

  const a = proxyServer('0123456789abcdef', 'a', { source: SUB.id })
  const other = { ...SUB, id: 'sub-ffffffffffffffff', name: 'other.example.com' }
  for (const phase of ['starting', 'stopping'] as const) {
    const s = proxyState([a], a.id, [SUB, other], { phase })
    expect(refreshRequest(s, SUB), phase).toEqual({ subscription: true, phaseBusy: true })
    expect(refreshRequest(s, other), phase).toEqual({ subscription: true, phaseBusy: false })
    expect(addRequest(s, 'https://sub.example.com/x'), phase).toEqual({ subscription: true, phaseBusy: true })
  }
  const on = proxyState([a], a.id, [SUB], { phase: 'on' })
  // live: a busy one is a download (the phase answers wrong_phase)
  expect(refreshRequest(on, SUB)).toEqual({ subscription: true, phaseBusy: false })
  expect(addRequest(on, '  https://sub.example.com/x  ')).toEqual({ subscription: true, phaseBusy: false })
  expect(addRequest(on, 'vless://made-up@192.0.2.1:443')).toEqual({ subscription: false, phaseBusy: false })
  // Tailscale in force: the proxy list is free in every phase
  expect(addRequest({ ...on, connector: 'tailscale', phase: 'starting' }, 'https://sub.example.com/x').phaseBusy).toBe(false)
})

test('a subscription address is one https address alone; an http one is no download (the helper refuses it at once)', () => {
  expect(isSubscriptionAddress('https://sub.example.com/list?token=made-up')).toBe(true)
  expect(isSubscriptionAddress('  HTTPS://sub.example.com \n')).toBe(true)
  expect(isSubscriptionAddress('  HTTP://sub.example.com \n')).toBe(false)
  expect(isSubscriptionAddress('http://sub.example.com/list?token=made-up')).toBe(false)
  expect(isSubscriptionAddress('https://a.example.com\nhttps://b.example.com')).toBe(false)
  expect(isSubscriptionAddress('vless://x@a.example.com:443')).toBe(false)
  expect(isSubscriptionAddress('')).toBe(false)
  // so a busy answer to it is the general text, not «another download»
  const on = proxyState([], null, [], { phase: 'on' })
  expect(proxyErrorText({ code: 'busy', message: '' }, addRequest(on, 'http://sub.example.com/x'))).toBe(t('error.busy'))
  expect(rejectionLine({ line: 0, code: 'fetch_not_https' })).toBe('Адрес подписки должен начинаться с https://')
})

test('apply_failed of a proxy method is the list of servers, not the layout', () => {
  const failed = { code: 'apply_failed' as const, message: 'servers.json cannot be read now' }
  for (const req of [{ subscription: true, phaseBusy: false }, { subscription: false, phaseBusy: false }, FILE_REQUEST]) {
    expect(proxyErrorText(failed, req)).toBe('Не удалось прочитать или сохранить список серверов')
  }
  expect(outcomeNotice({ kind: 'failed', error: failed }, FILE_REQUEST)).toBe('Не удалось прочитать или сохранить список серверов')
  const id = '0123456789abcdef'
  expect(commandErrorText({ method: 'proxy.remove', id }, failed)).toBe('Не удалось прочитать или сохранить список серверов')
  expect(commandErrorText({ method: 'proxy.select', id }, failed)).toBe('Не удалось прочитать или сохранить список серверов')
  expect(commandErrorText({ method: 'proxy.remove', id: SUB.id }, failed)).toBe('Не удалось прочитать или сохранить список серверов')
  // other errors of the proxy methods and apply_failed of the others keep their texts
  expect(commandErrorText({ method: 'proxy.remove', id }, { code: 'wrong_phase', message: '' })).toBe(t('error.wrong_phase'))
  expect(commandErrorText({ method: 'lanes.set', apps: [], defaultLane: 'vpn' }, failed)).toBe(t('error.apply_failed'))
  expect(commandErrorText({ method: 'routing.allDirect', on: true }, failed)).toBe(
    'Раскладку не удалось применить, прежняя возвращена'
  )
  setLang('en')
  expect(proxyErrorText(failed, FILE_REQUEST)).toBe("Couldn't read or save the list of servers")
  expect(commandErrorText({ method: 'proxy.select', id }, failed)).toBe("Couldn't read or save the list of servers")
})

test('a WireGuard file that sends only some addresses through the server says what file is needed', () => {
  expect(rejectionText('wireguard_partial')).toBe(
    'Этот файл WireGuard ведёт через сервер не все адреса. Для SplitLane нужен файл с AllowedIPs = 0.0.0.0/0'
  )
  expect(rejectionLine({ line: 0, code: 'wireguard_partial' })).toBe(rejectionText('wireguard_partial'))
  setLang('en')
  expect(rejectionText('wireguard_partial')).toBe(
    'This WireGuard file sends only some addresses through the server. SplitLane needs a file with AllowedIPs = 0.0.0.0/0'
  )
})

test('the notice of a late answer: the numbers only, never a line', () => {
  const req = { subscription: false, phaseBusy: false }
  const done = (added: number, rejected: number, more: number) => ({
    kind: 'done' as const,
    result: {
      added: Array.from({ length: added }, (_, i) => `00000000000000${String(i).padStart(2, '0')}`),
      existing: 4,
      servers: [],
      subscription: null,
      rejected: Array.from({ length: rejected }, (_, i) => ({ line: i + 1, code: 'bad_link' })),
      rejectedMore: more
    }
  })
  expect(outcomeNotice(done(2, 0, 0), req)).toBe('Добавлено серверов: 2')
  expect(outcomeNotice(done(1, 3, 2), req)).toBe('Добавлено серверов: 1 · Отклонено строк: 5')
  expect(outcomeNotice(done(1, 3, 2), req)).not.toMatch(/Строка|Ссылка/)
  expect(outcomeNotice({ kind: 'failed', error: { code: 'fetch_timeout', message: '' } }, req)).toBe(
    'Сервер подписки не ответил за 20 секунд'
  )
  expect(outcomeNotice({ kind: 'cancelled' }, req)).toBeNull()
  setLang('en')
  expect(outcomeNotice(done(0, 1, 0), req)).toBe('Servers added: 0 · Lines rejected: 1')
})

test('the texts of the limits take their numbers from the shared limits', () => {
  expect(LIMITS).toMatchObject({ maxServers: 500, maxSubscriptions: 20, maxSubscriptionBytes: 1 << 20, fetchTimeoutSeconds: 20 })
  const texts = (): Record<string, string> => ({
    too_many: errorText({ code: 'too_many', message: '' }),
    too_long: rejectionText('too_long'),
    fetch_too_large: rejectionText('fetch_too_large'),
    fetch_timeout: subscriptionErrorText('fetch_timeout')
  })
  for (const lang of ['ru', 'en'] as const) {
    setLang(lang)
    const x = texts()
    expect(x.too_many, lang).toMatch(new RegExp(`\\b${LIMITS.maxServers}\\b.*\\b${LIMITS.maxSubscriptions}\\b`))
    expect(x.too_long, lang).toContain(`${LIMITS.maxProxyTextBytes / 1024} `)
    expect(x.fetch_too_large, lang).toContain(`${LIMITS.maxSubscriptionBytes / (1024 * 1024)} `)
    expect(x.fetch_timeout, lang).toContain(`${LIMITS.fetchTimeoutSeconds} `)
    for (const text of Object.values(x)) expect(text, lang).not.toContain('{')
  }
  setLang('ru')
  expect(texts()).toEqual({
    too_many: 'Слишком много серверов или подписок: можно до 500 серверов и 20 подписок',
    too_long: 'Текст или файл больше 64 КБ',
    fetch_too_large: 'Подписка больше 1 МБ',
    fetch_timeout: 'Сервер подписки не ответил за 20 секунд'
  })
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
