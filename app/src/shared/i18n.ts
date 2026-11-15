/** The languages of the window. */
export type Lang = 'ru' | 'en'

/** The setting: a language, or the one of Windows. */
export type LanguageSetting = 'system' | Lang

export const LANGUAGE_SETTINGS: readonly LanguageSetting[] = ['system', 'ru', 'en']

export function isLanguageSetting(x: unknown): x is LanguageSetting {
  return x === 'system' || x === 'ru' || x === 'en'
}

/**
 * The language to show: the setting when it names one; otherwise the first language of Windows: Russian when it
 * starts with `ru` (any case), English in every other case, also when the list is empty.
 */
export function resolveLanguage(setting: LanguageSetting, system: readonly string[]): Lang {
  if (setting !== 'system') return setting
  const first = system[0]
  return typeof first === 'string' && first.toLowerCase().startsWith('ru') ? 'ru' : 'en'
}
