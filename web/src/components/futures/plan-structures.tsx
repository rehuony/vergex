import { TriangleAlert } from 'lucide-react'
import type { Plan } from '@/api/futures'
import { cn } from '@/utils/cn'
import type { PlanCheck } from '@/utils/futures/futures-plans'
import { validationSummary } from '@/utils/futures/futures-presentation'
import { useLanguage } from '@/contexts/language-context'
import { structureName } from './futures-activity'
import { useFuturesLabel } from './use-futures-label'

const price = (value: string) =>
  Number(value).toLocaleString('en-US', { maximumFractionDigits: 8 })

// The head and every row share these columns, so prices line up from plan to plan. A phone
// has no room for the ratio, which the selected plan's figures also give.
const rowClass =
  'grid grid-cols-[72px_repeat(3,minmax(0,1fr))] items-center gap-x-2 px-4 sm:grid-cols-[80px_repeat(3,minmax(0,1fr))_36px]'
const ratioClass = 'text-right max-sm:hidden'

function Level({
  value,
  tone,
  title,
  prefix = '',
}: {
  value: string
  tone: string
  title?: string
  prefix?: string
}) {
  const shown = price(value)
  return (
    <span
      className={cn('mono truncate text-right text-xs', tone)}
      title={title ?? shown}
    >
      {prefix}
      {shown}
    </span>
  )
}

// PlanStructures lists the ways of entering that came to something, one row each and in one
// order. Pressing a plan puts it on the chart. A way with no plan says why instead, and one
// the model never proposed has nothing to say, so it only shows while the model may still.
export function PlanStructures({
  checks,
  plans,
  busy,
  selected,
  onSelect,
  now = 0,
  showAllTypes = true,
}: {
  checks: PlanCheck[]
  plans: Plan[]
  busy: boolean
  selected: string
  onSelect: (plan: Plan) => void
  showAllTypes?: boolean
  now?: number
}) {
  const { language } = useLanguage()
  const label = useFuturesLabel('structures')
  const kinds = ['market', 'limit', 'ladder', 'twap'].filter(
    (kind) =>
      (showAllTypes && busy) ||
      checks.some((check) => check.kind === kind) ||
      plans.some((plan) => plan.idea.execution.kind === kind)
  )
  if (!kinds.length) return null
  return (
    <section aria-label={label('name')}>
      <div
        className={cn(
          rowClass,
          'sticky top-0 z-10 h-8 border-b border-vergex-line bg-vergex-surface-muted text-[11px] font-medium text-vergex-muted'
        )}
      >
        <span>{label('kind')}</span>
        <span className="text-right">{label('entry')}</span>
        <span className="text-right">{label('stop')}</span>
        <span className="text-right">{label('target')}</span>
        <span className={ratioClass}>{label('rewardRisk')}</span>
      </div>
      {kinds.map((kind) => {
        const plan = plans.find((p) => p.idea.execution.kind === kind)
        const name = structureName(kind, language)
        if (!plan) {
          const check = checks.filter((row) => row.kind === kind).slice(-1)[0]
          return (
            <article
              key={kind}
              className={cn(
                rowClass,
                'min-h-11 border-b border-vergex-line py-2'
              )}
            >
              <span
                className="truncate text-[13px] font-medium text-vergex-muted"
                title={name}
              >
                {name}
              </span>
              <p className="col-span-3 min-w-0 text-xs text-vergex-muted sm:col-span-4">
                <span
                  className={cn(
                    check?.status === 'rejected' && 'text-vergex-danger'
                  )}
                >
                  {check?.status === 'deferred'
                    ? label('waitEntry')
                    : check?.status === 'rejected'
                      ? label('notFeasible')
                      : label('evaluating')}
                </span>
                {check?.reason && (
                  <span className="ml-1.5">
                    {validationSummary(check.reason, language)}
                  </span>
                )}
              </p>
            </article>
          )
        }
        const long = plan.idea.side === 'long'
        return (
          <article key={kind} className="border-b border-vergex-line">
            <button
              type="button"
              aria-label={label('viewOnChartName', { name: plan.idea.name })}
              aria-current={plan.id === selected ? 'true' : undefined}
              title={plan.idea.name}
              onClick={() => onSelect(plan)}
              className={cn(
                rowClass,
                'h-11 w-full text-left font-normal transition-colors [text-rendering:inherit] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-vergex-ink/25 max-sm:h-12',
                plan.id === selected
                  ? 'bg-vergex-ink-tint'
                  : 'hover:bg-vergex-surface-muted'
              )}
            >
              <span className="min-w-0">
                <span className="flex items-center gap-1">
                  <span className="truncate text-[13px] font-medium leading-tight text-vergex-ink">
                    {name}
                  </span>
                  {Date.parse(plan.expires) <= now && (
                    <TriangleAlert
                      className="size-3 shrink-0 text-vergex-warning"
                      aria-label={label('refreshFirst')}
                    />
                  )}
                </span>
                <span
                  className={cn(
                    'block text-[11px] leading-tight',
                    long ? 'text-vergex-success' : 'text-vergex-danger'
                  )}
                >
                  {label(long ? 'long' : 'short')}
                </span>
              </span>
              <Level
                value={plan.idea.entry}
                tone="text-vergex-ink"
                prefix={kind === 'market' ? '≈' : ''}
                title={kind === 'market' ? label('estEntry') : undefined}
              />
              <Level value={plan.idea.stop} tone="text-vergex-danger" />
              <Level value={plan.idea.target} tone="text-vergex-success" />
              <span
                className={cn('mono text-xs text-vergex-ink', ratioClass)}
                title={label('rewardRisk')}
              >
                {Number(plan.reward_risk).toFixed(2)}
              </span>
            </button>
          </article>
        )
      })}
    </section>
  )
}
