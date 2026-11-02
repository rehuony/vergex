import type { FuturesState } from '@/api/futures'
import { StatStrip } from '@/components/common/figures'
import { ExecutionSchedule } from './execution-schedule'
import { PlanFunding } from './plan-funding'
import { useFuturesLabel } from './use-futures-label'

interface SelectedPlanDetailsProps {
  record: FuturesState['records'][string]
}

const money = (value: string) =>
  Number(value).toLocaleString('en-US', {
    maximumFractionDigits: Math.abs(Number(value)) < 0.01 ? 4 : 2,
  })

// SelectedPlanDetails is what the plan on the chart rests on: its reasoning, what it risks
// and how it is paid for, and its child orders. Its prices are in its row, and on the chart.
export function SelectedPlanDetails({ record }: SelectedPlanDetailsProps) {
  const label = useFuturesLabel('structures')
  const { plan } = record
  return (
    <div>
      <div className="px-4 py-3">
        <h3 className="text-[13px] font-medium text-vergex-ink">
          {plan.idea.name}
        </h3>
        {plan.idea.rationale && (
          <p className="mt-1 text-[13px] leading-relaxed text-vergex-ink-soft">
            {plan.idea.rationale}
          </p>
        )}
      </div>
      <StatStrip
        small
        className="border-t"
        stats={[
          {
            name: label('plannedLoss'),
            value: (
              <span className="text-vergex-danger">−{money(plan.risk)}</span>
            ),
          },
          {
            name: label('targetReturn'),
            value: (
              <span className="text-vergex-success">+{money(plan.reward)}</span>
            ),
          },
          {
            name: label('rewardRisk'),
            value: Number(plan.reward_risk).toFixed(2),
          },
        ]}
      />
      <PlanFunding plan={plan} flush />
      <ExecutionSchedule plan={plan} />
    </div>
  )
}
