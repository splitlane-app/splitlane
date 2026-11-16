import { act, cleanup, fireEvent, screen, within } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'
import type { HelperState } from '../../shared/protocol'
import { fakeBridge, proxyServer, proxyState, renderWith, servicesFor, snap, state } from '../testing'
import { ServerList } from './ServerList'

afterEach(cleanup)

const SUB = { id: 'sub-0123456789abcdef', name: 'sub.example.com', updatedAt: '', count: 1, error: null }
const A = proxyServer('0123456789abcdef', 'edge-a.example.com')
const B = proxyServer('fedcba9876543210', 'edge-b.example.com', { protocol: 'hysteria2', source: SUB.id })
const C = proxyServer('00000000000000c0', 'home', { protocol: 'wireguard', source: 'file' })

const radio = (name: RegExp): HTMLInputElement => screen.getByRole('radio', { name }) as HTMLInputElement

function setup(s: HelperState, removable = false, answer?: Parameters<typeof fakeBridge>[1]) {
  const { bridge, commands } = fakeBridge({}, answer)
  const services = servicesFor(bridge, { ...snap(), state: s })
  const view = renderWith(<ServerList state={s} removable={removable} />, services)
  return { commands, services, view }
}

test('each server shows its name, protocol and source; the one in force is checked', () => {
  setup(proxyState([A, B, C], B.id, [SUB], { phase: 'off' }))
  // a fieldset named by its legend: the native radios group by their name
  const group = screen.getByRole('group', { name: 'Серверы' })
  expect(group.tagName).toBe('FIELDSET')
  expect(screen.queryByRole('radiogroup')).toBeNull()
  const labels = within(group)
    .getAllByRole('radio')
    .map((r) => r.closest('label')?.textContent)
  expect(labels).toEqual(['edge-a.example.comVLESS · вручную', 'edge-b.example.comHysteria2 · sub.example.com', 'homeWireGuard · файл'])
  expect(radio(/edge-b/).checked).toBe(true)
  expect(radio(/edge-a/).checked).toBe(false)
})

test('with Tailscale in force no server is checked, and choosing the selected one switches to it', async () => {
  const ts = state({ phase: 'off', proxy: { servers: [A, B], selected: A.id, subscriptions: [] } })
  const { commands } = setup(ts)
  expect(radio(/edge-a/).checked).toBe(false)
  expect(radio(/edge-b/).checked).toBe(false)
  fireEvent.click(radio(/edge-a/))
  await vi.waitFor(() => expect(commands).toEqual([{ method: 'proxy.select', id: A.id }]))
})

test('off routing: a choice is sent at once; the one in force sends nothing', async () => {
  const { commands } = setup(proxyState([A, B], A.id, [], { phase: 'off' }))
  fireEvent.click(radio(/edge-a/))
  expect(commands).toEqual([])
  fireEvent.click(radio(/edge-b/))
  await vi.waitFor(() => expect(commands).toEqual([{ method: 'proxy.select', id: B.id }]))
  expect(screen.queryByText('Смена сервера ненадолго перезапускает деление')).toBeNull()
})

test('live routing: quick changes send only the last one after the pause, and the note says it restarts', async () => {
  const { commands } = setup(proxyState([A, B, C], A.id, [], { phase: 'on' }))
  const group = screen.getByRole('group', { name: 'Серверы' })
  const note = screen.getByText('Смена сервера ненадолго перезапускает деление')
  expect(group.compareDocumentPosition(note) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  vi.useFakeTimers()
  try {
    fireEvent.click(radio(/edge-b/))
    await act(async () => {
      vi.advanceTimersByTime(300)
    })
    fireEvent.click(radio(/home/))
    expect(radio(/home/).checked).toBe(true)
    await act(async () => {
      vi.advanceTimersByTime(499)
    })
    expect(commands).toEqual([])
    await act(async () => {
      vi.advanceTimersByTime(1)
    })
    expect(commands).toEqual([{ method: 'proxy.select', id: C.id }])
  } finally {
    vi.useRealTimers()
  }
})

test('a choice still waiting is applied when the list goes away', async () => {
  const { commands, view } = setup(proxyState([A, B], A.id, [], { phase: 'on' }))
  vi.useFakeTimers()
  try {
    fireEvent.click(radio(/edge-b/))
    expect(commands).toEqual([])
    await act(async () => {
      view.unmount()
    })
    expect(commands).toEqual([{ method: 'proxy.select', id: B.id }])
  } finally {
    vi.useRealTimers()
  }
})

test('remove: at once for any server, but not the selected one while live routing runs on it', async () => {
  const { commands } = setup(proxyState([A, B], A.id, [], { phase: 'on' }), true)
  const removeA = screen.getByRole('button', { name: 'Удалить edge-a.example.com' }) as HTMLButtonElement
  const removeB = screen.getByRole('button', { name: 'Удалить edge-b.example.com' }) as HTMLButtonElement
  expect(removeA.disabled).toBe(true)
  expect(removeA.title).toBe('Выбранный сервер можно удалить, когда деление выключено')
  expect(screen.getByText('Выбранный сервер можно удалить, когда деление выключено')).toBeTruthy()
  expect(removeB.disabled).toBe(false)
  expect(removeB.textContent).toBe('Удалить')
  await act(async () => {
    fireEvent.click(removeB)
  })
  expect(commands).toEqual([{ method: 'proxy.remove', id: B.id }])
})

test('remove of the selected server is free while routing is off, or while Tailscale is in force', () => {
  for (const s of [
    proxyState([A, B], A.id, [], { phase: 'off' }),
    proxyState([A, B], A.id, [], { phase: 'error' }),
    state({ phase: 'on', proxy: { servers: [A, B], selected: A.id, subscriptions: [] } })
  ]) {
    setup(s, true)
    expect((screen.getByRole('button', { name: 'Удалить edge-a.example.com' }) as HTMLButtonElement).disabled).toBe(false)
    expect(screen.queryByText('Выбранный сервер можно удалить, когда деление выключено')).toBeNull()
    cleanup()
  }
})

test('a refused remove becomes a notice', async () => {
  const { services } = setup(proxyState([A, B], A.id, [], { phase: 'off' }), true, {
    ok: false,
    error: { code: 'unknown_server', message: 'no server has this id' }
  })
  await act(async () => {
    fireEvent.click(screen.getByRole('button', { name: 'Удалить edge-b.example.com' }))
  })
  expect(services.store.getState().notice).toBe('Такого сервера или подписки уже нет в списке')
})
