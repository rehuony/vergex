import type { PlanningState } from '@/api/planning'
import { PlanFunding } from './plan-funding'
import { format } from './planning-labels'
import { useFrontierLabel } from './use-frontier-label'

interface SelectedPlanDetailsProps {
  record: PlanningState['records'][string]
}

// SelectedPlanDetails summarises the plan on the chart: its price levels,
// planned loss, funding and child orders.
export function SelectedPlanDetails({ record }: SelectedPlanDetailsProps) {
  const label = useFrontierLabel('selected')
  const { plan } = record
  return (
    <div className="planning-selected">
      <b>
        {label(record.state === 'draft' ? 'draft' : 'trade')} · {plan.idea.name}{' '}
        · {label(plan.idea.side === 'long' ? 'long' : 'short')}
      </b>
      <div className="planning-price-row">
        <span>
          {label(plan.idea.execution.kind === 'market' ? 'estEntry' : 'entry')}{' '}
          <strong>{format(plan.idea.entry)}</strong>
        </span>
        <span>
          {label('stopLoss')}{' '}
          <strong className="stop-text">{format(plan.idea.stop)}</strong>
        </span>
        <span>
          {label('target')}{' '}
          <strong className="target-text">{format(plan.idea.target)}</strong>
        </span>
        <span>
          {label('plannedLoss')}{' '}
          <strong>
            {format(plan.risk)} {plan.snapshot.quote_asset}
          </strong>
        </span>
      </div>
      <PlanFunding plan={plan} />
      <details>
        <summary>
          {label('inspect', { count: plan.orders.length || 1 })}
        </summary>
        <table>
          <thead>
            <tr>
              <th>{label('childOrder')}</th>
              <th>
                {label(
                  plan.idea.execution.kind === 'market'
                    ? 'executionEstimate'
                    : 'limitPrice'
                )}
              </th>
              <th>{label('quantity')}</th>
              <th>{label('releaseTime')}</th>
            </tr>
          </thead>
          <tbody>
            {plan.orders.map((o) => (
              <tr key={o.index}>
                <td>{o.index}</td>
                <td>{format(o.price)}</td>
                <td>
                  {format(o.quantity)} {plan.snapshot.base_asset}
                </td>
                <td>
                  {o.offset_seconds === 0
                    ? label('atStart')
                    : label('secondsLater', { count: o.offset_seconds })}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        <p>{plan.idea.rationale}</p>
      </details>
    </div>
  )
}
