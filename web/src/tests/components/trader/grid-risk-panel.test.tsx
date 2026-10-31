import { fireEvent, render, screen } from '@testing-library/react'
import { SWRConfig } from 'swr'
import { expect, it, vi } from 'vitest'
import { GridRiskPanel } from '@/components/trader/grid-risk-panel'

vi.mock('@/api', () => ({
  api: {
    getGridRiskInfo: vi.fn().mockResolvedValue({
      current_leverage: 5,
      effective_leverage: 2.4,
      recommended_leverage: 3,
      current_position: 480,
      max_position: 1000,
      position_percent: 48,
      liquidation_price: 64210.5,
      liquidation_distance: 19.8,
      regime_level: 'volatile',
      short_box_upper: 81200,
      short_box_lower: 79400,
      mid_box_upper: 82500,
      mid_box_lower: 78000,
      long_box_upper: 85000,
      long_box_lower: 75000,
      current_price: 80190.2,
      breakout_level: 'short',
      breakout_direction: 'up',
    }),
  },
}))

it('summarises grid risk and expands into the details', async () => {
  render(
    <SWRConfig value={{ provider: () => new Map() }}>
      <GridRiskPanel traderId="grid-1" language="en" />
    </SWRConfig>
  )
  const toggle = await screen.findByRole('button', { name: /Grid Risk/ })
  expect(toggle).toHaveAttribute('aria-expanded', 'false')
  expect(toggle).toHaveTextContent('Volatile')
  expect(screen.queryByText('Liquidation')).toBeNull()

  fireEvent.click(toggle)
  expect(toggle).toHaveAttribute('aria-expanded', 'true')
  expect(screen.getByText('Liquidation')).toBeInTheDocument()
  expect(screen.getByText('64,210.50')).toBeInTheDocument()
})
