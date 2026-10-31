import type { Plan } from '@/api/planning'
import { useFrontierLabel } from './use-frontier-label'

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

export function PlanFunding({
  plan,
  compact = false,
}: {
  plan: Plan
  compact?: boolean
}) {
  const label = useFrontierLabel('funding')
  const notional = planNotional(plan)
  const leverage = plan.constraints.leverage || 1
  const margin = notional / leverage
  const quote = plan.snapshot.quote_asset
  const available = Number(plan.constraints.available_margin)
  if (compact)
    return (
      <div className="frontier-funding-compact">
        <span>
          {label('positionValue')} <b>{leverage}x</b>
        </span>
        <strong>
          {money(notional)} <small>{quote}</small>
        </strong>
        <small>
          {label('estMargin')} {money(margin)} {quote}
        </small>
        {plan.constraints.planning_only && (
          <small>{label('equityPreview')}</small>
        )}
      </div>
    )
  return (
    <section className="frontier-funding" aria-label={label('name')}>
      <div className="frontier-funding-equation">
        <div>
          <span>{label('estMargin')}</span>
          <strong>
            {money(margin)} <small>{quote}</small>
          </strong>
        </div>
        <span aria-hidden="true">×</span>
        <div>
          <span>{label('leverage')}</span>
          <strong>{leverage}x</strong>
        </div>
        <span aria-hidden="true">=</span>
        <div>
          <span>{label('positionValue')}</span>
          <strong>
            {money(notional)} <small>{quote}</small>
          </strong>
        </div>
      </div>
      {plan.constraints.planning_only ? (
        <p>{label('previewOnly')}</p>
      ) : Number.isFinite(available) && available > 0 ? (
        <p>
          {label('availableAtSizing', {
            available: money(available),
            quote,
            percent: money((margin / available) * 100),
          })}
        </p>
      ) : (
        <p>{label('savedSize')}</p>
      )}
      {plan.sizing_limit === 'liquidity' && (
        <p className="stop-text">{label('oldCap')}</p>
      )}
      {plan.sizing_limit === 'risk' && <p>{label('riskReduced')}</p>}
    </section>
  )
}
