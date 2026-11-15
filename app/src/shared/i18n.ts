/** The languages of the window. */
export type Lang = 'ru' | 'en'

/** The setting: a language, or the one of Windows. */
export type LanguageSetting = 'system' | Lang

export function isLanguageSetting(x: unknown): x is LanguageSetting {
  return x === 'system' || x === 'ru' || x === 'en'
}

/** The language of one entry of the preferred list of Windows: `ru`, `ru-…`, `en`, `en-…` (any case), else null. */
function languageOfEntry(entry: unknown): Lang | null {
  if (typeof entry !== 'string') return null
  const tag = entry.toLowerCase()
  if (tag === 'ru' || tag.startsWith('ru-')) return 'ru'
  if (tag === 'en' || tag.startsWith('en-')) return 'en'
  return null
}

/**
 * The language to show: the setting when it names one; otherwise the first entry of the preferred list of Windows
 * that is Russian or English decides, English when there is none (also when the list is empty).
 */
export function resolveLanguage(setting: LanguageSetting, system: readonly string[]): Lang {
  if (setting !== 'system') return setting
  for (const entry of system) {
    const lang = languageOfEntry(entry)
    if (lang !== null) return lang
  }
  return 'en'
}

/** The language the main process puts in the page URL (`?lang=ru|en`): null for a missing or any other value. */
export function langFromSearch(search: string): Lang | null {
  const value = new URLSearchParams(search).get('lang')
  return value === 'ru' || value === 'en' ? value : null
}
