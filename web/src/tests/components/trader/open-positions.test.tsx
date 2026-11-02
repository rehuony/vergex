import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, expect, it } from 'vitest'
import { OpenPositions } from '@/components/trader/open-positions'
import { QueryWrapper } from '@/tests/query-wrapper'
import type { Position } from '@/types'

afterEach(cleanup)

const position = {
  symbol: 'BTCUSDT',
  side: 'long',
  entry_price: 100,
  mark_price: 110,
  quantity: 1,
  unrealized_pnl: 10,
} as unknown as Position

const show = (failed: boolean) =>
  render(
    <QueryWrapper>
      <OpenPositions
        traderId="t1"
        positions={[position]}
        failed={failed}
        language="en"
      />
    </QueryWrapper>
  )

it('keeps what it last read and says it is not updating when a read fails', () => {
  show(true)
  expect(screen.getByRole('status')).toHaveTextContent('Not updating')
  expect(screen.getByText('BTC')).toBeInTheDocument()
})

it('has no mark while reads work', () => {
  show(false)
  expect(screen.queryByRole('status')).toBeNull()
})
