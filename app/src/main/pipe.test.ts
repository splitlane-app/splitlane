import net from 'node:net'
import { afterEach, expect, test } from 'vitest'
import { DialError, classifyDialError, dialPipe } from './pipe'

const name = String.raw`\\.\pipe\SplitLane.WindowTest.` + process.pid
let server: net.Server | undefined
afterEach(async () => {
  const s = server
  server = undefined
  if (s) await new Promise<void>((r) => s.close(() => r()))
})

test('dials a pipe and exchanges a line', async () => {
  server = net.createServer((sock) => {
    sock.on('data', (d) => sock.write(Buffer.concat([Buffer.from('echo:'), d])))
  })
  await new Promise<void>((r) => server!.listen(name, r))
  const conn = await dialPipe(name)
  const got = new Promise<string>((res) => {
    let acc = ''
    conn.onData((c) => {
      acc += c.toString('utf8')
      if (acc.endsWith('\n')) res(acc)
    })
  })
  const closed = new Promise<void>((res) => conn.onClose(() => res()))
  conn.write('hi\n')
  expect(await got).toBe('echo:hi\n')
  conn.end()
  await closed
})

test('reports absent for a missing pipe', async () => {
  const e = await dialPipe(name + '.missing', 1000).catch((x: unknown) => x)
  expect(e).toBeInstanceOf(DialError)
  expect((e as DialError).failure).toBe('absent')
  expect((e as DialError).code).toBe('ENOENT')
})

test('classifyDialError maps codes', () => {
  expect(classifyDialError('ENOENT')).toBe('absent')
  expect(classifyDialError('EACCES')).toBe('denied')
  expect(classifyDialError('EPERM')).toBe('denied')
  expect(classifyDialError('EBUSY')).toBe('other')
  expect(classifyDialError(undefined)).toBe('other')
})
