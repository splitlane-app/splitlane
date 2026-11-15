import { expect, test } from 'vitest'
import type { UpdateState } from '../../shared/update'
import { updateBannerModel, updateStatusLine } from './update'

const N = 'https://github.com/splitlane-app/splitlane/releases/tag/v1.2.0'

const rows: [string, UpdateState, { text: string; actions: string[]; tone: string } | null, string | null][] = [
  ['idle', { kind: 'idle' }, null, null],
  ['checking', { kind: 'checking', manual: true }, null, 'Проверка обновлений…'],
  ['upToDate', { kind: 'upToDate' }, null, 'Установлена последняя версия'],
  ['checkFailed', { kind: 'checkFailed', reason: 'HTTP 403' }, null, 'Не удалось проверить обновления: HTTP 403'],
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
    { kind: 'downloadFailed', version: '1.2.0', notesUrl: N, reason: 'HTTP 500' },
    { text: 'Не удалось скачать версию 1.2.0: HTTP 500', actions: ['retry', 'dismiss'], tone: 'warn' },
    'Не удалось скачать версию 1.2.0: HTTP 500'
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
  ]
]

test('updateBannerModel for every state', () => {
  for (const [name, state, banner] of rows) expect(updateBannerModel(state), name).toEqual(banner)
})

test('updateStatusLine for every state', () => {
  for (const [name, state, , line] of rows) expect(updateStatusLine(state), name).toBe(line)
})
