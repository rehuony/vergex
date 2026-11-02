import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, expect, it } from 'vitest'
import type { BacktestMetrics, BacktestReport } from '@/api/forecast'
import { BacktestReportView } from '@/components/forecast/backtest-report'

afterEach(cleanup)

const metrics = (patch: Partial<BacktestMetrics> = {}): BacktestMetrics => ({
  samples: 100,
  signals: 80,
  hits: 40,
  misses: 40,
  hit_rate: 0.5,
  chance_rate: 0.5,
  baseline_up: 0.5,
  brier: 0.25,
  brier_candles: 0.25,
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

it('shows a missing hit rate as 0', () => {
  render(
    <BacktestReportView
      report={report(metrics({ lift: -0.1, hit_rate: null }))}
      language="en"
    />
  )
  expect(screen.getAllByText('0.0%').length).toBeGreaterThan(0)
  expect(screen.queryByText('—')).toBeNull()
})

it('shows no return where no call was made', () => {
  render(
    <BacktestReportView
      report={report(metrics({ signals: 0, hit_rate: null }))}
      language="en"
    />
  )
  expect(screen.queryByText('-0.10%')).toBeNull()
})
