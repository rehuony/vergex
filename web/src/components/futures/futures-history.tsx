import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import {
  expired,
  futuresAPI,
  type LiveRun,
  type Plan,
  type FuturesState,
} from '@/api/futures'
import {
  cellClass,
  headClass,
  numberClass,
  StatStrip,
  tableClass,
} from '@/components/common/figures'
import { Panel, PanelState } from '@/components/common/panel'
import { RailItem, SplitRail, SplitView } from '@/components/common/split-view'
import { fmtUsd } from '@/utils/trader-format'
import { Button } from '@/components/ui/button'
import { ScrollArea } from '@/components/ui/scroll-area'
import { AppSelect } from '@/components/ui/select'
import { useLanguage } from '@/contexts/language-context'
import type { FuturesConversation } from '@/hooks/futures/use-futures-data'
import { activeLanguage, t } from '@/i18n'
import { cn } from '@/utils/cn'
import { Message } from './assistant-thread'
import { format, orderStateLabel, planStateLabel } from './futures-labels'
import type { ChatMessage } from './use-futures-assistant'

interface PlanResult {
  realized_pnl: number
  closes: number
}

type DraftRecord = FuturesState['records'][string]

const when = (value: string | undefined) =>
  value
    ? new Date(value).toLocaleString(activeLanguage(), {
        month: 'short',
        day: 'numeric',
        hour: '2-digit',
        minute: '2-digit',
      })
    : ''

const resultTone = (pnl: number | undefined) =>
  pnl == null
    ? 'text-vergex-muted'
    : pnl >= 0
      ? 'text-vergex-success'
      : 'text-vergex-danger'

// What a plan's detail shows, finished or not: its levels, why it was made and a way back
// to the chart.
function PlanDetail({
  plan,
  state,
  result,
  busy,
  onChoose,
  orders,
}: {
  plan: Plan
  state: string
  result?: PlanResult
  busy: boolean
  onChoose: (plan: Plan) => void
  orders?: LiveRun['orders']
}) {
  const { language } = useLanguage()
  const label = (key: string) => t(`futures.${key}`, language)
  return (
    <Panel
      className="flex min-h-0 min-w-0 flex-col overflow-hidden"
      title={plan.idea.name}
      meta={[state, when(plan.created)].filter(Boolean).join(' · ')}
      actions={
        <Button
          type="button"
          size="sm"
          className="max-sm:h-10"
          disabled={busy}
          onClick={() => onChoose(plan)}
        >
          {label('account.viewOnChart')}
        </Button>
      }
    >
      <ScrollArea className="min-h-0 flex-1 rounded-b-2xl border-t border-vergex-line">
        <StatStrip
          stats={[
            ...(orders
              ? [
                  {
                    name: label('realized'),
                    value: fmtUsd(result?.realized_pnl, true),
                    gain: result?.realized_pnl ?? null,
                  },
                ]
              : []),
            { name: label('entry'), value: format(plan.idea.entry) },
            { name: label('stopLoss'), value: format(plan.idea.stop) },
            { name: label('takeProfit'), value: format(plan.idea.target) },
            ...(orders
              ? []
              : [
                  {
                    name: label('structures.plannedLoss'),
                    value: Number(plan.risk).toFixed(2),
                    note: plan.snapshot.quote_asset,
                  },
                ]),
          ]}
        />
        {plan.idea.rationale && (
          <p className="whitespace-pre-wrap break-words border-b border-vergex-line px-4 py-3 text-[13px] leading-relaxed text-vergex-ink-soft">
            {plan.idea.rationale}
          </p>
        )}
        {orders && (
          <table className={tableClass}>
            <thead>
              <tr>
                <th className={headClass}>{label('orderType')}</th>
                <th className={headClass}>{label('orderState')}</th>
                <th className={cn(headClass, 'text-right')}>
                  {label('filledQuantity')}
                </th>
                <th className={cn(headClass, 'max-sm:hidden')}>
                  {label('orderId')}
                </th>
              </tr>
            </thead>
            <tbody>
              {orders.map((order) => (
                <tr key={order.client_id}>
                  <td className={cn(cellClass, 'text-xs text-vergex-ink')}>
                    {order.role === 'entry'
                      ? label('entry')
                      : order.role === 'sl'
                        ? label('stopLoss')
                        : label('takeProfit')}
                  </td>
                  <td className={cn(cellClass, 'text-xs')}>
                    {orderStateLabel(order.status, language)}
                  </td>
                  <td className={cn(cellClass, numberClass)}>
                    {format(order.filled)}
                  </td>
                  <td
                    className={cn(
                      cellClass,
                      'mono truncate text-xs text-vergex-muted max-sm:hidden'
                    )}
                    title={order.order_id}
                  >
                    {order.order_id || label('unconfirmed')}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </ScrollArea>
    </Panel>
  )
}

// What was said in a past conversation, and a way to carry on with it on the trade tab.
function ConversationDetail({
  conversation,
  busy,
  onOpen,
}: {
  conversation: FuturesConversation
  busy: boolean
  onOpen: (conversation: FuturesConversation) => void
}) {
  const { language } = useLanguage()
  const label = (key: string) => t(`futures.${key}`, language)
  const { data, error } = useQuery({
    queryKey: ['futures-conversation', conversation.id],
    queryFn: () =>
      futuresAPI<{ messages: ChatMessage[] }>(
        `conversations/${conversation.id}`
      ),
    retry: false,
  })
  return (
    <Panel
      className="flex min-h-0 min-w-0 flex-col overflow-hidden"
      title={conversation.title || conversation.symbol}
      meta={[conversation.symbol, when(conversation.updated_at)]
        .filter(Boolean)
        .join(' · ')}
      actions={
        <Button
          type="button"
          size="sm"
          className="max-sm:h-10"
          disabled={busy}
          onClick={() => onOpen(conversation)}
        >
          {label('continueConversation')}
        </Button>
      }
    >
      <ScrollArea className="min-h-0 flex-1 rounded-b-2xl border-t border-vergex-line">
        {error ? (
          <PanelState role="alert">{label('conversationFailed')}</PanelState>
        ) : !data ? (
          <PanelState role="status">{label('loadingConversation')}</PanelState>
        ) : (
          <div className="flex flex-col gap-4 p-4">
            {data.messages.map((message, index) => (
              <Message key={index} message={message} />
            ))}
          </div>
        )}
      </ScrollArea>
    </Panel>
  )
}

function RailGroup({ children }: { children: string }) {
  return (
    <li className="px-3 pb-1 pt-4 text-[11px] font-medium text-vergex-muted first:pt-0">
      {children}
    </li>
  )
}

// FuturesHistory is what has already happened on this account and market: finished plans
// with what they realized, drafts from earlier sessions and past conversations, in one
// list beside the one opened. Results are only asked for while the tab is shown.
export function FuturesHistory({
  active,
  accountID,
  runs,
  archived,
  clock,
  conversations,
  busy,
  onChoose,
  onOpenConversation,
}: {
  active: boolean
  accountID: string
  runs: LiveRun[]
  archived: DraftRecord[]
  clock: number
  conversations: FuturesConversation[]
  busy: boolean
  onChoose: (plan: Plan) => void
  onOpenConversation: (conversation: FuturesConversation) => void
}) {
  const { language } = useLanguage()
  const label = (key: string) => t(`futures.${key}`, language)
  const { data: results } = useQuery({
    queryKey: ['plan-results', accountID, runs.length],
    queryFn: () =>
      futuresAPI<Record<string, PlanResult>>(
        'results?account_id=' + encodeURIComponent(accountID)
      ),
    enabled: Boolean(active && accountID && runs.length > 0),
    refetchOnWindowFocus: false,
    retry: false,
  })

  const entries = [
    ...runs.map((run) => ({ key: `run:${run.id}`, name: run.plan.idea.name })),
    ...archived.map((record) => ({
      key: `draft:${record.plan.id}`,
      name: record.plan.idea.name,
    })),
    ...conversations.map((conversation) => ({
      key: `chat:${conversation.id}`,
      name: conversation.title || conversation.symbol,
    })),
  ]
  const [picked, setPicked] = useState('')
  const open = entries.some((entry) => entry.key === picked)
    ? picked
    : (entries[0]?.key ?? '')
  useEffect(() => setPicked(open), [open])

  if (entries.length === 0) {
    return (
      <Panel>
        <PanelState role="status">{label('noHistory')}</PanelState>
      </Panel>
    )
  }

  const run = runs.find((row) => `run:${row.id}` === open)
  const draft = archived.find((row) => `draft:${row.plan.id}` === open)
  const conversation = conversations.find((row) => `chat:${row.id}` === open)
  const draftState = (record: DraftRecord) =>
    expired(record.plan, clock) && record.state === 'draft'
      ? label('card.expired')
      : planStateLabel(record.state, language)

  return (
    <SplitView
      rail={
        <SplitRail label={label('historyTab')}>
          {runs.length > 0 && <RailGroup>{label('finishedPlans')}</RailGroup>}
          {runs.map((row) => (
            <RailItem
              key={row.id}
              current={open === `run:${row.id}`}
              title={row.plan.idea.name}
              name={row.plan.idea.name}
              details={
                <span className="flex justify-between gap-2 text-xs font-normal text-vergex-muted">
                  <span className="truncate">
                    {planStateLabel(row.state, language)}
                  </span>
                  <span
                    className={cn(
                      'mono shrink-0',
                      resultTone(results?.[row.id]?.realized_pnl)
                    )}
                  >
                    {fmtUsd(results?.[row.id]?.realized_pnl, true)}
                  </span>
                </span>
              }
              onOpen={() => setPicked(`run:${row.id}`)}
            />
          ))}
          {archived.length > 0 && (
            <RailGroup>{label('earlierDrafts')}</RailGroup>
          )}
          {archived.map((record) => (
            <RailItem
              key={record.plan.id}
              current={open === `draft:${record.plan.id}`}
              title={record.plan.idea.name}
              name={record.plan.idea.name}
              details={
                <span className="truncate text-xs font-normal text-vergex-muted">
                  {t(
                    `futures.structure.${record.plan.idea.execution.kind}`,
                    language
                  )}{' '}
                  · {draftState(record)}
                </span>
              }
              onOpen={() => setPicked(`draft:${record.plan.id}`)}
            />
          ))}
          {conversations.length > 0 && (
            <RailGroup>{label('conversations')}</RailGroup>
          )}
          {conversations.map((row) => (
            <RailItem
              key={row.id}
              current={open === `chat:${row.id}`}
              title={row.title || row.symbol}
              name={row.title || row.symbol}
              details={
                <span className="truncate text-xs font-normal text-vergex-muted">
                  {row.symbol} · {when(row.updated_at)}
                </span>
              }
              onOpen={() => setPicked(`chat:${row.id}`)}
            />
          ))}
        </SplitRail>
      }
    >
      <div className="flex min-h-0 min-w-0 flex-col gap-3 max-lg:h-[80dvh] max-lg:min-h-96">
        <AppSelect
          aria-label={label('historyTab')}
          className="w-full lg:hidden max-sm:h-10"
          value={open}
          onChange={setPicked}
          options={entries.map((entry) => ({
            value: entry.key,
            label: entry.name,
          }))}
        />
        <div className="grid min-h-0 flex-1">
          {run ? (
            <PlanDetail
              plan={run.plan}
              state={planStateLabel(run.state, language)}
              result={results?.[run.id]}
              orders={run.orders}
              busy={busy}
              onChoose={onChoose}
            />
          ) : draft ? (
            <PlanDetail
              plan={draft.plan}
              state={draftState(draft)}
              busy={busy}
              onChoose={onChoose}
            />
          ) : (
            conversation && (
              <ConversationDetail
                conversation={conversation}
                busy={busy}
                onOpen={onOpenConversation}
              />
            )
          )}
        </div>
      </div>
    </SplitView>
  )
}
