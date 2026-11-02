import type { Plan } from '@/api/futures'
import { StatStrip } from '@/components/common/figures'
import { cn } from '@/utils/cn'
import { useFuturesLabel } from './use-futures-label'

const money = (value: number) =>
  value.toLocaleString('en-US', {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  })

export function planNotional(plan: Plan) {
  return plan.orders?.length
    ? plan.orders.reduce(
        (sum, order) => sum + Number(order.price) * Number(order.quantity),
        0
      )
    : Number(plan.idea.entry) * Number(plan.quantity)
}

// PlanFunding is how a plan is paid for: margin, times leverage, is the position.
export function PlanFunding({ plan, flush }: { plan: Plan; flush?: boolean }) {
  const label = useFuturesLabel('funding')
  const notional = planNotional(plan)
  const leverage = plan.constraints.leverage || 1
  const margin = notional / leverage
  const quote = plan.snapshot.quote_asset
  const available = Number(plan.constraints.available_margin)
  return (
    <section aria-label={label('name')}>
      <StatStrip
        small
        className={flush ? undefined : 'overflow-hidden rounded-md border'}
        stats={[
          { name: label('estMargin'), value: money(margin), note: quote },
          { name: label('leverage'), value: `${leverage}x` },
          { name: label('positionValue'), value: money(notional), note: quote },
        ]}
      />
      {/* Only what changes how to read the figures above. */}
      {(plan.constraints.planning_only ||
        plan.sizing_limit === 'liquidity' ||
        plan.sizing_limit === 'risk' ||
        (Number.isFinite(available) && available > 0)) && (
        <div
          className={cn(
            'space-y-1 text-xs text-vergex-muted',
            flush ? 'px-4 py-2' : 'mt-2'
          )}
        >
          {plan.constraints.planning_only ? (
            <p>{label('previewOnly')}</p>
          ) : (
            Number.isFinite(available) &&
            available > 0 && (
              <p>
                {label('availableAtSizing', {
                  available: money(available),
                  quote,
                  percent: money((margin / available) * 100),
                })}
              </p>
            )
          )}
          {plan.sizing_limit === 'liquidity' && (
            <p className="text-vergex-danger">{label('oldCap')}</p>
          )}
          {plan.sizing_limit === 'risk' && <p>{label('riskReduced')}</p>}
        </div>
      )}
    </section>
  )
}
