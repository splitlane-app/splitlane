import { afterEach, describe, expect, it } from 'vitest'
import { setLang } from '../i18n/t'
import { formatSpeed, maskAddress } from './format'

afterEach(() => setLang('ru'))

describe('format', () => {
  it('formatSpeed covers bytes, KB and MB with a comma', () => {
    expect(formatSpeed(0)).toBe('')
    expect(formatSpeed(-5)).toBe('')
    expect(formatSpeed(512)).toBe('512 Б/с')
    expect(formatSpeed(1023.6)).toBe('1024 Б/с')
    expect(formatSpeed(1024)).toBe('1,0 КБ/с')
    expect(formatSpeed(1536)).toBe('1,5 КБ/с')
    expect(formatSpeed(1024 * 1024)).toBe('1,0 МБ/с')
    expect(formatSpeed(5.25 * 1024 * 1024)).toBe('5,3 МБ/с')
  })

  it('formatSpeed per language: a point and English units in English, a comma in Russian', () => {
    setLang('en')
    expect(formatSpeed(0)).toBe('')
    expect(formatSpeed(512)).toBe('512 B/s')
    expect(formatSpeed(1536)).toBe('1.5 KB/s')
    expect(formatSpeed(1.5 * 1024 * 1024)).toBe('1.5 MB/s')
    expect(formatSpeed(5.25 * 1024 * 1024)).toBe('5.3 MB/s')
    setLang('ru')
    expect(formatSpeed(1.5 * 1024 * 1024)).toBe('1,5 МБ/с')
  })

  it('maskAddress hides the last two numbers', () => {
    expect(maskAddress('203.0.113.7')).toBe('203.0.•.•')
    expect(maskAddress(null)).toBe('')
    expect(maskAddress('2001:db8::1')).toBe('2001:•')
  })
})
