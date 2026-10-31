import type { LiveRun, Plan } from '@/api/planning'
import { Dialog, DialogNote } from '@/components/common/dialog'
import { Button } from '@/components/ui/button'
import { PlanFunding } from './plan-funding'
import { format } from './planning-labels'
import { useFrontierLabel } from './use-frontier-label'

// PlanReviewDialog is the final review before a plan is locked and sent live.
export function PlanReviewDialog({
  plan,
  accountName,
  replacement,
  busy,
  expired,
  onBack,
  onConfirm,
}: {
  plan: Plan
  accountName?: string
  replacement: LiveRun | null
  busy: boolean
  expired: boolean
  onBack: () => void
  onConfirm: () => void
}) {
  const label = useFrontierLabel('review')
  const side = label(plan.idea.side === 'long' ? 'long' : 'short')
  const market = plan.idea.execution.kind === 'market'
  const prices: [string, string, string][] = [
    [label(market ? 'estEntry' : 'entry'), plan.idea.entry, ''],
    [label('stopLoss'), plan.idea.stop, 'text-vergex-danger'],
    [label('target'), plan.idea.target, 'text-vergex-success'],
  ]
  return (
    <Dialog
      title={label('title', { side, symbol: plan.snapshot.symbol })}
      description={`${accountName ?? ''} · ${plan.snapshot.venue} · ${plan.snapshot.symbol} · ${side}`}
      onClose={onBack}
      footer={
        <>
          <Button type="button" disabled={busy} onClick={onBack}>
            {label('back')}
          </Button>
          <Button
            type="button"
            variant="primary"
            disabled={busy || expired}
            onClick={onConfirm}
          >
            {label('confirm')}
          </Button>
        </>
      }
    >
      <div className="space-y-4 text-[13px] leading-relaxed text-vergex-text">
        <dl className="grid grid-cols-3 gap-3 rounded-xl bg-[var(--canvas)] p-4">
          {prices.map(([name, value, tone]) => (
            <div key={name}>
              <dt className="text-xs text-vergex-text-muted">{name}</dt>
              <dd
                className={`mt-1 text-base font-semibold tabular-nums ${tone}`}
              >
                {format(value)}
              </dd>
            </div>
          ))}
        </dl>
        <p>
          {label('summary', {
            quantity: format(plan.quantity),
            loss: format(plan.risk),
            quote: plan.snapshot.quote_asset,
            orders: plan.orders.length || 1,
            leverage: plan.constraints.leverage || 1,
          })}
        </p>
        <PlanFunding plan={plan} />
        {replacement && (
          <div role="note">
            <DialogNote tone="warning">
              {label('replaces', { symbol: replacement.plan.snapshot.symbol })}
            </DialogNote>
          </div>
        )}
        <p className="text-vergex-text-muted">{label('locked')}</p>
        <details>
          <summary className="cursor-pointer font-medium">
            {label('details')}
          </summary>
          <p className="mt-2 text-vergex-text-muted">
            {label(market ? 'marketNote' : 'limitNote')}{' '}
            {label('protectionNote')}
          </p>
        </details>
        <table className="w-full text-left tabular-nums">
          <thead className="text-xs text-vergex-text-muted">
            <tr>
              <th className="py-1.5 font-medium">
                {label(market ? 'executionEstimate' : 'limitPrice')}
              </th>
              <th className="py-1.5 font-medium">{label('quantity')}</th>
              <th className="py-1.5 font-medium">{label('releaseTime')}</th>
            </tr>
          </thead>
          <tbody>
            {plan.orders.map((o) => (
              <tr key={o.index} className="border-t border-[var(--line)]">
                <td className="py-1.5">{format(o.price)}</td>
                <td className="py-1.5">{format(o.quantity)}</td>
                <td className="py-1.5">
                  {label('seconds', { count: o.offset_seconds })}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        <details>
          <summary className="cursor-pointer font-medium">
            {label('reference')}
          </summary>
          <code className="mt-2 block break-all text-xs text-vergex-text-muted">
            {plan.hash}
          </code>
        </details>
      </div>
    </Dialog>
  )
}
