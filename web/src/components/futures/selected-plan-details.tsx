import { X } from 'lucide-react'
import type { FuturesState } from '@/api/futures'
import { StatStrip } from '@/components/common/figures'
import { Button } from '@/components/ui/button'
import { ExecutionSchedule } from './execution-schedule'
import { PlanFunding } from './plan-funding'
import { useFuturesLabel } from './use-futures-label'

interface SelectedPlanDetailsProps {
  record: FuturesState['records'][string]
  onClose: () => void
}

const money = (value: string) =>
  Number(value).toLocaleString('en-US', {
    maximumFractionDigits: Math.abs(Number(value)) < 0.01 ? 4 : 2,
  })

// SelectedPlanDetails is what the plan on the chart rests on: its reasoning, what it risks
// and how it is paid for, and its child orders. Its prices are in its row, and on the chart.
export function SelectedPlanDetails({
  record,
  onClose,
}: SelectedPlanDetailsProps) {
  const label = useFuturesLabel('structures')
  const { plan } = record
  return (
    <div>
      <div className="px-4 py-3">
        <div className="flex items-start gap-2">
          <h3 className="min-w-0 flex-1 text-[13px] font-medium text-vergex-ink">
            {plan.idea.name}
          </h3>
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className="-mr-2 -mt-1.5 size-8 max-sm:size-10"
            aria-label={label('closePlan')}
            title={label('closePlan')}
            onClick={onClose}
          >
            <X className="size-4" aria-hidden="true" />
          </Button>
        </div>
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
