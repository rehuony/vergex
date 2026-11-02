import { useState } from 'react'
import { ChevronRight, TriangleAlert } from 'lucide-react'
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
  'grid grid-cols-[96px_repeat(3,minmax(0,1fr))] items-center gap-x-2 px-4 sm:grid-cols-[96px_repeat(3,minmax(0,1fr))_36px]'
const ratioClass = 'text-right max-sm:hidden'

function Level({
  value,
  tone,
  title,
}: {
  value: string
  tone: string
  title?: string
}) {
  const shown = price(value)
  return (
    <span
      className={cn('mono truncate text-right text-xs', tone)}
      title={title ?? shown}
    >
      {shown}
    </span>
  )
}

// A way of entering with no plan: what became of it, on one line that opens to the reason.
function PassedRow({
  name,
  status,
  reason,
}: {
  name: string
  status: string
  reason?: string
}) {
  const [open, setOpen] = useState(false)
  const label = useFuturesLabel('structures')
  const summary = (
    <>
      <span
        className={cn(
          'truncate text-[13px] font-medium text-vergex-muted',
          open && 'self-start'
        )}
        title={name}
      >
        {name}
      </span>
      <span className="col-span-3 flex min-w-0 items-baseline gap-1.5 text-xs text-vergex-muted sm:col-span-4">
        <span
          className={cn(
            'shrink-0',
            status === 'rejected' && 'text-vergex-danger'
          )}
        >
          {status === 'deferred'
            ? label('waitEntry')
            : status === 'rejected'
              ? label('notFeasible')
              : label('evaluating')}
        </span>
        {reason && (
          <>
            <span className={cn('min-w-0 flex-1', !open && 'truncate')}>
              {reason}
            </span>
            <ChevronRight
              className={cn(
                'size-3.5 shrink-0 transition-transform',
                open ? 'rotate-90 self-start' : 'self-center'
              )}
              aria-hidden="true"
            />
          </>
        )}
      </span>
    </>
  )
  return (
    <article className="border-b border-vergex-line">
      {reason ? (
        <button
          type="button"
          aria-expanded={open}
          onClick={() => setOpen(!open)}
          className={cn(
            rowClass,
            'min-h-11 w-full py-2 text-left font-normal transition-colors hover:bg-vergex-surface-muted hover:duration-0 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-vergex-ink/25 max-sm:min-h-12'
          )}
        >
          {summary}
        </button>
      ) : (
        <div className={cn(rowClass, 'min-h-11 py-2 max-sm:min-h-12')}>
          {summary}
        </div>
      )}
    </article>
  )
}

// PlanStructures lists what each way of entering came to, in one order: a row for each plan,
// and pressing one opens it and puts it on the chart. A way with no plan says why instead, and one the
// model never proposed has nothing to say, so it only shows while the model may still.
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
        const name = structureName(kind, language)
        const rows = plans.filter((plan) => plan.idea.execution.kind === kind)
        if (!rows.length) {
          const check = checks.filter((row) => row.kind === kind).slice(-1)[0]
          return (
            <PassedRow
              key={kind}
              name={name}
              status={check?.status ?? ''}
              reason={
                check?.reason && validationSummary(check.reason, language)
              }
            />
          )
        }
        return rows.map((plan) => {
          const long = plan.idea.side === 'long'
          return (
            <article key={plan.id} className="border-b border-vergex-line">
              <button
                type="button"
                aria-label={label('viewOnChartName', { name: plan.idea.name })}
                aria-current={plan.id === selected ? 'true' : undefined}
                aria-expanded={plan.id === selected}
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
                <span className="flex min-w-0 items-center gap-1">
                  <span className="truncate text-[13px] font-medium text-vergex-ink">
                    {name}
                  </span>
                  <span
                    className={cn(
                      'shrink-0 text-[11px]',
                      long ? 'text-vergex-success' : 'text-vergex-danger'
                    )}
                  >
                    {label(long ? 'long' : 'short')}
                  </span>
                  {Date.parse(plan.expires) <= now && (
                    <span className="shrink-0" title={label('refreshFirst')}>
                      <TriangleAlert
                        className="size-3 text-vergex-warning"
                        aria-label={label('refreshFirst')}
                      />
                    </span>
                  )}
                </span>
                <Level
                  value={plan.idea.entry}
                  tone="text-vergex-ink"
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
        })
      })}
    </section>
  )
}
