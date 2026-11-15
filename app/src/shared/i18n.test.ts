import { describe, expect, it } from 'vitest'
import { langFromSearch, resolveLanguage } from './i18n'

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
    [['kk-KZ', 'ru-RU'], 'ru'],
    [['de-DE', 'en-US', 'ru-RU'], 'en'],
    [['de-DE', 'ru-RU', 'en-US'], 'ru'],
    [['rue'], 'en'],
    [['ruby-RU'], 'en'],
    [['english'], 'en'],
    [['EN'], 'en'],
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

describe('langFromSearch', () => {
  it.each([
    ['?lang=ru', 'ru'],
    ['?lang=en', 'en'],
    ['lang=en&x=1', 'en'],
    ['?lang=de', null],
    ['?lang=RU', null],
    ['?lang=', null],
    ['?x=1', null],
    ['', null]
  ])('%j gives %s', (search, lang) => {
    expect(langFromSearch(search)).toBe(lang)
  })
})
