import { expired, type Plan, type PlanningState } from '@/api/planning'
import { Button } from '@/components/ui/button'
import { useLanguage } from '@/contexts/language-context'
import { format, planStateLabel } from './planning-labels'
import { useFrontierLabel } from './use-frontier-label'

// PlanCard is one selectable plan in the Frontier plan list.
export function PlanCard({
  record,
  selected,
  compareIndex,
  clock,
  onChoose,
}: {
  record: PlanningState['records'][string]
  selected: boolean
  compareIndex: number
  clock: number
  onChoose: (plan: Plan) => void
}) {
  const { language } = useLanguage()
  const label = useFrontierLabel('card')
  return (
    <Button
      type="button"
      variant="option"
      aria-pressed={selected}
      className="planning-card h-auto min-h-[174px] w-[230px] flex-[0_0_230px] flex-col items-stretch justify-start gap-3 whitespace-normal rounded-[10px] p-3.5 text-left text-[13px] font-normal text-vergex-text aria-pressed:shadow-[inset_0_3px_0_var(--brand)]"
      onClick={() => onChoose(record.plan)}
    >
      <span
        className={
          record.plan.idea.side === 'long' ? 'target-text' : 'stop-text'
        }
      >
        {compareIndex >= 0
          ? String.fromCharCode(65 + compareIndex) + ' · '
          : ''}
        {label(record.plan.idea.side === 'long' ? 'long' : 'short')} ·{' '}
        {label(
          record.plan.idea.execution.kind === 'market'
            ? 'market'
            : record.plan.idea.execution.kind === 'ladder'
              ? 'ladder'
              : record.plan.idea.execution.kind === 'twap'
                ? 'twap'
                : 'limit'
        )}
      </span>
      <strong>{record.plan.idea.name}</strong>
      <span>{label('entry', { price: format(record.plan.idea.entry) })}</span>
      <div>
        <span>
          {label('plannedLoss')} <b>{format(record.plan.risk)}</b>
        </span>
        <span>
          {label('rewardRisk')}{' '}
          <b>{Number(record.plan.reward_risk).toFixed(2)} : 1</b>
        </span>
      </div>
      <small>
        {expired(record.plan, clock) && record.state === 'draft'
          ? label('expired')
          : planStateLabel(record.state, language)}
      </small>
    </Button>
  )
}
