import { describe, expect, it } from 'vitest'
import { formatSpeed, maskAddress } from './format'

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

  it('maskAddress hides the last two numbers', () => {
    expect(maskAddress('203.0.113.7')).toBe('203.0.•.•')
    expect(maskAddress(null)).toBe('')
    expect(maskAddress('2001:db8::1')).toBe('2001:•')
  })
})
