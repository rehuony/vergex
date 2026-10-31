import useSWR from 'swr'
import {
  planningAPI,
  type LiveRun,
  type Plan,
  type PlanningState,
} from '@/api/planning'
import { Panel, PanelState } from '@/components/common/panel'
import { fmtUsd } from '@/components/trader/trader-format'
import { Button } from '@/components/ui/button'
import { useLanguage } from '@/contexts/language-context'
import type { PlanningConversation } from '@/hooks/planning/use-planning-data'
import { t } from '@/i18n/translations'
import { cn } from '@/utils/cn'
import { PlanCard } from './plan-card'
import { format, orderStateLabel, planStateLabel } from './planning-labels'

interface PlanResult {
  realized_pnl: number
  closes: number
}

// FrontierHistory is what has already happened on this account and market:
// finished plans with what they realized, drafts from earlier sessions and past
// conversations. Results come from the exchange, so they load only while shown.
export function FrontierHistory({
  active,
  accountID,
  runs,
  archived,
  selected,
  compare,
  clock,
  conversations,
  busy,
  onChoose,
  onOpenConversation,
}: {
  active: boolean
  accountID: string
  runs: LiveRun[]
  archived: PlanningState['records'][string][]
  selected: string
  compare: string[]
  clock: number
  conversations: PlanningConversation[]
  busy: boolean
  onChoose: (plan: Plan) => void
  onOpenConversation: (conversation: PlanningConversation) => void
}) {
  const { language } = useLanguage()
  const label = (key: string) => t(`frontier.${key}`, language)
  const { data: results } = useSWR<Record<string, PlanResult>>(
    active && accountID && runs.length > 0
      ? ['plan-results', accountID, runs.length]
      : null,
    () =>
      planningAPI<Record<string, PlanResult>>(
        'results?exchange_id=' + encodeURIComponent(accountID)
      ),
    { revalidateOnFocus: false, shouldRetryOnError: false }
  )
  return (
    <div className="flex flex-col gap-4">
      <Panel title={label('finishedPlans')}>
        {runs.length === 0 ? (
          <PanelState role="status">{label('noFinishedPlans')}</PanelState>
        ) : (
          <ul className="divide-y divide-[var(--line)]">
            {runs.map((run) => (
              <li key={run.id} className="px-4 py-3">
                <div className="flex items-baseline gap-3">
                  <p className="min-w-0 flex-1 truncate text-[13px] font-medium text-vergex-text">
                    {run.plan.idea.name} · {planStateLabel(run.state, language)}
                  </p>
                  <RunResult
                    result={results?.[run.id]}
                    label={label('realized')}
                  />
                </div>
                {run.error && (
                  <p className="mt-1 text-xs break-words text-vergex-danger">
                    {run.error}
                  </p>
                )}
                <div className="mt-2 overflow-x-auto">
                  <table className="w-full text-left text-xs tabular-nums">
                    <thead>
                      <tr>
                        <th className="py-1 pr-3 font-medium">
                          {label('orderType')}
                        </th>
                        <th className="py-1 pr-3 font-medium">
                          {label('orderId')}
                        </th>
                        <th className="py-1 pr-3 font-medium">
                          {label('orderState')}
                        </th>
                        <th className="py-1 text-right font-medium">
                          {label('filledQuantity')}
                        </th>
                      </tr>
                    </thead>
                    <tbody>
                      {run.orders.map((order) => (
                        <tr key={order.client_id}>
                          <td className="py-1 pr-3">{order.role}</td>
                          <td className="py-1 pr-3">
                            {order.order_id || label('unconfirmed')}
                          </td>
                          <td className="py-1 pr-3">
                            {orderStateLabel(order.status, language)}
                          </td>
                          <td className="py-1 text-right">
                            {format(order.filled)}
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </li>
            ))}
          </ul>
        )}
      </Panel>
      {archived.length > 0 && (
        <Panel title={label('earlierDrafts')} meta={String(archived.length)}>
          <div className="planning-card-list px-4 pb-4">
            {archived.map((record) => (
              <PlanCard
                key={record.plan.id}
                record={record}
                selected={selected === record.plan.id}
                compareIndex={compare.indexOf(record.plan.id)}
                clock={clock}
                onChoose={onChoose}
              />
            ))}
          </div>
        </Panel>
      )}
      <Panel title={label('conversations')}>
        {conversations.length === 0 ? (
          <PanelState role="status">{label('noConversations')}</PanelState>
        ) : (
          <ul className="px-2 pb-2">
            {conversations.map((conversation) => (
              <li key={conversation.id}>
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  className="w-full justify-start max-sm:h-10"
                  disabled={busy}
                  onClick={() => onOpenConversation(conversation)}
                >
                  {conversation.symbol} · {conversation.id.slice(0, 8)}
                </Button>
              </li>
            ))}
          </ul>
        )}
      </Panel>
    </div>
  )
}

function RunResult({ result, label }: { result?: PlanResult; label: string }) {
  const pnl = result?.realized_pnl
  return (
    <span className="whitespace-nowrap text-xs text-vergex-text-muted">
      {label}{' '}
      <span
        className={cn(
          'font-medium tabular-nums',
          pnl == null
            ? 'text-vergex-text-muted'
            : pnl >= 0
              ? 'text-vergex-success'
              : 'text-vergex-danger'
        )}
      >
        {fmtUsd(pnl, true)}
      </span>
    </span>
  )
}
