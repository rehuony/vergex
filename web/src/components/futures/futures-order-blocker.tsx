import { useState } from 'react'
import { TriangleAlert } from 'lucide-react'
import type { LiveRun } from '@/api/futures'
import type { AccountObservation } from '@/utils/futures/futures-workspace'
import { nativeMarket } from '@/utils/futures/futures-account'
import { Button } from '@/components/ui/button'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import { cn } from '@/utils/cn'
import { useFuturesLabel } from './use-futures-label'

// FuturesOrderBlocker takes the line above the assistant's input while an earlier trade
// reserves the account. The line says what the trade is doing, and what can be done about
// it opens from there, so nothing on the page moves for it.
export function FuturesOrderBlocker({
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
  const label = useFuturesLabel('blocker')
  const [open, setOpen] = useState(false)
  const venue = run.plan.snapshot.venue
  const symbol = run.plan.snapshot.symbol
  const fresh =
    observation?.accountID === run.account_id &&
    Math.abs(now - observation.checkedAt) < 30000
  const position = fresh
    ? observation.positions.find(
        (p) => nativeMarket(p.symbol, venue) === nativeMarket(symbol, venue)
      )
    : undefined
  const approved = run.state === 'approved'
  const active = ['starting', 'running', 'stopping'].includes(run.state)
  const needsCheck = !active && !approved
  const unverified = Boolean(position && run.state === 'needs_attention')
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
  const calm = (position && run.state !== 'needs_attention') || active
  const note = unverified ? `${label('exitsUnverified')} ${detail}` : detail
  // These leave the tab the line is on, so what they opened from closes first.
  const leaving = (action?: () => void) => () => {
    setOpen(false)
    action?.()
  }
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <button
          type="button"
          className={cn(
            'flex min-w-0 flex-1 items-center gap-1 self-stretch text-left',
            checkError
              ? 'text-vergex-danger'
              : calm
                ? 'text-vergex-ink'
                : 'text-vergex-warning'
          )}
        >
          {!calm && (
            <TriangleAlert className="size-3.5 shrink-0" aria-hidden="true" />
          )}
          <span className="min-w-0 truncate">{checkError || title}</span>
        </button>
      </PopoverTrigger>
      <PopoverContent
        aria-label={label('name')}
        className="w-[min(360px,calc(100vw-32px))] space-y-3 p-3 text-[13px] leading-relaxed"
      >
        <div>
          <p
            className={cn(
              'font-medium',
              calm ? 'text-vergex-ink' : 'text-vergex-warning'
            )}
          >
            {title}
          </p>
          <p className="mt-1 break-words text-vergex-muted">{note}</p>
          {checkError && (
            <p role="alert" className="mt-1 break-words text-vergex-danger">
              {checkError}
            </p>
          )}
        </div>
        <div className="flex flex-wrap justify-end gap-2">
          {position ? (
            <Button
              type="button"
              size="sm"
              className="max-sm:h-10"
              onClick={leaving(onAccount)}
            >
              {label('viewPosition')}
            </Button>
          ) : (
            <Button
              type="button"
              size="sm"
              className="max-sm:h-10"
              disabled={busy}
              onClick={leaving(onView)}
            >
              {label('viewTrade')}
            </Button>
          )}
          {needsCheck && (
            <Button
              type="button"
              size="sm"
              className="max-sm:h-10"
              disabled={busy}
              onClick={onCheck}
            >
              {label('verifyStatus')}
            </Button>
          )}
          {approved && (
            <Button
              type="button"
              size="sm"
              className="max-sm:h-10"
              disabled={busy}
              onClick={onRevoke}
            >
              {label('cancelApproval')}
            </Button>
          )}
        </div>
      </PopoverContent>
    </Popover>
  )
}
