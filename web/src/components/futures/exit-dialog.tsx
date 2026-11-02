import { useId, useState } from 'react'
import { futuresAPI, type LiveRun } from '@/api/futures'
import {
  Dialog,
  DialogField,
  dialogInputClass,
} from '@/components/common/dialog'
import { notify } from '@/components/common/notify'
import { Button } from '@/components/ui/button'
import type { AccountPosition } from '@/utils/futures/futures-account'
import { useFuturesLabel } from './use-futures-label'

// The price a run's stop or target stands at now: its newest order of that kind still
// working, which a change of price may have made different from the plan's.
export function workingExit(run: LiveRun, role: 'sl' | 'tp') {
  return run.orders
    .filter((order) => order.role === role && order.status === 'NEW')
    .pop()?.price
}

// breakeven is the entry on the market's tick, rounded to the side that loses nothing: an
// average entry is rarely on a tick itself.
function breakeven(run: LiveRun, position: AccountPosition) {
  const tick = Number(run.plan.snapshot.tick)
  const decimals = run.plan.snapshot.tick.split('.')[1]?.length ?? 0
  const steps = position.entry / tick
  const rounded =
    position.side === 'long' ? Math.ceil(steps) : Math.floor(steps)
  return (rounded * tick).toFixed(decimals)
}

// ExitDialog moves the stop and target of a position a plan holds. Each goes to the server
// on its own, the stop first, and what the server refuses is said here, so the dialog stays
// open to fix it.
export function ExitDialog({
  run,
  position,
  onClose,
  onDone,
}: {
  run: LiveRun
  position: AccountPosition
  onClose: () => void
  onDone: () => Promise<unknown>
}) {
  const label = useFuturesLabel('account')
  const id = useId()
  const currentStop = workingExit(run, 'sl') ?? ''
  const currentTarget = workingExit(run, 'tp') ?? ''
  const [stop, setStop] = useState(currentStop)
  const [target, setTarget] = useState(currentTarget)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  const changes = (
    [
      ['sl', stop, currentStop],
      ['tp', target, currentTarget],
    ] as const
  ).filter(
    ([, value, current]) =>
      value.trim() !== '' && Number(value) !== Number(current)
  )

  const save = async () => {
    setBusy(true)
    setError('')
    try {
      for (const [role, value] of changes) {
        await futuresAPI('amend-exit', {
          id: run.id,
          role,
          price: value.trim(),
        })
      }
      notify.success(label('exitsSaved'))
      onClose()
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(false)
      await onDone()
    }
  }

  return (
    <Dialog
      title={label('exitsTitle', { market: position.symbol })}
      size="md"
      onClose={onClose}
      footer={
        <>
          <span
            role={error ? 'alert' : undefined}
            className="mr-auto min-w-0 break-words text-xs text-vergex-danger"
          >
            {error}
          </span>
          <Button type="button" variant="secondary" onClick={onClose}>
            {label('exitsCancel')}
          </Button>
          <Button
            type="button"
            disabled={busy || changes.length === 0}
            onClick={() => void save()}
          >
            {label('exitsSave')}
          </Button>
        </>
      }
    >
      <div className="flex flex-col gap-4">
        <DialogField label={label('stopLoss')} htmlFor={`${id}-stop`}>
          <div className="flex items-center gap-2">
            <input
              id={`${id}-stop`}
              inputMode="decimal"
              className={dialogInputClass}
              value={stop}
              onChange={(event) => setStop(event.target.value)}
            />
            <Button
              type="button"
              variant="secondary"
              className="shrink-0"
              onClick={() => setStop(breakeven(run, position))}
            >
              {label('exitsBreakeven')}
            </Button>
          </div>
        </DialogField>
        <DialogField label={label('takeProfit')} htmlFor={`${id}-target`}>
          <input
            id={`${id}-target`}
            inputMode="decimal"
            className={dialogInputClass}
            value={target}
            onChange={(event) => setTarget(event.target.value)}
          />
        </DialogField>
      </div>
    </Dialog>
  )
}
