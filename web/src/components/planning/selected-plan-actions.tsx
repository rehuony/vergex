import { expired, type LiveRun, type PlanningState } from '@/api/planning'
import { runSettled } from '@/utils/planning/frontier-plans'
import { Button } from '@/components/ui/button'
import { useLanguage } from '@/contexts/language-context'
import { planStateLabel } from './planning-labels'
import { useFrontierLabel } from './use-frontier-label'

interface SelectedPlanActionsProps {
  record: PlanningState['records'][string] | undefined
  run: LiveRun | undefined
  accountName: string | undefined
  connected: boolean
  reviewDisabled: boolean
  busy: boolean
  clock: number
  onReview: () => void
  onRunAction: (action: 'execute' | 'stop', run: LiveRun) => void
}

// SelectedPlanActions is the next step for the plan on the chart: review a
// draft, place or revoke an approved trade, or stop a running one.
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
  const label = useFrontierLabel('selected')
  return (
    <div className="planning-review">
      {record ? (
        <>
          <b>{record.plan.idea.name}</b>
          <p>
            {planStateLabel(run?.state || record.state, language)} ·{' '}
            {label('liveTrading')} · {accountName}
          </p>
          {run?.error && <p className="stop-text">{run.error}</p>}
          {record.state === 'draft' ? (
            <Button
              type="button"
              variant="primary"
              className="w-full"
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
                variant="primary"
                className="w-full"
                disabled={busy}
                onClick={() => onRunAction('execute', run)}
              >
                {label('placeApproved')}
              </Button>
              <Button
                type="button"
                className="w-full"
                disabled={busy}
                onClick={() => onRunAction('stop', run)}
              >
                {label('revoke')}
              </Button>
            </>
          ) : (
            run &&
            !runSettled(run.state) && (
              <Button
                type="button"
                className="w-full"
                disabled={busy}
                onClick={() => onRunAction('stop', run)}
              >
                {label('stopEntries')}
              </Button>
            )
          )}
          <small>{label('note')}</small>
        </>
      ) : (
        <p className="frontier-no-orders">
          <span aria-hidden="true">○</span> {label('drafts')}
        </p>
      )}
    </div>
  )
}
