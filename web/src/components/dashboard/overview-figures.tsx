import type { ReactNode } from 'react'
import type { Overview } from '@/api/dashboard'
import { StatStrip, type Stat } from '@/components/common/figures'
import { Skeleton } from '@/components/ui/skeleton'
import { fmtUsd } from '@/utils/trader-format'
import { equityText } from './flow-members'

type Label = (key: string, params?: Record<string, string | number>) => string

// What the page answers first: how the money stands and whether anything needs the user.
// Until the overview is read the names stand with skeletons where the figures will be.
export function OverviewFigures({
  overview,
  attention,
  label,
}: {
  overview: Overview | undefined
  /** Always the last cell, as a problem must not move the rest. */
  attention: ReactNode
  label: Label
}) {
  const today = overview?.realized_today
  const running = overview?.traders.filter((row) => row.running).length
  const reading = <Skeleton className="w-20" />
  const stats: Stat[] = [
    {
      name: label('figures.equity'),
      value: overview ? equityText(overview) : reading,
    },
    {
      name: label('figures.today'),
      value: overview ? fmtUsd(today ?? undefined, true) : reading,
      gain: today,
    },
    {
      name: label('figures.running'),
      value: overview ? `${running} / ${overview.traders.length}` : reading,
    },
    {
      name: label('figures.positions'),
      value: overview ? overview.positions.length : reading,
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
