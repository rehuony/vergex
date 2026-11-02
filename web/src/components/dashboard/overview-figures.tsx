import type { ReactNode } from 'react'
import type { Overview } from '@/api/dashboard'
import { StatStrip, type Stat } from '@/components/common/figures'
import { fmtUsd } from '@/utils/trader-format'
import { equityText } from './flow-members'

type Label = (key: string, params?: Record<string, string | number>) => string

// What the page answers first: how the money stands and whether anything needs the user.
export function OverviewFigures({
  overview,
  attention,
  label,
}: {
  overview: Overview
  /** Always the last cell, as a problem must not move the rest. */
  attention: ReactNode
  label: Label
}) {
  const today = overview.realized_today
  const running = overview.traders.filter((row) => row.is_running).length
  const stats: Stat[] = [
    {
      name: label('figures.equity'),
      value: equityText(overview),
    },
    {
      name: label('figures.today'),
      value: fmtUsd(today ?? undefined, true),
      gain: today,
    },
    {
      name: label('figures.running'),
      value: `${running} / ${overview.traders.length}`,
    },
    {
      name: label('figures.positions'),
      value: overview.positions.length,
    },
  ]
  return (
    <div className="grid shrink-0 grid-cols-1 border-b border-vergex-line sm:grid-cols-[minmax(0,1fr)_auto]">
      <StatStrip stats={stats} className="border-b-0 max-sm:border-b" />
      <div className="flex items-center sm:min-w-44 sm:border-l sm:border-vergex-line">
        {attention}
      </div>
    </div>
  )
}
