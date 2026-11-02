import type { Plan } from '@/api/futures'
import {
  cellClass,
  headClass,
  numberClass,
  tableClass,
} from '@/components/common/figures'
import { cn } from '@/utils/cn'
import { useFuturesLabel } from './use-futures-label'

const number = (value: string) =>
  Number(value).toLocaleString('en-US', { maximumFractionDigits: 8 })

// ExecutionSchedule lists a plan's child orders, where it has more than one: when each is
// released or at what price, and how much of the plan each carries.
export function ExecutionSchedule({ plan }: { plan: Plan }) {
  const label = useFuturesLabel('schedule')
  const kind = plan.idea.execution.kind
  if (kind === 'market' || kind === 'limit' || !plan.orders.length) return null
  const timed = kind === 'twap'
  const total = Number(plan.quantity)
  return (
    <section aria-label={label(timed ? 'twapName' : 'ladderName')}>
      <table className={tableClass}>
        <thead>
          <tr>
            <th className={cn(headClass, 'font-medium')} colSpan={3}>
              <span className="text-vergex-ink">
                {label(timed ? 'twapTitle' : 'ladderTitle')}
              </span>{' '}
              · {label('preview')}
            </th>
          </tr>
        </thead>
        <tbody>
          {plan.orders.map((order) => (
            <tr key={order.index}>
              <td className={cn(cellClass, 'mono text-xs text-vergex-ink')}>
                {timed
                  ? `T+${order.offset_seconds}s`
                  : label('limit', { price: number(order.price) })}
              </td>
              <td className={cn(cellClass, numberClass)}>
                {number(order.quantity)} {plan.snapshot.base_asset}
                <span className="ml-1 text-vergex-muted">
                  {total > 0
                    ? ((Number(order.quantity) / total) * 100).toFixed(0)
                    : '0'}
                  %
                </span>
              </td>
              <td
                className={cn(
                  cellClass,
                  'w-28 text-right text-xs text-vergex-muted'
                )}
              >
                {timed
                  ? label('limit', { price: number(order.price) })
                  : label('together')}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
      <p className="px-4 py-2 text-[11px] text-vergex-muted">
        {label(timed ? 'twapNote' : 'ladderNote')}
      </p>
    </section>
  )
}
