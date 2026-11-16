import { afterEach, describe, expect, it } from 'vitest'
import type { VpsEvent, VpsOutcome } from '../../shared/ipc'
import { en } from '../i18n/en'
import { ru } from '../i18n/ru'
import { setLang } from '../i18n/t'
import {
  elapsedText,
  EMPTY_FORM,
  formErrors,
  newRun,
  retryFocus,
  runEnd,
  runEvent,
  stepView,
  VPS_ERROR_CODES,
  VPS_STEPS,
  vpsErrorText,
  vpsNotice,
  vpsRequest,
  warningTexts,
  type VpsForm,
  type VpsRun
} from './vps'

afterEach(() => setLang('ru'))

const form = (over: Partial<VpsForm> = {}): VpsForm => ({ ...EMPTY_FORM, password: 'made-up', ...over })

describe('the form', () => {
  it('starts with root as the login and the password way in', () => {
    expect(EMPTY_FORM).toEqual({ address: '', user: 'root', useKey: false, password: '', keyName: null, passphrase: '' })
  })

  // Review Focus 3: the forms people type, and the garbage
  it.each([
    ['198.51.100.7'],
    ['198.51.100.7:2222'],
    ['[2001:db8::7]:22'],
    ['2001:db8::7'],
    ['vps.example.com'],
    ['vps.example.com:2222'],
    ['  203.0.113.10  ']
  ])('takes the address %j', (address) => {
    expect(formErrors(form({ address })).address).toBeUndefined()
  })

  it.each([
    ['', 'vps.check.addressEmpty'],
    ['   ', 'vps.check.addressEmpty'],
    ['http://203.0.113.10', 'vps.check.address'],
    ['a b', 'vps.check.address'],
    ['203.0.113.10:0', 'vps.check.address'],
    ['203.0.113.10:70000', 'vps.check.address'],
    ['198.51.100.300', 'vps.check.address'],
    ['[2001:db8::7', 'vps.check.address']
  ])('refuses the address %j', (address, key) => {
    expect(formErrors(form({ address })).address).toBe(key)
  })

  it.each([['root'], [' root '], ['ubuntu'], ['deploy-1'], ['_svc'], ['a'.repeat(32)]])('takes the login %j', (user) => {
    expect(formErrors(form({ address: '203.0.113.10', user }))).toEqual({})
  })

  it.each([
    ['', 'vps.check.userEmpty'],
    ['  ', 'vps.check.userEmpty'],
    ['Root', 'vps.check.user'],
    ['1user', 'vps.check.user'],
    ['a b', 'vps.check.user'],
    ['a'.repeat(33), 'vps.check.user'],
    ['админ', 'vps.check.user']
  ])('refuses the login %j', (user, key) => {
    expect(formErrors(form({ address: '203.0.113.10', user })).user).toBe(key)
  })

  it('wants a password of at most 1024 characters, as the main process does', () => {
    const ok = { address: '203.0.113.10' }
    expect(formErrors(form({ ...ok, password: '' }))).toEqual({ password: 'vps.check.passwordEmpty' })
    expect(formErrors(form({ ...ok, password: 'x'.repeat(1024) }))).toEqual({})
    // a long password is a password error, never the one of the address and the login
    expect(formErrors(form({ ...ok, password: 'x'.repeat(1025) }))).toEqual({ password: 'vps.check.passwordLong' })
    // spaces are part of a password
    expect(formErrors(form({ ...ok, password: ' ' }))).toEqual({})
  })

  it('wants a key file for the key, and a passphrase of at most 1024 characters', () => {
    const key = { address: '203.0.113.10', useKey: true, password: '' }
    expect(formErrors(form(key))).toEqual({ key: 'vps.check.key' })
    expect(formErrors(form({ ...key, keyName: 'id_ed25519' }))).toEqual({})
    expect(formErrors(form({ ...key, keyName: 'id_ed25519', passphrase: 'x'.repeat(1024) }))).toEqual({})
    expect(formErrors(form({ ...key, keyName: 'id_ed25519', passphrase: 'x'.repeat(1025) }))).toEqual({
      passphrase: 'vps.check.passphraseLong'
    })
  })

  it('the request: the address and the login trimmed, only the secret of the way in', () => {
    const secret = `p'a"s$ s\\w!рд`
    expect(vpsRequest(form({ address: ' 203.0.113.10:2222 ', user: ' root ', password: secret }))).toEqual({
      address: '203.0.113.10:2222',
      user: 'root',
      password: secret,
      useKey: false
    })
    const key = form({ address: 'vps.example.com', useKey: true, password: secret, keyName: 'id_ed25519' })
    expect(vpsRequest(key)).toEqual({ address: 'vps.example.com', user: 'root', useKey: true })
    expect(vpsRequest({ ...key, passphrase: 'made-up phrase' })).toEqual({
      address: 'vps.example.com',
      user: 'root',
      useKey: true,
      keyPassphrase: 'made-up phrase'
    })
  })
})

describe('the steps', () => {
  const views = (run: VpsRun): string[] => VPS_STEPS.map((s) => stepView(run.steps[s]))
  const feed = (run: VpsRun, events: VpsEvent[]): VpsRun => events.reduce(runEvent, run)

  it('are six, in order, all waiting at the start', () => {
    expect(VPS_STEPS).toEqual(['connect', 'system', 'install', 'harden', 'subscription', 'add'])
    const run = newRun(1000, false)
    expect(run).toMatchObject({ startedAt: 1000, endedAt: null, outcome: null, useKey: false })
    expect(views(run)).toEqual(['waiting', 'waiting', 'waiting', 'waiting', 'waiting', 'waiting'])
  })

  it('go from waiting to running to done, skipped or not worked', () => {
    let run = feed(newRun(0, false), [
      { step: 'connect', state: 'start' },
      { step: 'connect', state: 'done' },
      { step: 'system', state: 'start' }
    ])
    expect(views(run)).toEqual(['done', 'running', 'waiting', 'waiting', 'waiting', 'waiting'])
    run = feed(run, [
      { step: 'system', state: 'done' },
      { step: 'install', state: 'start' },
      { step: 'install', state: 'done' },
      { step: 'harden', state: 'start' },
      { step: 'harden', state: 'warn', detail: 'hardening failed' },
      { step: 'subscription', state: 'skip' }
    ])
    expect(views(run)).toEqual(['done', 'done', 'done', 'warn', 'skip', 'waiting'])
  })

  it('the install has three parts: it runs until the last one ends, and a skipped part does not hide a done one', () => {
    let run = feed(newRun(0, false), [
      { step: 'install', state: 'start', detail: 'sing-box' },
      { step: 'install', state: 'skip', detail: 'sing-box 1.14.4 is there' },
      { step: 'install', state: 'start', detail: 'config' }
    ])
    expect(stepView(run.steps.install)).toBe('running')
    run = feed(run, [
      { step: 'install', state: 'done' },
      { step: 'install', state: 'start', detail: 'service' }
    ])
    expect(stepView(run.steps.install)).toBe('running')
    run = runEvent(run, { step: 'install', state: 'skip' })
    expect(stepView(run.steps.install)).toBe('done')
  })

  it('a failed step stays failed; an unknown step or state changes nothing', () => {
    const run = feed(newRun(0, false), [
      { step: 'connect', state: 'start' },
      { step: 'connect', state: 'failed', detail: 'ssh_auth' }
    ])
    expect(views(run)[0]).toBe('failed')
    expect(runEvent(run, { step: 'reboot', state: 'start' } as unknown as VpsEvent)).toBe(run)
    expect(runEvent(run, { step: 'system', state: 'paused' } as unknown as VpsEvent)).toBe(run)
  })

  it('the end closes the step still running: done, cancelled or failed', () => {
    const running = feed(newRun(0, false), [
      { step: 'connect', state: 'start' },
      { step: 'connect', state: 'done' },
      { step: 'install', state: 'start' }
    ])
    const ended = (o: VpsOutcome): string[] => views(runEnd(running, o, 5000))
    expect(ended({ kind: 'done', servers: 2, subscriptionId: null, warnings: [] })[2]).toBe('done')
    expect(ended({ kind: 'cancelled' })[2]).toBe('cancelled')
    expect(ended({ kind: 'failed', code: 'kit_failed' })[2]).toBe('failed')
    // what had ended stays as it was, what never started stays waiting
    expect(ended({ kind: 'cancelled' })).toEqual(['done', 'waiting', 'cancelled', 'waiting', 'waiting', 'waiting'])
    const end = runEnd(running, { kind: 'cancelled' }, 5000)
    expect(end).toMatchObject({ endedAt: 5000, outcome: { kind: 'cancelled' } })
  })

  it('the elapsed time reads as minutes and seconds', () => {
    expect(elapsedText(0)).toBe('0:00')
    expect(elapsedText(-20)).toBe('0:00')
    expect(elapsedText(999)).toBe('0:00')
    expect(elapsedText(65_000)).toBe('1:05')
    expect(elapsedText(10 * 60_000 + 59_999)).toBe('10:59')
  })
})

describe('the texts of the end', () => {
  const failed = (code: string, detail?: string): VpsOutcome =>
    detail === undefined ? { kind: 'failed', code } : { kind: 'failed', code, detail }

  it('every code of the setup and of the window has a text of its own, in both languages', () => {
    expect([...VPS_ERROR_CODES].sort()).toEqual(
      [
        'add_failed',
        'bad_input',
        'busy',
        'cancelled',
        'connection_lost',
        'download_failed',
        'kit_failed',
        'no_admin',
        'os_unsupported',
        'password_expired',
        'ports_busy',
        'routing_on',
        'ssh_auth',
        'ssh_key',
        'ssh_unreachable',
        'timeout'
      ].sort()
    )
    for (const lang of ['ru', 'en'] as const) {
      setLang(lang)
      const texts = VPS_ERROR_CODES.map((c) => vpsErrorText(c, undefined, false))
      for (const [i, text] of texts.entries()) {
        expect(text.trim().length, VPS_ERROR_CODES[i]).toBeGreaterThan(0)
        expect(text, VPS_ERROR_CODES[i]).not.toContain(VPS_ERROR_CODES[i])
      }
      expect(new Set(texts).size).toBe(texts.length)
    }
  })

  it('the spec wording of the codes', () => {
    expect(vpsErrorText('bad_input', 'address', false)).toBe('Проверьте адрес сервера и логин')
    expect(vpsErrorText('ssh_auth', undefined, false)).toBe('Неверный логин или пароль')
    expect(vpsErrorText('ssh_unreachable', 'timeout', false)).toMatch(
      /^Не удалось подключиться к серверу: проверьте адрес; порт SSH может быть закрыт/
    )
    expect(vpsErrorText('no_admin', 'sudo_tty', false)).toMatch(/^У этого пользователя на сервере нет прав администратора/)
    expect(vpsErrorText('os_unsupported', 'debian 11', false)).toMatch(/^Система сервера не подходит: нужна Debian 12\+ или Ubuntu 22\.04\+/)
    expect(vpsErrorText('download_failed', undefined, false)).toBe('Сервер не смог скачать sing-box: проверьте его доступ в интернет')
    expect(vpsErrorText('ports_busy', undefined, false)).toBe('На сервере заняты все подходящие порты')
    expect(vpsErrorText('cancelled', undefined, false)).toBe('Настройка отменена')
    expect(vpsErrorText('connection_lost', undefined, false)).toBe(
      'Связь с сервером прервалась; запустите настройку снова — она продолжит с того же места'
    )
    expect(vpsErrorText('password_expired', undefined, false)).toBe(
      'Хостинг требует сменить пароль при первом входе: войдите один раз через его консоль или по SSH, смените пароль и запустите настройку снова'
    )
    expect(vpsErrorText('routing_on', undefined, false)).toBe('Выключите деление на время настройки')
  })

  it('the details that change the text', () => {
    // the key: a PuTTY key tells how to export it, a passphrase and a format say so
    expect(vpsErrorText('ssh_key', 'ppk', true)).toMatch(/^Этот файл ключа не подходит: это ключ PuTTY \(\.ppk\)\. Откройте его в PuTTYgen/)
    expect(vpsErrorText('ssh_key', 'ppk', true)).toContain('Export OpenSSH key')
    expect(vpsErrorText('ssh_key', 'passphrase', true)).toBe('Этот файл ключа не подходит: пароль ключа не введён или неверен')
    expect(vpsErrorText('ssh_key', 'format', true)).toBe('Этот файл ключа не подходит: это не закрытый ключ OpenSSH или PEM')
    expect(vpsErrorText('ssh_key', 'other', true)).toBe('Этот файл ключа не подходит')
    // the login by a key is refused as a key
    expect(vpsErrorText('ssh_auth', undefined, true)).toBe('Сервер не принял логин или ключ')
    // a refused request says which part
    expect(vpsErrorText('bad_input', 'password', false)).toBe('Проверьте пароль')
    expect(vpsErrorText('bad_input', 'passphrase', true)).toBe('Проверьте пароль ключа')
    expect(vpsErrorText('bad_input', 'key', true)).toBe('Выберите файл ключа ещё раз')
    // the kit's own message follows its text; the window that cannot start the setup says what to do
    expect(vpsErrorText('kit_failed', 'sing-box service did not start', false)).toBe(
      'Установка не удалась: sing-box service did not start'
    )
    expect(vpsErrorText('kit_failed', undefined, false)).toBe('Установка не удалась')
    expect(vpsErrorText('kit_failed', 'spawn', false)).toBe(
      'Не удалось запустить настройку на этом компьютере. Переустановите SplitLane'
    )
    // the helper did not take the result: the reason of its code
    expect(vpsErrorText('add_failed', 'too_many', false)).toBe(
      'Сервер настроен, но SplitLane не смог его добавить. Слишком много серверов или подписок: можно до 500 серверов и 20 подписок'
    )
    expect(vpsErrorText('add_failed', 'apply_failed', false)).toBe(
      'Сервер настроен, но SplitLane не смог его добавить. Не удалось прочитать или сохранить список серверов'
    )
    expect(vpsErrorText('add_failed', 'no_servers', false)).toBe(
      'Сервер настроен, но SplitLane не смог его добавить. Попробуйте снова'
    )
    // a code of a newer window
    expect(vpsErrorText('quota', undefined, false)).toBe('Настроить сервер не удалось (код quota)')
    expect(vpsErrorText('unknown', undefined, false)).toBe('Настроить сервер не удалось (код unknown)')

    setLang('en')
    expect(vpsErrorText('ssh_key', 'ppk', true)).toMatch(/^This key file can't be used: it's a PuTTY key \(\.ppk\)/)
    expect(vpsErrorText('add_failed', 'too_many', false)).toBe(
      "The server is set up, but SplitLane couldn't add it. Too many servers or subscriptions: the limit is 500 servers and 20 subscriptions"
    )
    expect(vpsErrorText('kit_failed', 'mktemp: no space', false)).toBe('The installation failed: mktemp: no space')
  })

  it('the warnings: one line for the phone (its first reason), one for the protection', () => {
    expect(warningTexts([])).toEqual([])
    expect(warningTexts(['harden_failed'])).toEqual(['Защиту сервера поставить не удалось. VPN работает и без неё'])
    for (const w of ['sub_unreachable', 'sub_port80_busy', 'sub_skipped']) {
      const [line] = warningTexts([w])
      expect(line, w).toMatch(/^Телефон подключить не получилось: /)
    }
    expect(warningTexts(['sub_port80_busy'])[0]).toContain('порт 80')
    expect(warningTexts(['sub_skipped'])[0]).toContain('Ubuntu 22.04')
    const both = warningTexts(['sub_skipped', 'harden_failed', 'sub_unreachable', 'new_warning'])
    expect(both).toHaveLength(2)
    expect(both[0]).toMatch(/^Защиту сервера/)
    expect(both[1]).toBe(warningTexts(['sub_skipped'])[0])
    setLang('en')
    expect(warningTexts(['sub_unreachable'])[0]).toMatch(/^Couldn't connect a phone: /)
  })

  it('the notice of an end that no screen showed', () => {
    expect(vpsNotice({ kind: 'done', servers: 2, subscriptionId: 'sub-0123456789abcdef', warnings: [] }, false)).toBe(
      'Готово. Добавлено серверов: 2'
    )
    expect(vpsNotice({ kind: 'cancelled' }, false)).toBe('Настройка отменена')
    expect(vpsNotice(failed('ssh_auth'), true)).toBe('Сервер не принял логин или ключ')
  })

  it('the field a retry starts on', () => {
    expect(retryFocus(failed('ssh_auth'), false)).toBe('password')
    expect(retryFocus(failed('ssh_auth'), true)).toBe('key')
    expect(retryFocus(failed('password_expired'), false)).toBe('password')
    expect(retryFocus(failed('ssh_unreachable', 'name'), false)).toBe('address')
    expect(retryFocus(failed('os_unsupported'), false)).toBe('address')
    expect(retryFocus(failed('ssh_key', 'passphrase'), true)).toBe('passphrase')
    expect(retryFocus(failed('ssh_key', 'ppk'), true)).toBe('key')
    expect(retryFocus(failed('no_admin', 'sudo_denied'), false)).toBe('user')
    expect(retryFocus(failed('bad_input', 'user'), false)).toBe('user')
    expect(retryFocus(failed('bad_input', 'key'), true)).toBe('key')
    expect(retryFocus(failed('timeout'), false)).toBeNull()
    expect(retryFocus({ kind: 'cancelled' }, false)).toBeNull()
  })

  it('every text key of the steps and their states exists in both dictionaries', () => {
    for (const step of VPS_STEPS) {
      expect(ru[`vps.step.${step}`]).toBeTruthy()
      expect(en[`vps.step.${step}`]).toBeTruthy()
    }
  })
})
