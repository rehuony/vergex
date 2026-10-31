import type { Plan } from '@/api/planning'
import type { PlanCheck } from '@/utils/planning/frontier-plans'
import { validationSummary } from '@/utils/planning/planning-presentation'
import { useLanguage } from '@/contexts/language-context'
import { structureName } from './planning-activity'
import { useFrontierLabel } from './use-frontier-label'
import { PlanFunding } from './plan-funding'
import { Button } from '@/components/ui/button'
export function PlanStructures({
  checks,
  plans,
  busy,
  selected,
  onSelect,
  onReview,
  reviewDisabled = false,
  connected = true,
  now = 0,
  showAllTypes = true,
}: {
  checks: PlanCheck[]
  plans: Plan[]
  busy: boolean
  selected: string
  onSelect: (plan: Plan) => void
  onReview?: (plan: Plan) => void
  reviewDisabled?: boolean
  connected?: boolean
  showAllTypes?: boolean
  now?: number
}) {
  const { language } = useLanguage()
  const label = useFrontierLabel('structures')
  const price = (value: string) =>
    Number(value).toLocaleString('en-US', { maximumFractionDigits: 8 })
  const money = (value: string) =>
    Number(value).toLocaleString('en-US', {
      maximumFractionDigits: Math.abs(Number(value)) < 0.01 ? 4 : 2,
    })
  if (!checks.length && !plans.length && !busy) return null
  return (
    <section className="planning-structures" aria-label={label('name')}>
      {['market', 'limit', 'ladder', 'twap']
        .filter(
          (kind) =>
            showAllTypes ||
            checks.some((check) => check.kind === kind) ||
            plans.some((plan) => plan.idea.execution.kind === kind)
        )
        .map((kind) => {
          const plan = plans.find((p) => p.idea.execution.kind === kind)
          const stale = plan ? Date.parse(plan.expires) <= now : false
          const check = checks.filter((row) => row.kind === kind).slice(-1)[0]
          return (
            <article
              key={kind}
              data-empty={!plan}
              className={plan?.id === selected ? 'selected' : ''}
            >
              <header>
                <strong>{structureName(kind, language)}</strong>
                <span
                  className={
                    plan
                      ? stale
                        ? 'frontier-draft-stale'
                        : 'target-text'
                      : check?.status === 'rejected'
                        ? 'stop-text'
                        : ''
                  }
                >
                  {plan
                    ? stale
                      ? label('refreshFirst')
                      : label('validated')
                    : check?.status === 'deferred'
                      ? label('waitEntry')
                      : check?.status === 'rejected'
                        ? label('notFeasible')
                        : busy
                          ? label('evaluating')
                          : label('notProposed')}
                </span>
              </header>
              {plan ? (
                <>
                  <b>{plan.idea.name}</b>
                  <span
                    className={`frontier-plan-side ${plan.idea.side === 'long' ? 'target-text' : 'stop-text'}`}
                  >
                    {label(plan.idea.side === 'long' ? 'long' : 'short')} ·{' '}
                    {plan.snapshot.symbol}
                  </span>
                  <p>
                    {plan.orders.length}{' '}
                    {label(
                      kind === 'twap'
                        ? 'timedSlices'
                        : kind === 'ladder'
                          ? 'priceLevels'
                          : 'entryCount'
                    )}{' '}
                    ·{' '}
                    {label('netRR', {
                      ratio: Number(plan.reward_risk).toFixed(2),
                    })}
                  </p>
                  <dl className="frontier-plan-levels">
                    <div>
                      <dt>{label(kind === 'market' ? 'estEntry' : 'entry')}</dt>
                      <dd>{price(plan.idea.entry)}</dd>
                    </div>
                    <div>
                      <dt>{label('stop')}</dt>
                      <dd className="stop-text">{price(plan.idea.stop)}</dd>
                    </div>
                    <div>
                      <dt>{label('target')}</dt>
                      <dd className="target-text">{price(plan.idea.target)}</dd>
                    </div>
                  </dl>
                  <p>
                    {kind === 'market'
                      ? label('marketNote')
                      : kind === 'twap'
                        ? label('twapNote', {
                            count: plan.orders.length,
                            seconds: plan.idea.execution.interval_seconds || 0,
                          })
                        : kind === 'ladder'
                          ? label('ladderNote')
                          : label('limitNote')}
                  </p>
                  {!connected && (
                    <div className="frontier-illustrative">
                      {label('illustrative')}
                    </div>
                  )}
                  <PlanFunding plan={plan} compact />
                  <div className="frontier-plan-outcomes">
                    <span>
                      {label('plannedLoss')}{' '}
                      <b className="stop-text">−{money(plan.risk)}</b>
                    </span>
                    <span>
                      {label('targetReturn')}{' '}
                      <b className="target-text">+{money(plan.reward)}</b>
                    </span>
                    <small>
                      {label('costsIncluded', {
                        quote: plan.snapshot.quote_asset,
                      })}
                    </small>
                  </div>
                  <div className="frontier-plan-actions">
                    <Button
                      type="button"
                      className="h-[42px] px-2 text-[13px]"
                      onClick={() => onSelect(plan)}
                      aria-label={label('viewOnChartName', {
                        name: plan.idea.name,
                      })}
                    >
                      {label(plan.id === selected ? 'onChart' : 'viewOnChart')}
                    </Button>
                    {onReview && (
                      <Button
                        type="button"
                        variant="primary"
                        className="h-[42px] px-2 text-[13px]"
                        disabled={reviewDisabled}
                        onClick={() => onReview(plan)}
                        aria-label={label('reviewName', {
                          name: structureName(kind, language),
                        })}
                      >
                        {!connected
                          ? label('connect')
                          : stale
                            ? label('refreshReview')
                            : label('review')}
                      </Button>
                    )}
                  </div>
                </>
              ) : (
                <p>
                  {(check?.reason
                    ? validationSummary(check.reason, language)
                    : '') || (busy ? label('reading') : label('noCandidate'))}
                </p>
              )}
            </article>
          )
        })}
    </section>
  )
}
