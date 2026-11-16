import type { Lang } from '../../shared/i18n'
import { LIMITS } from '../../shared/protocol'
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

/**
 * The numbers the texts of the helper's limits name (`error.too_many`, `error.too_long`, …), from the shared limits;
 * every text of a code gets them, the ones without such a placeholder ignore them.
 */
export const LIMIT_PARAMS = {
  servers: LIMITS.maxServers,
  subscriptions: LIMITS.maxSubscriptions,
  kb: LIMITS.maxProxyTextBytes / 1024,
  mb: LIMITS.maxSubscriptionBytes / (1024 * 1024),
  seconds: LIMITS.fetchTimeoutSeconds
} as const

/** Looks a text of the current language up and fills `{name}` placeholders; a missing parameter is a programming error. */
export function t(key: Key, params?: Record<string, string | number>): string {
  return DICTIONARIES[current][key].replace(/\{(\w+)\}/g, (_m, name: string) => {
    const value = params?.[name]
    if (value === undefined) throw new Error(`t(${key}): missing parameter "${name}"`)
    return String(value)
  })
}
