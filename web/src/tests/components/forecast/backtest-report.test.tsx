import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, expect, it } from 'vitest'
import type { BacktestMetrics, BacktestReport } from '@/api/forecast'
import {
  BacktestReportView,
  hasEdge,
  hitRateText,
} from '@/components/forecast/backtest-report'

afterEach(cleanup)

const metrics = (patch: Partial<BacktestMetrics> = {}): BacktestMetrics => ({
  samples: 100,
  signals: 80,
  hits: 40,
  misses: 40,
  hit_rate: 0.5,
  baseline_up: 0.5,
  brier: 0.25,
  baseline_brier: 0.25,
  lift: 0,
  avg_return_pct: 0,
  avg_return_after_fee_pct: -0.1,
  move_coverage: 0.8,
  derivative_coverage: 0,
  ...patch,
})

const report = (overall: BacktestMetrics): BacktestReport => ({
  fee_pct: 0.1,
  overall,
  by_interval: { '1h': overall },
  by_regime: { trending: overall },
  by_symbol: { BTCUSDT: overall },
  reliability: [
    { from: 0.6, to: 0.7, samples: 10, avg_prob: 0.65, up_rate: 0.5 },
  ],
  first_time: 1,
  last_time: 2,
})

it('only claims an edge that beats the base rate and pays after fees', () => {
  expect(hasEdge(metrics({ lift: 0.02, avg_return_after_fee_pct: 0.1 }))).toBe(
    true
  )
  expect(hasEdge(metrics({ lift: 0.02, avg_return_after_fee_pct: -0.1 }))).toBe(
    false
  )
  expect(hasEdge(metrics({ lift: -0.02, avg_return_after_fee_pct: 0.1 }))).toBe(
    false
  )
})

it('leads with the verdict and shows a missing hit rate as a dash', () => {
  render(
    <BacktestReportView
      id={3}
      report={report(metrics({ lift: -0.1 }))}
      language="en"
    />
  )
  expect(screen.getByText(/^No edge/)).toBeVisible()
  expect(screen.getByText('Run 3')).toBeVisible()
  expect(hitRateText(metrics({ hit_rate: null }))).toBe('—')
})
