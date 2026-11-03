import { t, type Language } from '@/i18n'
import type { DecisionRecord, Position } from '@/types'
import { actionSide, tradeActions, type ActionSide } from './decision-format'
import { baseLabel } from '@/utils/trader-format'

interface Item {
  symbol: string
  side: ActionSide
}

const sideClass: Record<ActionSide, string> = {
  long: 'text-vergex-success',
  short: 'text-vergex-danger',
  flat: 'text-vergex-muted',
}

function Layer({ title, items }: { title: string; items: Item[] }) {
  return (
    <div className="min-w-0 bg-vergex-surface px-4 py-3">
      <dt className="text-xs text-vergex-muted">{title}</dt>
      <dd className="mono mt-1 text-2xl leading-[1.2] tracking-[-0.02em] text-vergex-ink">
        {items.length}
      </dd>
      <dd>
        <ul className="mt-1 flex max-h-10 flex-wrap gap-x-2 overflow-hidden text-xs leading-5">
          {items.map((item) => (
            <li key={item.symbol} className={sideClass[item.side]}>
              {item.symbol}
            </li>
          ))}
        </ul>
      </dd>
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
  if (!latest) return null
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
  return (
    <section
      aria-label={`${label('latestCycle')} · ${t('traderDetail.cycleNumber', language, { cycle: latest.seq })}`}
    >
      <dl className="grid grid-cols-3 gap-px border-b border-vergex-line bg-vergex-line max-sm:grid-cols-1">
        <Layer title={label('candidates')} items={candidates} />
        <Layer title={label('executed')} items={executed} />
        <Layer title={label('holding')} items={held} />
      </dl>
    </section>
  )
}
