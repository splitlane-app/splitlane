import type { Lang } from '../../shared/i18n'
import { en } from './en'
import { ru, type Key } from './ru'

const DICTIONARIES: Record<Lang, Record<Key, string>> = { ru, en }

/** The language of the texts: guessed at start (main.tsx), then the one of every snapshot (the store sets it). */
let current: Lang = 'ru'

export function setLang(lang: Lang): void {
  current = lang
}

export function currentLang(): Lang {
  return current
}

/** Whether a text of this key exists (a code from a newer helper may have none). */
export function isKey(key: string): key is Key {
  return Object.hasOwn(ru, key)
}

/** Looks a text of the current language up and fills `{name}` placeholders; a missing parameter is a programming error. */
export function t(key: Key, params?: Record<string, string | number>): string {
  return DICTIONARIES[current][key].replace(/\{(\w+)\}/g, (_m, name: string) => {
    const value = params?.[name]
    if (value === undefined) throw new Error(`t(${key}): missing parameter "${name}"`)
    return String(value)
  })
}
