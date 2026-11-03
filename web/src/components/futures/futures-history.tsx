import { useEffect, useState, type ReactNode } from 'react'
import { useQuery } from '@tanstack/react-query'
import {
  ChartCandlestick,
  ChevronRight,
  MessageSquare,
  Pencil,
  Trash2,
} from 'lucide-react'
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
import { EditableTitle, NameInput } from '@/components/common/name-editing'
import { confirmToast, notify } from '@/components/common/notify'
import { Panel, PanelLoading, PanelState } from '@/components/common/panel'
import { SearchField } from '@/components/common/search-field'
import {
  ActionMenu,
  RailItem,
  SplitRail,
  SplitView,
  type RailAction,
} from '@/components/common/split-view'
import { fmtUsd } from '@/utils/trader-format'
import { Button } from '@/components/ui/button'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Skeleton } from '@/components/ui/skeleton'
import { AppSelect } from '@/components/ui/select'
import { useLanguage } from '@/contexts/language-context'
import type { FuturesConversation } from '@/hooks/futures/use-futures-data'
import { activeLanguage, t } from '@/i18n'
import { cn } from '@/utils/cn'
import { format, orderStateLabel, planStateLabel } from './futures-labels'
import { Message, type ChatMessage } from '@/components/common/chat-thread'

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
  menu,
}: {
  plan: Plan
  state: string
  result?: PlanResult
  busy: boolean
  onChoose: (plan: Plan) => void
  orders?: LiveRun['orders']
  menu?: RailAction[]
}) {
  const { language } = useLanguage()
  const label = (key: string) => t(`futures.${key}`, language)
  return (
    <Panel
      className="flex min-h-0 min-w-0 flex-col overflow-hidden"
      singleLine
      title={
        <span className="block truncate" title={plan.idea.name}>
          {plan.idea.name}
        </span>
      }
      meta={[state, when(plan.created)].filter(Boolean).join(' · ')}
      actions={
        <>
          <Button
            type="button"
            size="sm"
            className="max-sm:h-10"
            disabled={busy}
            onClick={() => onChoose(plan)}
          >
            {label('account.viewOnChart')}
          </Button>
          {menu && (
            <ActionMenu
              label={label('historyActions')}
              actions={menu}
              className="size-8 max-sm:size-10"
            />
          )}
        </>
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

// What was said in a past conversation, and what can be done with it: carrying on with it
// on the trade tab, naming it and deleting it. Where the list is not beside it, this is
// the only place to do them.
function ConversationDetail({
  conversation,
  busy,
  onOpen,
  onRename,
  onDelete,
}: {
  conversation: FuturesConversation
  busy: boolean
  onOpen: (conversation: FuturesConversation) => void
  onRename: (title: string) => void
  onDelete: () => void
}) {
  const { language } = useLanguage()
  const label = (key: string) => t(`futures.${key}`, language)
  const [renaming, setRenaming] = useState(false)
  const name = conversation.title || conversation.symbol
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
      singleLine
      title={
        <EditableTitle
          name={name}
          editing={renaming}
          renameLabel={label('renameConversation')}
          onEdit={() => setRenaming(true)}
          field={
            <NameInput
              value={name}
              label={label('conversationName')}
              language={language}
              onSave={(title) => {
                setRenaming(false)
                onRename(title)
              }}
              onCancel={() => setRenaming(false)}
              className="max-w-64 md:max-w-96"
            />
          }
        />
      }
      meta={
        <span className={cn('block truncate', renaming && 'invisible')}>
          {[conversation.symbol, when(conversation.updated_at)]
            .filter(Boolean)
            .join(' · ')}
        </span>
      }
      actions={
        <>
          <Button
            type="button"
            size="sm"
            className="max-sm:h-10"
            disabled={busy}
            onClick={() => onOpen(conversation)}
          >
            {label('continueConversation')}
          </Button>
          <ActionMenu
            label={label('historyActions')}
            className="size-8 max-sm:size-10"
            actions={[
              {
                label: label('renameConversation'),
                icon: Pencil,
                movesFocus: true,
                onSelect: () => setRenaming(true),
              },
              {
                label: label('deleteConversation'),
                icon: Trash2,
                danger: true,
                onSelect: onDelete,
              },
            ]}
          />
        </>
      }
    >
      <ScrollArea className="min-h-0 flex-1 rounded-b-2xl border-t border-vergex-line">
        {error ? (
          <PanelState role="alert">{label('conversationFailed')}</PanelState>
        ) : !data ? (
          <PanelLoading />
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

// One folder of the directory: its name and how much is in it, opening and closing what
// is under it. A folder with nothing in it is not shown.
function RailFolder({
  name,
  count,
  open,
  onToggle,
  children,
}: {
  name: string
  count: number
  open: boolean
  onToggle: () => void
  children: ReactNode
}) {
  if (count === 0) return null
  return (
    <li className="pb-2 last:pb-0">
      <button
        type="button"
        aria-expanded={open}
        onClick={onToggle}
        className="flex h-8 w-full min-w-0 items-center gap-1.5 rounded-md px-2 text-left text-xs font-medium text-vergex-muted hover:bg-vergex-surface-muted hover:duration-0 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-vergex-ink/25"
      >
        <ChevronRight
          className={cn(
            'size-3.5 shrink-0 transition-transform',
            open && 'rotate-90'
          )}
          aria-hidden="true"
        />
        <span className="truncate">{name}</span>
        <span className="mono ml-auto shrink-0">{count}</span>
      </button>
      {open && (
        <ul className="ml-[15px] space-y-0.5 border-l border-vergex-line pl-2">
          {children}
        </ul>
      )}
    </li>
  )
}

// FuturesHistory is what has already happened on this account: its past conversations, and
// on this market the finished plans with what they realized and drafts from earlier sessions,
// as a directory beside the one opened. Conversations can be continued, renamed and
// deleted. A draft can be deleted, and so can a finished plan, whose orders stay in the
// account's records. Neither can be renamed, since a plan's name is part of what its
// approval signs. Results
// are only asked for while the tab is shown.
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
  onRenameConversation,
  onDeleteConversation,
  onDeletePlan,
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
  onRenameConversation: (id: string, title: string) => Promise<void>
  onDeleteConversation: (conversation: FuturesConversation) => Promise<void>
  onDeletePlan: (plan: Plan) => Promise<void>
}) {
  const { language } = useLanguage()
  const label = (key: string) => t(`futures.${key}`, language)
  const [closed, setClosed] = useState<string[]>([])
  const [renaming, setRenaming] = useState('')
  const [query, setQuery] = useState('')
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
    ...conversations.map((conversation) => ({
      key: `chat:${conversation.id}`,
      name: conversation.title || conversation.symbol,
    })),
    ...runs.map((run) => ({ key: `run:${run.id}`, name: run.plan.idea.name })),
    ...archived.map((record) => ({
      key: `draft:${record.plan.id}`,
      name: record.plan.idea.name,
    })),
  ]
  const [picked, setPicked] = useState('')
  const open = entries.some((entry) => entry.key === picked)
    ? picked
    : (entries[0]?.key ?? '')
  useEffect(() => setPicked(open), [open])

  const run = runs.find((row) => `run:${row.id}` === open)
  const draft = archived.find((row) => `draft:${row.plan.id}` === open)
  const conversation = conversations.find((row) => `chat:${row.id}` === open)
  const draftState = (record: DraftRecord) =>
    expired(record.plan, clock) && record.status === 'draft'
      ? label('card.expired')
      : planStateLabel(record.status, language)

  const needle = query.trim().toLowerCase()
  const matches = (...names: string[]) =>
    !needle || names.some((name) => name.toLowerCase().includes(needle))
  const shownChats = conversations.filter((row) =>
    matches(row.title, row.symbol)
  )
  const shownRuns = runs.filter((row) => matches(row.plan.idea.name))
  const shownDrafts = archived.filter((row) => matches(row.plan.idea.name))
  const folderProps = (key: string) => ({
    // A search shows what it found wherever it is.
    open: Boolean(needle) || !closed.includes(key),
    onToggle: () =>
      setClosed((keys) =>
        keys.includes(key)
          ? keys.filter((other) => other !== key)
          : [...keys, key]
      ),
  })

  const rename = async (id: string, title: string) => {
    setRenaming('')
    try {
      await onRenameConversation(id, title)
    } catch (e) {
      notify.error(e instanceof Error ? e.message : String(e))
    }
  }
  const remove = async (
    message: string,
    title: string,
    deleteIt: () => Promise<void>
  ) => {
    const ok = await confirmToast(message, {
      title,
      okText: title,
      cancelText: label('deleteCancel'),
      tone: 'danger',
    })
    if (!ok) return
    try {
      await deleteIt()
    } catch (e) {
      notify.error(e instanceof Error ? e.message : String(e))
    }
  }
  const removeConversation = (row: FuturesConversation) =>
    void remove(
      t('futures.deleteConversationMessage', language, {
        name: row.title || row.symbol,
      }),
      label('deleteConversation'),
      () => onDeleteConversation(row)
    )
  const draftActions = (record: DraftRecord): RailAction[] => [
    {
      label: label('deleteDraft'),
      icon: Trash2,
      danger: true,
      onSelect: () =>
        void remove(
          t('futures.deleteDraftMessage', language, {
            name: record.plan.idea.name,
          }),
          label('deleteDraft'),
          () => onDeletePlan(record.plan)
        ),
    },
  ]
  // What a finished plan sent stays in the account's records, so only Futures' own copy goes.
  const finishedActions = (row: LiveRun): RailAction[] => [
    {
      label: label('deletePlan'),
      icon: Trash2,
      danger: true,
      onSelect: () =>
        void remove(
          t('futures.deletePlanMessage', language, {
            name: row.plan.idea.name,
          }),
          label('deletePlan'),
          () => onDeletePlan(row.plan)
        ),
    },
  ]
  const viewOnChart = (plan: Plan): RailAction => ({
    label: label('account.viewOnChart'),
    icon: ChartCandlestick,
    disabled: busy,
    onSelect: () => onChoose(plan),
  })

  return (
    <SplitView
      rail={
        entries.length > 0 && (
          <SplitRail
            label={label('historyTab')}
            top={
              <SearchField
                label={label('historySearch')}
                value={query}
                onChange={setQuery}
              />
            }
          >
            {needle &&
              shownChats.length + shownRuns.length + shownDrafts.length ===
                0 && (
                <li className="px-2 py-2 text-xs text-vergex-muted">
                  {label('historyNoMatch')}
                </li>
              )}
            <RailFolder
              name={label('conversations')}
              count={shownChats.length}
              {...folderProps('chats')}
            >
              {shownChats.map((row) => (
                <RailItem
                  key={row.id}
                  current={open === `chat:${row.id}`}
                  title={row.title || row.symbol}
                  name={row.title || row.symbol}
                  details={
                    <span className="truncate text-xs font-normal text-vergex-muted">
                      {[row.symbol, when(row.updated_at)]
                        .filter(Boolean)
                        .join(' · ')}
                    </span>
                  }
                  onOpen={() => setPicked(`chat:${row.id}`)}
                  onStartRename={() => setRenaming(row.id)}
                  rename={
                    renaming === row.id && (
                      <NameInput
                        row
                        value={row.title || row.symbol}
                        label={label('conversationName')}
                        language={language}
                        onSave={(title) => void rename(row.id, title)}
                        onCancel={() => setRenaming('')}
                      />
                    )
                  }
                  actionsLabel={label('historyActions')}
                  actions={[
                    {
                      label: label('continueConversation'),
                      icon: MessageSquare,
                      disabled: busy,
                      onSelect: () => onOpenConversation(row),
                    },
                    {
                      label: label('renameConversation'),
                      icon: Pencil,
                      movesFocus: true,
                      onSelect: () => setRenaming(row.id),
                    },
                    {
                      label: label('deleteConversation'),
                      icon: Trash2,
                      danger: true,
                      onSelect: () => removeConversation(row),
                    },
                  ]}
                />
              ))}
            </RailFolder>
            <RailFolder
              name={label('finishedPlans')}
              count={shownRuns.length}
              {...folderProps('runs')}
            >
              {shownRuns.map((row) => (
                <RailItem
                  key={row.id}
                  current={open === `run:${row.id}`}
                  title={row.plan.idea.name}
                  name={row.plan.idea.name}
                  details={
                    <span className="flex justify-between gap-2 text-xs font-normal text-vergex-muted">
                      <span className="truncate">
                        {planStateLabel(row.status, language)}
                      </span>
                      <span
                        className={cn(
                          'mono shrink-0',
                          resultTone(results?.[row.id]?.realized_pnl)
                        )}
                      >
                        {results ? (
                          fmtUsd(results[row.id]?.realized_pnl, true)
                        ) : (
                          <Skeleton className="w-12" />
                        )}
                      </span>
                    </span>
                  }
                  onOpen={() => setPicked(`run:${row.id}`)}
                  actionsLabel={label('historyActions')}
                  actions={[viewOnChart(row.plan), ...finishedActions(row)]}
                />
              ))}
            </RailFolder>
            <RailFolder
              name={label('expiredDrafts')}
              count={shownDrafts.length}
              {...folderProps('drafts')}
            >
              {shownDrafts.map((record) => (
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
                  actionsLabel={label('historyActions')}
                  actions={[viewOnChart(record.plan), ...draftActions(record)]}
                />
              ))}
            </RailFolder>
          </SplitRail>
        )
      }
    >
      <div className="flex min-h-0 min-w-0 flex-col gap-3 max-lg:h-[80dvh] max-lg:min-h-96">
        {entries.length > 0 && (
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
        )}
        <div className="grid min-h-0 flex-1">
          {entries.length === 0 ? (
            <Panel className="flex flex-col justify-center">
              <PanelState role="status">{label('noHistory')}</PanelState>
            </Panel>
          ) : run ? (
            <PlanDetail
              plan={run.plan}
              state={planStateLabel(run.status, language)}
              result={results?.[run.id]}
              orders={run.orders}
              busy={busy}
              onChoose={onChoose}
              menu={finishedActions(run)}
            />
          ) : draft ? (
            <PlanDetail
              plan={draft.plan}
              state={draftState(draft)}
              busy={busy}
              onChoose={onChoose}
              menu={draftActions(draft)}
            />
          ) : (
            conversation && (
              <ConversationDetail
                key={conversation.id}
                conversation={conversation}
                busy={busy}
                onOpen={onOpenConversation}
                onRename={(title) => void rename(conversation.id, title)}
                onDelete={() => removeConversation(conversation)}
              />
            )
          )}
        </div>
      </div>
    </SplitView>
  )
}
