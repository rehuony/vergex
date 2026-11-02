import { expired, type LiveRun, type FuturesState } from '@/api/futures'
import { runSettled } from '@/utils/futures/futures-plans'
import { Button } from '@/components/ui/button'
import { useLanguage } from '@/contexts/language-context'
import { planStateLabel } from './futures-labels'
import { useFuturesLabel } from './use-futures-label'

interface SelectedPlanActionsProps {
  record: FuturesState['records'][string] | undefined
  run: LiveRun | undefined
  accountName: string | undefined
  connected: boolean
  reviewDisabled: boolean
  busy: boolean
  clock: number
  onReview: () => void
  onRunAction: (action: 'execute' | 'stop', run: LiveRun) => void
}

// SelectedPlanActions is the next step for the plan on the chart, pinned under the plans:
// review a draft, place or revoke an approved trade, or stop a running one.
export function SelectedPlanActions({
  record,
  run,
  accountName,
  connected,
  reviewDisabled,
  busy,
  clock,
  onReview,
  onRunAction,
}: SelectedPlanActionsProps) {
  const { language } = useLanguage()
  const label = useFuturesLabel('selected')
  if (!record) return null
  const state = [
    planStateLabel(run?.state || record.state, language),
    label('liveTrading'),
    accountName,
  ]
    .filter(Boolean)
    .join(' · ')
  return (
    <div className="shrink-0 space-y-1 border-t border-vergex-line px-4 py-2">
      {run?.error && (
        <p className="truncate text-xs text-vergex-danger" title={run.error}>
          {run.error}
        </p>
      )}
      <div className="flex min-h-8 items-center justify-end gap-2">
        <p
          className="min-w-0 flex-1 truncate text-xs text-vergex-muted"
          title={state}
        >
          {state}
        </p>
        {record.state === 'draft' ? (
          <Button
            type="button"
            variant="primary"
            size="sm"
            className="max-sm:h-10"
            disabled={reviewDisabled}
            onClick={onReview}
          >
            {!connected
              ? label('connect')
              : expired(record.plan, clock)
                ? label('refreshReview')
                : label('review')}
          </Button>
        ) : run?.state === 'approved' ? (
          <>
            <Button
              type="button"
              size="sm"
              className="max-sm:h-10"
              disabled={busy}
              onClick={() => onRunAction('stop', run)}
            >
              {label('revoke')}
            </Button>
            <Button
              type="button"
              variant="primary"
              size="sm"
              className="max-sm:h-10"
              disabled={busy}
              onClick={() => onRunAction('execute', run)}
            >
              {label('placeApproved')}
            </Button>
          </>
        ) : (
          run &&
          !runSettled(run.state) && (
            <Button
              type="button"
              size="sm"
              className="max-sm:h-10"
              disabled={busy}
              onClick={() => onRunAction('stop', run)}
            >
              {label('stopEntries')}
            </Button>
          )
        )}
      </div>
    </div>
  )
}
