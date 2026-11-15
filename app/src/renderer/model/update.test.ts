import { afterEach, expect, test } from 'vitest'
import netHopSource from '../../main/netHop.ts?raw'
import updaterSource from '../../main/updater.ts?raw'
import sharedUpdateSource from '../../shared/update.ts?raw'
import type { UpdateState } from '../../shared/update'
import { setLang } from '../i18n/t'
import { translateUpdateReason, updateBannerModel, updateStatusLine } from './update'

afterEach(() => setLang('ru'))

const N = 'https://github.com/splitlane-app/splitlane/releases/tag/v1.2.0'

const rows: [string, UpdateState, { text: string; actions: string[]; tone: string } | null, string | null][] = [
  ['idle', { kind: 'idle' }, null, null],
  ['checking', { kind: 'checking', manual: true }, null, 'Проверка обновлений…'],
  ['upToDate', { kind: 'upToDate' }, null, 'Установлена последняя версия'],
  ['checkFailed', { kind: 'checkFailed', reason: 'HTTP 404' }, null, 'Не удалось проверить обновления: ошибка HTTP 404'],
  [
    'available',
    { kind: 'available', version: '1.2.0', notesUrl: N, hidden: false },
    { text: 'Доступна версия 1.2.0', actions: ['install', 'notes', 'later'], tone: 'info' },
    'Доступна версия 1.2.0'
  ],
  ['available hidden', { kind: 'available', version: '1.2.0', notesUrl: N, hidden: true }, null, 'Доступна версия 1.2.0'],
  [
    'downloading',
    { kind: 'downloading', version: '1.2.0', notesUrl: N, percent: 42 },
    { text: 'Загрузка версии 1.2.0: 42%', actions: ['cancel'], tone: 'info' },
    'Загрузка версии 1.2.0: 42%'
  ],
  [
    'verifying',
    { kind: 'verifying', version: '1.2.0', notesUrl: N },
    { text: 'Проверка версии 1.2.0…', actions: [], tone: 'info' },
    'Проверка версии 1.2.0…'
  ],
  [
    'launching',
    { kind: 'launching', version: '1.2.0', notesUrl: N },
    { text: 'Установка версии 1.2.0: Windows попросит подтверждение', actions: [], tone: 'info' },
    'Установка версии 1.2.0: Windows попросит подтверждение'
  ],
  [
    'cancelled',
    { kind: 'cancelled', version: '1.2.0', notesUrl: N },
    { text: 'Обновление отменено', actions: ['install', 'dismiss'], tone: 'info' },
    'Обновление отменено'
  ],
  [
    'downloadFailed',
    { kind: 'downloadFailed', version: '1.2.0', notesUrl: N, reason: 'the file does not match its checksum' },
    {
      text: 'Не удалось скачать версию 1.2.0: файл не совпадает с контрольной суммой',
      actions: ['retry', 'dismiss'],
      tone: 'warn'
    },
    'Не удалось скачать версию 1.2.0: файл не совпадает с контрольной суммой'
  ],
  [
    'installed',
    { kind: 'installed', version: '1.2.0' },
    { text: 'SplitLane обновлён до 1.2.0', actions: ['dismiss'], tone: 'info' },
    'SplitLane обновлён до 1.2.0'
  ],
  [
    'installRefused',
    { kind: 'installRefused', version: '1.2.0', notesUrl: N, reason: 'not signed', canRetry: true },
    { text: 'Обновление до 1.2.0 не установлено: not signed', actions: ['retry', 'notes', 'dismiss'], tone: 'warn' },
    'Обновление до 1.2.0 не установлено: not signed'
  ],
  [
    'installRefused without a reason',
    { kind: 'installRefused', version: '1.2.0', notesUrl: N, reason: '', canRetry: true },
    { text: 'Обновление до 1.2.0 не установлено', actions: ['retry', 'notes', 'dismiss'], tone: 'warn' },
    'Обновление до 1.2.0 не установлено'
  ],
  [
    'installRefused of the window version',
    { kind: 'installRefused', version: '1.2.0', notesUrl: N, reason: 'the task', canRetry: false },
    { text: 'Обновление до 1.2.0 не установлено: the task', actions: ['dismiss'], tone: 'warn' },
    'Обновление до 1.2.0 не установлено: the task'
  ],
  [
    'installRefused by the launch watch',
    { kind: 'installRefused', version: '1.2.0', notesUrl: N, reason: 'the installer did not start the update', canRetry: true },
    {
      text: 'Обновление до 1.2.0 не установлено: установщик не начал обновление',
      actions: ['retry', 'notes', 'dismiss'],
      tone: 'warn'
    },
    'Обновление до 1.2.0 не установлено: установщик не начал обновление'
  ]
]

test('updateBannerModel for every state', () => {
  for (const [name, state, banner] of rows) expect(updateBannerModel(state), name).toEqual(banner)
})

test('updateStatusLine for every state', () => {
  for (const [name, state, , line] of rows) expect(updateStatusLine(state), name).toBe(line)
})

test('the update texts follow the language, the reasons too', () => {
  setLang('en')
  expect(updateBannerModel({ kind: 'available', version: '1.2.0', notesUrl: N, hidden: false })?.text).toBe(
    'Version 1.2.0 is available'
  )
  expect(
    updateBannerModel({ kind: 'downloadFailed', version: '1.2.0', notesUrl: N, reason: 'too many redirects' })?.text
  ).toBe("Couldn't download version 1.2.0: too many redirects")
  expect(updateStatusLine({ kind: 'checkFailed', reason: 'HTTP 403 (GitHub request limit?)' })).toBe(
    "Couldn't check for updates: HTTP 403 (GitHub request limit reached?)"
  )
  expect(updateStatusLine({ kind: 'upToDate' })).toBe("You're up to date")
})

test('known update reasons are translated', () => {
  // [the text of the main process, Russian, English]
  const known: [string, string, string][] = [
    ['the release is not available', 'выпуск недоступен', "the release isn't available"],
    ['the checksum file is larger than 1 KB', 'файл контрольной суммы больше 1 КБ', 'the checksum file is larger than 1 KB'],
    ['the file is larger than the release says', 'файл больше, чем указано в выпуске', 'the file is larger than the release says'],
    ['the file is larger than 400 MB', 'файл больше 400 МБ', 'the file is larger than 400 MB'],
    ['the file does not match its checksum', 'файл не совпадает с контрольной суммой', "the file doesn't match its checksum"],
    ['the release answer is larger than 1 MB', 'ответ о выпуске больше 1 МБ', 'the release information is larger than 1 MB'],
    ['HTTP 403 (GitHub request limit?)', 'HTTP 403 (превышен лимит запросов к GitHub?)', 'HTTP 403 (GitHub request limit reached?)'],
    ['HTTP 500', 'ошибка HTTP 500', 'HTTP error 500'],
    ['HTTP 404', 'ошибка HTTP 404', 'HTTP error 404'],
    ['the installer did not start the update', 'установщик не начал обновление', "the installer didn't start the update"],
    ['redirect to a host that is not allowed', 'перенаправление на неразрешённый узел', "redirected to a host that isn't allowed"],
    ['too many redirects', 'слишком много перенаправлений', 'too many redirects'],
    ['the address is not allowed', 'адрес не разрешён', "the address isn't allowed"],
    ['powershell exit 1', 'PowerShell завершился с кодом 1', 'PowerShell exited with code 1'],
    [
      'release must have exactly one file SplitLane-Setup-1.2.0.exe.sha256',
      'в выпуске должен быть ровно один файл SplitLane-Setup-1.2.0.exe.sha256',
      'the release must have exactly one file SplitLane-Setup-1.2.0.exe.sha256'
    ]
  ]
  for (const [reason, ru, en] of known) {
    setLang('ru')
    expect(translateUpdateReason(reason), reason).toBe(ru)
    setLang('en')
    expect(translateUpdateReason(reason), reason).toBe(en)
  }
})

test('unknown update reasons pass through', () => {
  const unknown = [
    'net::ERR_INTERNET_DISCONNECTED',
    'the task cannot be registered',
    '',
    'HTTP 403 (another hint)',
    'HTTP 4040',
    'http 500',
    'too many redirects ',
    'constructor',
    'toString',
    '__proto__'
  ]
  for (const lang of ['ru', 'en'] as const) {
    setLang(lang)
    for (const reason of unknown) expect(translateUpdateReason(reason), `${lang}: ${reason}`).toBe(reason)
  }
  setLang('en')
  expect(updateStatusLine({ kind: 'checkFailed', reason: 'net::ERR_TIMED_OUT' })).toBe(
    "Couldn't check for updates: net::ERR_TIMED_OUT"
  )
  expect(
    updateBannerModel({ kind: 'installRefused', version: '1.2.0', notesUrl: N, reason: 'the task {v}', canRetry: true })
      ?.text
  ).toBe("Update to 1.2.0 wasn't installed: the task {v}")
})

/** The literal reason texts of a source file: the arguments of `new Error(…)`, `super(…)`, `bad(…)`, the limits. */
function literalReasons(text: string): string[] {
  const found: string[] = []
  for (const line of text.split(/\r?\n/)) {
    const marks = [/new Error\(/, /super\(/, /\bbad\(/, /this\.(?:fetchText|request)\(/, /^const NOT_\w+ = /]
    for (const mark of marks) {
      const m = mark.exec(line)
      if (m === null) continue
      for (const s of line.slice(m.index).matchAll(/'([^']+)'/g)) found.push(s[1])
    }
  }
  return [...new Set(found)]
}

test('every literal reason of the updater has a translation', () => {
  const reasons = [...literalReasons(updaterSource), ...literalReasons(netHopSource), ...literalReasons(sharedUpdateSource)]
  // the scan itself works: it finds the texts the brief names and the ones of the release check
  expect(reasons).toContain('the checksum file is larger than 1 KB')
  expect(reasons).toContain('redirect to a host that is not allowed')
  expect(reasons).toContain('HTTP 403 (GitHub request limit?)')
  expect(reasons).toContain('aborted')
  expect(reasons).toContain('bad release tag')
  expect(reasons.length).toBeGreaterThanOrEqual(25)
  setLang('ru')
  for (const reason of reasons) expect(translateUpdateReason(reason), reason).not.toBe(reason)
})
