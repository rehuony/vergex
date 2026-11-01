import { ArrowRight } from 'lucide-react'
import { t, type Language } from '@/i18n'
import type { DecisionRecord, Position } from '@/types'
import { cn } from '@/utils/cn'
import { Panel, PanelState } from '@/components/common/panel'
import { actionSide, tradeActions, type ActionSide } from './decision-format'
import { baseLabel } from '@/utils/trader-format'

interface Item {
  symbol: string
  side: ActionSide
}

const sideClass: Record<ActionSide, string> = {
  long: 'text-vergex-success',
  short: 'text-vergex-danger',
  flat: 'text-vergex-text-muted',
}

function Layer({ title, items }: { title: string; items: Item[] }) {
  return (
    <div className="min-w-0 flex-1 rounded-lg border border-[var(--line)] p-3">
      <div className="flex items-baseline justify-between gap-2">
        <span className="text-xs text-vergex-text-muted">{title}</span>
        <span className="text-base font-semibold tabular-nums text-vergex-text">
          {items.length}
        </span>
      </div>
      <ul className="mt-2 flex max-h-24 flex-wrap gap-x-2 gap-y-1 overflow-hidden text-xs font-medium">
        {items.map((item) => (
          <li key={item.symbol} className={sideClass[item.side]}>
            {item.symbol}
          </li>
        ))}
      </ul>
    </div>
  )
}

// DecisionFunnel follows the latest cycle: the coins the agent judged, the orders
// that went through, and what the trader holds now.
export function DecisionFunnel({
  latest,
  positions,
  language,
}: {
  latest?: DecisionRecord
  positions?: Position[]
  language: Language
}) {
  const label = (key: string) => t(`traderDetail.${key}`, language)
  if (!latest) {
    return (
      <Panel title={label('latestCycle')}>
        <PanelState role="status">{label('noDecisions')}</PanelState>
      </Panel>
    )
  }
  const decided = new Map<string, ActionSide>()
  for (const action of latest.decisions ?? []) {
    decided.set(baseLabel(action.symbol), actionSide(action.action))
  }
  const candidates: Item[] = [
    ...new Set((latest.candidate_coins ?? []).map(baseLabel)),
  ].map((symbol) => ({ symbol, side: decided.get(symbol) ?? 'flat' }))
  const executed: Item[] = tradeActions(latest)
    .filter((action) => action.success)
    .map((action) => ({
      symbol: baseLabel(action.symbol),
      side: actionSide(action.action),
    }))
  const held: Item[] = (positions ?? []).map((position) => ({
    symbol: baseLabel(position.symbol),
    side: /short/i.test(position.side) ? 'short' : 'long',
  }))
  const arrow = cn(
    'size-4 shrink-0 self-center text-vergex-text-muted max-sm:rotate-90'
  )
  return (
    <Panel
      title={label('latestCycle')}
      meta={t('traderDetail.cycleNumber', language, {
        cycle: latest.cycle_number,
      })}
    >
      <div className="flex gap-2 px-4 pb-4 max-sm:flex-col">
        <Layer title={label('candidates')} items={candidates} />
        <ArrowRight className={arrow} aria-hidden="true" />
        <Layer title={label('executed')} items={executed} />
        <ArrowRight className={arrow} aria-hidden="true" />
        <Layer title={label('holding')} items={held} />
      </div>
    </Panel>
  )
}
