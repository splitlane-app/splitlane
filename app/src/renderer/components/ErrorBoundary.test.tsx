import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'
import { ErrorBoundary } from './ErrorBoundary'

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

function Broken(): React.JSX.Element {
  throw new Error('render failed')
}

test('a render error shows the text of the dictionary and a button that reloads the window', () => {
  // React reports the caught error on the console; it is expected here
  vi.spyOn(console, 'error').mockImplementation(() => {})
  const reload = vi.fn()
  render(
    <ErrorBoundary onReload={reload}>
      <Broken />
    </ErrorBoundary>
  )
  expect(screen.getByRole('heading', { name: 'Сбой окна' })).toBeTruthy()
  expect(screen.getByText('В окне произошла ошибка. Деление и служебный процесс работают как прежде')).toBeTruthy()
  expect(screen.queryByText('render failed')).toBeNull()
  fireEvent.click(screen.getByRole('button', { name: 'Перезапустить окно' }))
  expect(reload).toHaveBeenCalledTimes(1)
})

test('without an error the boundary shows its children', () => {
  render(
    <ErrorBoundary onReload={() => {}}>
      <p>board</p>
    </ErrorBoundary>
  )
  expect(screen.getByText('board')).toBeTruthy()
})
