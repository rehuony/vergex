import type { Plan } from '@/api/planning'
import { useLanguage } from '@/contexts/language-context'
import { t } from '@/i18n/translations'
import type { PlanBatch } from '@/utils/planning/frontier-workspace'
import { PlanStructures } from './plan-structures'
import type { PlanCheck } from '@/utils/planning/frontier-plans'

interface FrontierPlanDeckProps {
  batch: PlanBatch | null
  plans: Plan[]
  checks: PlanCheck[]
  planningBusy: boolean
  waitingForEntry: boolean
  selected: string
  clock: number
  connected: boolean
  reviewDisabled: boolean
  onChoose: (plan: Plan) => void
  onReview: (plan: Plan) => void
}

// FrontierPlanDeck shows the latest generation of entry plans and the checks
// the assistant ran for each structure.
export function FrontierPlanDeck({
  batch,
  plans,
  checks,
  planningBusy,
  waitingForEntry,
  selected,
  clock,
  connected,
  reviewDisabled,
  onChoose,
  onReview,
}: FrontierPlanDeckProps) {
  const { language } = useLanguage()
  const label = (key: string, params?: Record<string, string | number>) =>
    t(`frontier.${key}`, language, params)
  const entryChecks =
    checks.length || planningBusy
      ? checks
      : (batch?.deferred || []).map((entry) => ({
          id: `deferred_${entry.kind}`,
          kind: entry.kind,
          name: label('waitConditions'),
          status: 'deferred',
          reason: entry.reason,
        }))
  return (
    <section
      className="frontier-plan-deck"
      id="frontier-plans"
      aria-label={label('entryPlans')}
    >
      <div className="frontier-deck-heading">
        <div>
          <h2>
            {waitingForEntry
              ? label('waitingForEntry')
              : batch
                ? label('latestPlans')
                : plans.length
                  ? label('savedIdea')
                  : label('chooseEntry')}
          </h2>
          <p>
            {waitingForEntry
              ? label('waitingHint')
              : batch
                ? label('generatedAt', {
                    time: new Date(batch.created).toLocaleString('en-US', {
                      month: 'short',
                      day: 'numeric',
                      hour: '2-digit',
                      minute: '2-digit',
                    }),
                  })
                : label('reviewHint')}
          </p>
        </div>
        <span>
          {label('planCount').replace('{count}', String(plans.length))}
        </span>
      </div>
      <PlanStructures
        checks={entryChecks}
        showAllTypes={Boolean(batch)}
        plans={plans}
        busy={planningBusy}
        selected={selected}
        onSelect={onChoose}
        onReview={onReview}
        reviewDisabled={reviewDisabled}
        connected={connected}
        now={clock}
      />
      {!plans.length &&
        !planningBusy &&
        !entryChecks.length &&
        !waitingForEntry && (
          <p className="frontier-deck-empty">{label('noPlans')}</p>
        )}
    </section>
  )
}
