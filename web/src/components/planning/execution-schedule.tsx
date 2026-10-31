import type { Plan } from '@/api/planning'
import { useFrontierLabel } from './use-frontier-label'

const number = (value: string) =>
  Number(value).toLocaleString('en-US', { maximumFractionDigits: 8 })
export function ExecutionSchedule({ plan }: { plan: Plan }) {
  const label = useFrontierLabel('schedule')
  const kind = plan.idea.execution.kind
  if (kind === 'market' || kind === 'limit' || !plan.orders.length) return null
  const timed = kind === 'twap'
  const total = Number(plan.quantity)
  return (
    <section
      className="planning-execution-band"
      aria-label={label(timed ? 'twapName' : 'ladderName')}
    >
      <header>
        <div>
          <strong>{label(timed ? 'twapTitle' : 'ladderTitle')}</strong>
          <span>
            {label(timed ? 'twapMeta' : 'ladderMeta', {
              count: plan.orders.length,
            })}
          </span>
        </div>
        <small>{label('preview')}</small>
      </header>
      <ol className="planning-execution-steps">
        {plan.orders.map((order) => (
          <li key={order.index}>
            <span className="planning-step-dot">{order.index}</span>
            <div>
              <b>
                {timed
                  ? `T+${order.offset_seconds}s`
                  : label('limit', { price: number(order.price) })}
              </b>
              <span>
                {number(order.quantity)} {plan.snapshot.base_asset}{' '}
                <small>
                  ·{' '}
                  {total > 0
                    ? ((Number(order.quantity) / total) * 100).toFixed(0)
                    : '0'}
                  %
                </small>
              </span>
              <small>
                {timed
                  ? label('limit', { price: number(order.price) })
                  : label('together')}
              </small>
            </div>
          </li>
        ))}
      </ol>
      <p>{label(timed ? 'twapNote' : 'ladderNote')}</p>
    </section>
  )
}
