import type { LiveRun } from '@/api/planning'
import type { AccountObservation } from '@/utils/planning/frontier-workspace'
import { nativeMarket } from '@/utils/planning/frontier-account'
import { Button } from '@/components/ui/button'
import { useFrontierLabel } from './use-frontier-label'

export function FrontierOrderBlocker({
  run,
  now,
  observation,
  checkError,
  busy,
  onCheck,
  onView,
  onRevoke,
  onAccount,
}: {
  run: LiveRun
  now: number
  observation?: AccountObservation | null
  checkError?: string
  busy: boolean
  onCheck: () => void
  onView: () => void
  onRevoke: () => void
  onAccount?: () => void
}) {
  const label = useFrontierLabel('blocker')
  const venue = run.plan.snapshot.venue
  const symbol = run.plan.snapshot.symbol
  const fresh =
    observation?.accountID === run.exchange_id &&
    Math.abs(now - observation.checkedAt) < 30000
  const position = fresh
    ? observation.positions.find(
        (p) => nativeMarket(p.symbol, venue) === nativeMarket(symbol, venue)
      )
    : undefined
  const approved = run.state === 'approved'
  const active = ['starting', 'running', 'stopping'].includes(run.state)
  const needsCheck = !active && !approved
  const title = position
    ? label('positionOpen', {
        symbol,
        side: label(position.side === 'long' ? 'long' : 'short'),
        leverage: position.leverage,
      })
    : approved
      ? label('notSubmitted', { symbol })
      : active
        ? run.state === 'stopping'
          ? label('stopping')
          : label('inProgress')
        : fresh
          ? label('noPosition', { symbol })
          : label('needsVerification', { symbol })
  const detail = position
    ? label('positionDetail', {
        value: (position.quantity * position.mark).toLocaleString('en-US', {
          maximumFractionDigits: 2,
        }),
        quote: run.plan.snapshot.quote_asset,
      })
    : approved
      ? label('approvedDetail')
      : active
        ? label('activeDetail')
        : fresh
          ? label('freshDetail')
          : label('staleDetail')
  return (
    <section
      className={`frontier-order-blocker ${(position && run.state !== 'needs_attention') || active ? 'is-active' : ''}`}
      aria-label={label('name')}
    >
      <div>
        <strong>{title}</strong>
        <p>{detail}</p>
        {position && run.state === 'needs_attention' && (
          <small>{label('exitsUnverified')}</small>
        )}
        {checkError && <p role="alert">{checkError}</p>}
      </div>
      <div className="frontier-blocker-actions">
        {position ? (
          <Button
            size="sm"
            className="h-8 border-[var(--warning-border)] text-xs"
            onClick={onAccount}
          >
            {label('viewPosition')}
          </Button>
        ) : (
          <Button
            size="sm"
            className="h-8 border-[var(--warning-border)] text-xs"
            disabled={busy}
            onClick={onView}
          >
            {label('viewTrade')}
          </Button>
        )}
        {needsCheck && (
          <Button
            size="sm"
            className="h-8 border-[var(--warning-border)] text-xs"
            disabled={busy}
            onClick={onCheck}
          >
            {label('verifyStatus')}
          </Button>
        )}
        {approved && (
          <Button
            size="sm"
            className="h-8 border-[var(--warning-border)] text-xs"
            disabled={busy}
            onClick={onRevoke}
          >
            {label('cancelApproval')}
          </Button>
        )}
      </div>
    </section>
  )
}
