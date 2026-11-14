import { render, screen } from '@testing-library/react'
import { expect, test } from 'vitest'
import { App } from './App'

test('shows the product title', () => {
  render(<App />)
  expect(screen.getByText('SplitLane')).toBeTruthy()
})
