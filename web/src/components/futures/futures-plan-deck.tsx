import type { Plan } from '@/api/futures'
import { PanelState } from '@/components/common/panel'
import { Button } from '@/components/ui/button'
import { useLanguage } from '@/contexts/language-context'
import { activeLanguage, t } from '@/i18n'
import type { PlanBatch } from '@/utils/futures/futures-workspace'
import { PlanStructures } from './plan-structures'
import type { PlanCheck } from '@/utils/futures/futures-plans'

interface FuturesPlanDeckProps {
  batch: PlanBatch | null
  plans: Plan[]
  checks: PlanCheck[]
  futuresBusy: boolean
  waitingForEntry: boolean
  selected: string
  clock: number
  connected: boolean
  comparing: boolean
  onChoose: (plan: Plan) => void
  onCompare: () => void
  onExitCompare: () => void
}

// FuturesPlanDeck shows the latest generation of entry plans beside the chart, and the
// checks the assistant ran for each structure.
export function FuturesPlanDeck({
  batch,
  plans,
  checks,
  futuresBusy,
  waitingForEntry,
  selected,
  clock,
  connected,
  comparing,
  onChoose,
  onCompare,
  onExitCompare,
}: FuturesPlanDeckProps) {
  const { language } = useLanguage()
  const label = (key: string, params?: Record<string, string | number>) =>
    t(`futures.${key}`, language, params)
  // What the generation left, then what this turn found, which is what a row goes by.
  const entryChecks = [
    ...(futuresBusy ? [] : batch?.deferred || []).map((entry) => ({
      id: `deferred_${entry.kind}`,
      kind: entry.kind,
      name: label('waitConditions'),
      status: entry.rejected ? 'rejected' : 'deferred',
      reason: entry.reason,
    })),
    ...checks,
  ]
  // Nothing has been generated: all there is to say is how to start.
  if (
    !batch &&
    !plans.length &&
    !futuresBusy &&
    !entryChecks.length &&
    !waitingForEntry
  ) {
    return (
      <section
        id="futures-plans"
        aria-label={label('entryPlans')}
        className="h-full"
      >
        <PanelState role="status" className="h-full">
          {label('noPlansYet')}
        </PanelState>
      </section>
    )
  }
  return (
    <section id="futures-plans" aria-label={label('entryPlans')}>
      <header className="flex min-h-11 items-center gap-2 py-1.5 pl-4 pr-2">
        <div className="flex min-w-0 flex-1 items-baseline gap-2">
          <h2 className="truncate text-[13px] font-medium text-vergex-ink">
            {waitingForEntry
              ? label('waitingForEntry')
              : batch
                ? label('latestPlans')
                : label('savedIdea')}
          </h2>
          {batch && !waitingForEntry && (
            <time className="whitespace-nowrap text-[11px] text-vergex-muted">
              {new Date(batch.created).toLocaleString(activeLanguage(), {
                month: 'short',
                day: 'numeric',
                hour: '2-digit',
                minute: '2-digit',
              })}
            </time>
          )}
          {!connected && (
            <span
              className="min-w-0 truncate text-[11px] text-vergex-muted"
              title={label('structures.illustrative')}
            >
              {label('structures.illustrativeShort')}
            </span>
          )}
        </div>
        {plans.length > 1 && (
          <Button
            type="button"
            size="sm"
            className="max-sm:h-10"
            onClick={comparing ? onExitCompare : onCompare}
          >
            {label(comparing ? 'exitCompare' : 'comparePlans')}
          </Button>
        )}
      </header>
      <PlanStructures
        checks={entryChecks}
        showAllTypes={Boolean(batch)}
        plans={plans}
        busy={futuresBusy}
        selected={selected}
        onSelect={onChoose}
        now={clock}
      />
    </section>
  )
}
