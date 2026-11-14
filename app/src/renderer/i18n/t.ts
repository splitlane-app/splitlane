import { ru, type Key } from './ru'

/** Looks a text up and fills `{name}` placeholders; a missing parameter is a programming error. */
export function t(key: Key, params?: Record<string, string | number>): string {
  return ru[key].replace(/\{(\w+)\}/g, (_m, name: string) => {
    const value = params?.[name]
    if (value === undefined) throw new Error(`t(${key}): missing parameter "${name}"`)
    return String(value)
  })
}
