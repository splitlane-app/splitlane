import { describe, expect, test } from 'vitest'
import { parseVpsAddress, validVpsUser } from './vps'

const long = (n: number): string => 'a'.repeat(n)

describe('parseVpsAddress', () => {
  const ok: [string, string, number][] = [
    ['198.51.100.7', '198.51.100.7', 22],
    ['  198.51.100.7\t', '198.51.100.7', 22],
    ['198.51.100.7:2222', '198.51.100.7', 2222],
    ['2001:db8::7', '2001:db8::7', 22],
    ['[2001:db8::7]', '2001:db8::7', 22],
    [' [2001:db8::7] ', '2001:db8::7', 22],
    ['[2001:db8::7]:2200', '2001:db8::7', 2200],
    ['::ffff:198.51.100.7', '::ffff:198.51.100.7', 22],
    ['::1', '::1', 22],
    ['2001:db8:1:2:3:4:5:6', '2001:db8:1:2:3:4:5:6', 22],
    ['vps.example.com', 'vps.example.com', 22],
    ['  vps.example.com  ', 'vps.example.com', 22],
    ['vps.example.com:65535', 'vps.example.com', 65535],
    ['VPS-1.Example.com:1', 'VPS-1.Example.com', 1],
    ['myvps', 'myvps', 22],
    [`${long(63)}.example.com`, `${long(63)}.example.com`, 22]
  ]
  test.each(ok)('%j is %s port %i', (input, host, port) => {
    expect(parseVpsAddress(input)).toEqual({ host, port })
  })

  const bad = [
    '', '   ', 'http://vps.example.com', 'https://198.51.100.7/', 'a b', '[198.51.100.7]', '[vps.example.com]', '[2001:db8::7',
    'fe80::1%eth0', '[fe80::1%25eth0]', '198.51.100.300', '198.51.100', '010.1.1.1', '12345', '-vps.example.com',
    'vps-.example.com', 'vps..example.com', '.example.com', 'vps.example.com.', `${long(64)}.example.com`,
    `${'abcdefghi.'.repeat(26)}com`, 'vps_1.example.com', 'пример.рф', 'root@vps.example.com', 'vps.example.com/x',
    'vps.example.com:', 'vps.example.com:0', 'vps.example.com:65536', 'vps.example.com:ssh', 'vps.example.com:22:22',
    '[2001:db8::7]:0', '[2001:db8::7]:', '[2001:db8::7]22', '2001:db8::7::1', '1:2:3:4:5:6:7:8:9', '1:2:3:4:5:6:7::8', ':::'
  ]
  test.each(bad)('%j is refused', (input) => {
    expect(parseVpsAddress(input)).toBeNull()
  })
})

describe('validVpsUser', () => {
  test('1..32 characters of [a-z_][a-z0-9_-]*', () => {
    for (const u of ['root', 'ubuntu', 'admin-1', '_svc', 'a', long(32)]) expect(validVpsUser(u), u).toBe(true)
    for (const u of ['', ' root', 'root ', 'Root', '1abc', '-a', 'a b', 'a.b', 'a@b', 'пользователь', long(33), 'root\n']) {
      expect(validVpsUser(u), JSON.stringify(u)).toBe(false)
    }
  })
})
