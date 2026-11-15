import { describe, expect, it } from 'vitest'
import { resolveLanguage } from './i18n'

describe('resolveLanguage', () => {
  it.each([
    [['ru-RU'], 'ru'],
    [['ru'], 'ru'],
    [['RU-ru'], 'ru'],
    [['en-US'], 'en'],
    [['uk-UA'], 'en'],
    [['be-BY'], 'en'],
    [['de-DE'], 'en'],
    [[], 'en'],
    [['en-US', 'ru-RU'], 'en'],
    [['ru-RU', 'en-US'], 'ru']
  ])('system %j gives %s', (system, lang) => {
    expect(resolveLanguage('system', system)).toBe(lang)
  })

  it('an explicit setting wins over the system', () => {
    expect(resolveLanguage('ru', ['en-US'])).toBe('ru')
    expect(resolveLanguage('en', ['ru-RU'])).toBe('en')
    expect(resolveLanguage('en', [])).toBe('en')
  })
})
