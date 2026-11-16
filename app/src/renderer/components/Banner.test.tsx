import { act, cleanup, fireEvent, screen } from '@testing-library/react'
import { afterEach, expect, test } from 'vitest'
import type { Snapshot } from '../../shared/ipc'
import { fakeBridge, renderWith, servicesFor, snap } from '../testing'
import { Banner } from './Banner'
import { Notice } from './Notice'

afterEach(cleanup)

const click = async (name: string): Promise<void> => {
  await act(async () => {
    fireEvent.click(screen.getByRole('button', { name }))
  })
}

test('Banner renders actions and calls the bridge', async () => {
  let saved = 0
  let retried = 0
  const { bridge, commands } = fakeBridge({
    saveDiagnostics: async () => {
      saved++
      return { saved: true }
    },
    retryLink: async () => {
      retried++
    }
  })
  const services = servicesFor(bridge, snap({ phase: 'off' }))
  const show = (s: Snapshot): void => act(() => services.store.getState().setSnapshot(s))
  renderWith(<Banner />, services)

  expect(screen.getByText('Выключено. Программы ходят как обычно, без деления')).toBeTruthy()
  await click('Включить')
  expect(commands.at(-1)).toEqual({ method: 'routing.enable' })

  show(snap({ phase: 'starting' }))
  await click('Отменить')
  expect(commands.at(-1)).toEqual({ method: 'routing.disable' })

  // degraded: «Повторить» is the extra check of the link
  show(snap({ phase: 'degraded', reason: { code: 'vpn_unreachable', params: {} } }))
  expect(screen.getByText(/^VPN-сервер node-a недоступен/)).toBeTruthy()
  await click('Повторить')
  expect(commands.at(-1)).toEqual({ method: 'routing.enable' })
  await click('Всё напрямую')
  expect(commands.at(-1)).toEqual({ method: 'routing.allDirect', on: true })

  show(snap({ phase: 'on', allDirect: true }))
  await click('Вернуть обычный режим')
  expect(commands.at(-1)).toEqual({ method: 'routing.allDirect', on: false })

  show(snap({ phase: 'degraded', reason: { code: 'login_expired', params: {} } }))
  await click('Выключить деление и войти')
  expect(commands.at(-1)).toEqual({ method: 'tailscale.login' })

  show(snap({ phase: 'blocked' }))
  await click('Выключить его')
  expect(commands.at(-1)).toEqual({ method: 'systemTailscale.clearExitNode' })
  await click('Выключить деление')
  expect(commands.at(-1)).toEqual({ method: 'routing.disable' })

  // stopping with restore_pending: «Повторить» retries the restore
  show(snap({ phase: 'stopping', reason: { code: 'restore_pending', params: {} } }))
  await click('Повторить')
  expect(commands.at(-1)).toEqual({ method: 'routing.disable' })
  await click('Выключить без возврата')
  expect(commands.at(-1)).toEqual({ method: 'routing.disable', skipRestore: true })

  show(snap({ phase: 'error', reason: { code: 'no_exit_node', params: {} } }))
  expect(screen.getByText('Не удалось включить: exit node не выбран')).toBeTruthy()
  await click('Повторить')
  expect(commands.at(-1)).toEqual({ method: 'routing.enable' })
  await click('Сохранить журнал')
  expect(saved).toBe(1)

  show({ ...snap({ phase: 'on' }), link: { kind: 'unresponsive' } })
  expect(screen.getByText('Служебный процесс не отвечает')).toBeTruthy()
  await click('Повторить')
  expect(retried).toBe(1)

  expect(commands).toHaveLength(11)
})

test('Banner shows the restore command under its hint', () => {
  const { bridge } = fakeBridge()
  const command = 'tailscale set --exit-node=100.64.0.2'
  const services = servicesFor(
    bridge,
    snap({ phase: 'stopping', reason: { code: 'restore_pending', params: { error: 'access denied', command } } })
  )
  renderWith(<Banner />, services)

  const hint = screen.getByText('Выполните в командной строке:')
  const code = screen.getByText(command)
  expect(hint.compareDocumentPosition(code) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  expect(screen.getByText('access denied')).toBeTruthy()
})

test('no banner in on', () => {
  const { bridge } = fakeBridge()
  const { container } = renderWith(<Banner />, servicesFor(bridge, snap({ phase: 'on' })))
  expect(container.textContent).toBe('')
})

test('a failed banner command shows the notice', async () => {
  const { bridge } = fakeBridge({}, { ok: false, error: { code: 'busy', message: 'busy' } })
  const services = servicesFor(bridge, snap({ phase: 'off' }))
  renderWith(
    <>
      <Banner />
      <Notice />
    </>,
    services
  )
  await click('Включить')
  expect(screen.getByText('Служебный процесс занят. Повторите через несколько секунд')).toBeTruthy()

  fireEvent.click(screen.getByRole('button', { name: 'Закрыть' }))
  expect(screen.queryByText('Служебный процесс занят. Повторите через несколько секунд')).toBeNull()
})

test('the degraded banner names the same server as the lane: the exit node remembered while the list is empty', () => {
  const base = snap({ phase: 'degraded', reason: { code: 'vpn_unreachable', params: {} } })
  const st = base.state!
  const s: Snapshot = {
    ...base,
    state: { ...st, tailscale: { ...st.tailscale, exitNodes: [] } },
    settings: { ...base.settings, lastExitNode: { id: '100.64.0.2', name: 'node-a' } }
  }
  renderWith(<Banner />, servicesFor(fakeBridge().bridge, s))
  expect(screen.getByText(/^VPN-сервер node-a недоступен/)).toBeTruthy()
})
