import { describe, expect, test } from 'vitest'
import { LineDecoder, encodeRequest, parseLine } from './wire'

describe('encodeRequest', () => {
  test('encodes a request without params', () => {
    const s = encodeRequest(1, 'state.get')
    expect(s.endsWith('\n')).toBe(true)
    const o = JSON.parse(s)
    expect(o).toEqual({ id: 1, method: 'state.get' })
    expect('params' in o).toBe(false)
  })
  test('encodes a request with params', () => {
    expect(JSON.parse(encodeRequest(7, 'hello', { protocol: 1 }))).toEqual({ id: 7, method: 'hello', params: { protocol: 1 } })
  })
})

test('parses result, error with rules and protocol, event, invalid', () => {
  expect(parseLine('{"id":3,"result":{"a":1}}')).toEqual({ kind: 'result', id: 3, result: { a: 1 } })
  expect(parseLine('{"id":4,"error":{"code":"rule_rejected","message":"m","rules":[{"id":"x","reason":"duplicate"}]}}')).toEqual({
    kind: 'error',
    id: 4,
    error: { code: 'rule_rejected', message: 'm', rules: [{ id: 'x', reason: 'duplicate' }] }
  })
  expect(parseLine('{"id":null,"error":{"code":"protocol_mismatch","message":"m","protocol":1}}')).toEqual({
    kind: 'error',
    id: null,
    error: { code: 'protocol_mismatch', message: 'm', protocol: 1 }
  })
  expect(parseLine('{"event":"activity","data":{"x":2}}')).toEqual({ kind: 'event', event: 'activity', data: { x: 2 } })
  expect(parseLine('not json')).toEqual({ kind: 'invalid', line: 'not json' })
  expect(parseLine('{"foo":1}')).toEqual({ kind: 'invalid', line: '{"foo":1}' })
  expect(parseLine('[1]').kind).toBe('invalid')
})

describe('LineDecoder', () => {
  test('joins a long line split into many chunks', () => {
    const line = 'x'.repeat(2 * 1024 * 1024)
    const bytes = new TextEncoder().encode(line + '\n')
    const d = new LineDecoder()
    const out: string[] = []
    for (let i = 0; i < bytes.length; i += 7) out.push(...d.push(bytes.subarray(i, i + 7)))
    expect(out).toHaveLength(1)
    expect(out[0].length).toBe(line.length)
  })
  test('keeps a multibyte character split across chunks', () => {
    const bytes = new TextEncoder().encode('Программы\n')
    const d = new LineDecoder()
    expect(d.push(bytes.subarray(0, 5))).toEqual([])
    expect(d.push(bytes.subarray(5))).toEqual(['Программы'])
  })
  test('skips empty lines and strips CR', () => {
    const d = new LineDecoder()
    expect(d.push('a\r\n\n\r\nb')).toEqual(['a'])
    expect(d.push('c\r\n')).toEqual(['bc'])
  })
})
