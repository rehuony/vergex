import type { Idea } from '@/api/planning'
import { Dialog, dialogInputClass } from '@/components/common/dialog'
import { Button } from '@/components/ui/button'
import { AppSelect } from '@/components/ui/select'
import { useFrontierLabel } from './use-frontier-label'

// ManualEntryDialog builds a user-defined draft plan; it never sends orders.
export function ManualEntryDialog({
  symbol,
  venue,
  accountName,
  leverage,
  onLeverageChange,
  idea,
  onIdeaChange,
  levels,
  onLevelsChange,
  busy,
  onCancel,
  onSave,
}: {
  symbol: string
  venue: string
  accountName?: string
  leverage: number
  onLeverageChange: (value: number) => void
  idea: Idea
  onIdeaChange: (idea: Idea) => void
  levels: string
  onLevelsChange: (value: string) => void
  busy: boolean
  onCancel: () => void
  onSave: () => void
}) {
  const label = useFrontierLabel('manual')
  const field = 'grid gap-1.5 text-[13px] font-medium text-vergex-text'
  return (
    <Dialog
      title={label('title', { symbol })}
      description={label('description', {
        account: accountName ?? '',
        venue,
        leverage,
      })}
      onClose={onCancel}
      footer={
        <>
          <Button type="button" disabled={busy} onClick={onCancel}>
            {label('cancel')}
          </Button>
          <Button
            type="button"
            variant="primary"
            disabled={busy}
            onClick={onSave}
          >
            {label('review')}
          </Button>
        </>
      }
    >
      <div className="grid gap-4 sm:grid-cols-2">
        <label className={field}>
          {label('leverage')}
          <AppSelect
            aria-label={label('leverageName')}
            value={leverage}
            onChange={(value) => onLeverageChange(Number(value))}
            options={[1, 2, 3, 5, 10, 20].map((value) => ({
              value,
              label: `${value}x`,
            }))}
          />
        </label>
        <label className={field}>
          {label('method')}
          <AppSelect
            aria-label={label('methodName')}
            value={idea.execution.kind}
            onChange={(value) =>
              onIdeaChange({
                ...idea,
                execution: {
                  kind: value as Idea['execution']['kind'],
                  ...(value === 'twap'
                    ? { slices: 4, interval_seconds: 30 }
                    : {}),
                },
              })
            }
            options={[
              { value: 'market', label: label('market') },
              { value: 'limit', label: label('limit') },
              { value: 'ladder', label: label('ladder') },
              { value: 'twap', label: label('twap') },
            ]}
          />
        </label>
        <label className={field}>
          {label('direction')}
          <AppSelect
            aria-label={label('directionName')}
            value={idea.side}
            onChange={(value) =>
              onIdeaChange({ ...idea, side: value as Idea['side'] })
            }
            options={[
              { value: 'long', label: label('long') },
              { value: 'short', label: label('short') },
            ]}
          />
        </label>
        {(['entry', 'stop', 'target'] as const)
          .filter((k) => k !== 'entry' || idea.execution.kind !== 'market')
          .map((k) => (
            <label key={k} className={field}>
              {label(k)}
              <input
                className={dialogInputClass}
                inputMode="decimal"
                value={idea[k]}
                onChange={(e) => onIdeaChange({ ...idea, [k]: e.target.value })}
              />
            </label>
          ))}
        {idea.execution.kind === 'ladder' && (
          <label className={`${field} sm:col-span-2`}>
            {label('levels')}
            <textarea
              className={`${dialogInputClass} h-28 py-2.5 font-mono`}
              placeholder={'82000 0.3\n81500 0.3\n81000 0.4'}
              value={levels}
              onChange={(e) => onLevelsChange(e.target.value)}
            />
          </label>
        )}
        {idea.execution.kind === 'twap' && (
          <>
            <label className={field}>
              {label('slices')}
              <input
                className={dialogInputClass}
                inputMode="numeric"
                value={idea.execution.slices}
                onChange={(e) =>
                  onIdeaChange({
                    ...idea,
                    execution: {
                      ...idea.execution,
                      slices: Number(e.target.value),
                    },
                  })
                }
              />
            </label>
            <label className={field}>
              {label('interval')}
              <input
                className={dialogInputClass}
                inputMode="numeric"
                value={idea.execution.interval_seconds}
                onChange={(e) =>
                  onIdeaChange({
                    ...idea,
                    execution: {
                      ...idea.execution,
                      interval_seconds: Number(e.target.value),
                    },
                  })
                }
              />
            </label>
          </>
        )}
      </div>
      <div className="mt-4 space-y-2 text-[13px] text-vergex-text-muted">
        {idea.execution.kind === 'market' && <p>{label('marketNote')}</p>}
        <p>{label('draftNote')}</p>
      </div>
    </Dialog>
  )
}
