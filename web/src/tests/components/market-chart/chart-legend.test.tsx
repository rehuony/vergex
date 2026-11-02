import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, expect, it } from 'vitest'
import { ChartLegend } from '@/components/market-chart/chart-legend'

afterEach(cleanup)

const text = { open: 'O', high: 'H', low: 'L', close: 'C', volume: 'V' }

it('reads a candle with its change and a compact volume', () => {
  render(
    <ChartLegend
      candle={{ open: 100, high: 110, low: 90, close: 105, volume: 1250000 }}
      text={text}
    />
  )
  expect(screen.getByText('100.000')).toBeVisible()
  expect(screen.getByText('110.000')).toBeVisible()
  expect(screen.getByText('105.000')).toHaveClass('text-vergex-success')
  expect(screen.getByText('+5.00%')).toBeVisible()
  expect(screen.getByText('1.25M')).toBeVisible()
})

it('shows a falling candle in the colour of a fall', () => {
  render(
    <ChartLegend
      candle={{ open: 0.02, high: 0.02, low: 0.01, close: 0.015, volume: 9 }}
      text={text}
    />
  )
  expect(screen.getByText('0.01500')).toHaveClass('text-vergex-danger')
  expect(screen.getByText('-25.00%')).toBeVisible()
})
