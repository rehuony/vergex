import { apiKeyVenues, venueName } from '@/utils/futures/futures-accounts'
import { useEffect, useRef, useState, type ReactNode } from 'react'
import { ChevronRight, TriangleAlert } from 'lucide-react'
import { api } from '@/api'
import {
  futuresAPI,
  CloseNotSubmittedError,
  type LiveRun,
  type FuturesState,
} from '@/api/futures'
import {
  accountPositions,
  closeAccountPosition,
  positionDecimal,
  nativeMarket,
  type AccountPosition,
} from '@/utils/futures/futures-account'
import { accountLabel } from '@/utils/futures/futures-accounts'
import { Dialog } from '@/components/common/dialog'
import {
  cellClass,
  EmptyLine,
  gainTone,
  headClass,
  numberClass,
  StatStrip,
  tableClass,
} from '@/components/common/figures'
import { Panel, PanelState } from '@/components/common/panel'
import { SettingsLink } from '@/components/settings/settings-link'
import { Button, buttonVariants } from '@/components/ui/button'
import { ScrollArea } from '@/components/ui/scroll-area'
import type { AccountWatch } from '@/hooks/futures/use-account-watch'
import type { Account } from '@/types'
import { cn } from '@/utils/cn'
import type { AccountCheck } from '@/utils/futures/futures-workspace'
import { useFuturesLabel } from './use-futures-label'

const format = (value: number | string) =>
  Number(value).toLocaleString('en-US', { maximumFractionDigits: 8 })
const amount = (value: number) =>
  Number.isFinite(value)
    ? value.toLocaleString('en-US', {
        minimumFractionDigits: 2,
        maximumFractionDigits:
          Math.abs(value) > 0 && Math.abs(value) < 0.01 ? 4 : 2,
      })
    : '—'
const signed = (value: number) => `${value > 0 ? '+' : ''}${amount(value)}`
// On a phone the panel's tables keep their columns, and scroll sideways with it.
const narrowTable = 'max-sm:min-w-[520px]'
const venueURL = (venue: string) =>
  venue === 'okx'
    ? 'https://www.okx.com/trade-swap/btc-usdt-swap'
    : 'https://www.binance.com/en/futures'
const liveStates = new Set([
  'starting',
  'running',
  'position_open',
  'stopping',
  'needs_attention',
  'approved',
])

export function FuturesAccount({
  account,
  accounts,
  accountCheck,
  venue,
  symbol,
  runs,
  busy,
  onView,
  onManual,
  onBusy,
  onRefresh,
  onCheck,
  watch,
}: {
  account?: Account
  accounts: Account[]
  accountCheck?: AccountCheck | null
  venue: string
  symbol: string
  runs: LiveRun[]
  busy: boolean
  onView: (run: LiveRun) => void
  onManual: (side: 'long' | 'short') => void
  onBusy: (value: boolean) => void
  onRefresh: () => Promise<void>
  onCheck?: (run: LiveRun) => Promise<void>
  watch: AccountWatch
}) {
  const label = useFuturesLabel('account')
  const [confirm, setConfirm] = useState<AccountPosition | null>(null),
    [notice, setNotice] = useState('')
  const [attempted, setAttempted] = useState<Set<string>>(new Set())
  const attemptLatch = useRef(new Set<string>())
  const accountID = account?.id || ''
  const { snapshot, orders, binding, loading, error, ordersFailed, refresh } =
    watch
  const orderError = ordersFailed ? label('ordersUnverified') : ''

  useEffect(() => {
    setConfirm(null)
    setNotice('')
  }, [accountID, symbol])

  // A close attempt stays latched until the exchange shows the position gone.
  useEffect(() => {
    if (snapshot?.accountID !== accountID) return
    for (const attempt of attemptLatch.current) {
      if (
        attempt.startsWith(accountID + '/') &&
        !snapshot.positions.some(
          (p) => attempt === accountID + '/' + p.symbol + '/' + p.side
        )
      )
        attemptLatch.current.delete(attempt)
    }
    setAttempted(new Set(attemptLatch.current))
  }, [snapshot, accountID])

  const positions = snapshot?.accountID === accountID ? snapshot.positions : []
  const currentBinding = binding?.accountID === accountID ? binding : null
  const hasPosition = positions.some(
    (p) => nativeMarket(p.symbol, venue) === nativeMarket(symbol, venue)
  )
  const open =
    orders?.accountID === accountID && orders.symbol === symbol
      ? orders.rows
      : []
  const importantRuns = runs
    .filter((run) => run.account_id === accountID && liveStates.has(run.state))
    .sort(
      (a, b) =>
        Number(b.state === 'needs_attention') -
        Number(a.state === 'needs_attention')
    )
  const key = (p: AccountPosition) => accountID + '/' + p.symbol + '/' + p.side

  async function closePosition() {
    if (
      !confirm ||
      !accountID ||
      (!apiKeyVenues.includes(venue) && !currentBinding) ||
      busy
    )
      return
    const position = confirm,
      traderID = currentBinding?.id,
      attempt = key(position)
    if (attemptLatch.current.has(attempt)) return
    attemptLatch.current.add(attempt)
    setAttempted(new Set(attemptLatch.current))
    setConfirm(null)
    onBusy(true)
    setNotice(label('stoppingEntries'))
    let closeSent = false
    let closeMessage = ''
    try {
      const traders = await api.getTraders(true)
      if (
        !apiKeyVenues.includes(venue) &&
        !traders.some(
          (t) => t.trader_id === traderID && t.account_id === accountID
        )
      )
        throw new Error(label('bindingChanged'))
      if (traders.some((t) => t.account_id === accountID && t.is_running))
        throw new Error(label('stopTraderFirst'))
      await closeAccountPosition(accountID, venue, position, runs, {
        stop: (run) =>
          futuresAPI('stop', { id: run.id, hash: run.approval_hash }),
        read: async () => (await futuresAPI<FuturesState>('state')).runs,
        wait: () => new Promise((resolve) => setTimeout(resolve, 500)),
        close: async () => {
          const fresh = accountPositions(
            await api.getAccountPositions(accountID, true)
          ).find(
            (p) => p.symbol === position.symbol && p.side === position.side
          )
          if (!fresh) throw new Error(label('noLongerOpen'))
          if (
            fresh.quantity > position.quantity ||
            fresh.entry !== position.entry
          )
            throw new Error(label('positionChanged'))
          if (apiKeyVenues.includes(venue)) {
            const quantity = positionDecimal(fresh.quantity)
            const entryPrice = positionDecimal(fresh.entry)
            closeSent = true
            const result = await futuresAPI<{ message: string }>(
              'close-position',
              {
                account_id: accountID,
                request_id: crypto.randomUUID(),
                symbol: position.symbol,
                side: position.side,
                quantity,
                entry_price: entryPrice,
              }
            )
            closeMessage = result.message
            return result
          }
          closeSent = true
          return api.closePosition(
            traderID!,
            position.symbol,
            position.side.toUpperCase()
          )
        },
      })
      setNotice(closeMessage || label('closeAccepted'))
    } catch (e) {
      const notSubmitted = !closeSent || e instanceof CloseNotSubmittedError
      if (notSubmitted) {
        attemptLatch.current.delete(attempt)
        setAttempted(new Set(attemptLatch.current))
      }
      setNotice(
        `${e instanceof Error ? e.message : String(e)} ${label(
          notSubmitted ? 'noCloseSent' : 'checkBeforeClosing'
        )}`
      )
    } finally {
      onBusy(false)
      await Promise.allSettled([refresh(), onRefresh()])
    }
  }

  if (!account) {
    return (
      <div role="region" aria-label={label('heading')}>
        <Panel>
          <PanelState role="status">
            {label('noAccount', { venue: venueName(venue) })}
            <SettingsLink
              section="accounts"
              className={buttonVariants({ variant: 'secondary', size: 'sm' })}
            >
              {label('addAccount')}
            </SettingsLink>
          </PanelState>
        </Panel>
      </div>
    )
  }

  const read = snapshot?.accountID === accountID
  const verified =
    accountCheck?.accountID === accountID && accountCheck.status === 'verified'
      ? accountCheck
      : null
  const unrealized = positions.reduce((sum, p) => sum + p.pnl, 0)
  const cantEnter = busy || !!currentBinding?.running || hasPosition
  const ordersRead = orders?.accountID === accountID && orders.symbol === symbol
  const exchangeLink = (text: string) => (
    <a
      href={venueURL(venue)}
      target="_blank"
      rel="noreferrer"
      className="text-vergex-ink underline"
    >
      {text}
    </a>
  )

  return (
    <div
      role="region"
      aria-label={label('heading')}
      className="flex min-h-0 flex-col max-lg:h-[80dvh] max-lg:min-h-96 lg:h-[max(520px,calc(100dvh-181px))]"
    >
      <Panel
        className="flex min-h-0 flex-1 flex-col overflow-hidden"
        title={accountLabel(account, accounts)}
        meta={[
          venueName(venue),
          read
            ? label(error ? 'lastReceived' : 'received', {
                time: new Date(snapshot.at).toLocaleTimeString(),
              })
            : label('waiting'),
        ].join(' · ')}
        actions={
          <>
            <a
              href={venueURL(venue)}
              target="_blank"
              rel="noreferrer"
              className={buttonVariants({
                variant: 'ghost',
                size: 'sm',
                className: 'max-sm:hidden',
              })}
            >
              {label('openAccount')}
            </a>
            <Button
              size="sm"
              className="max-sm:h-10"
              disabled={busy || loading}
              onClick={() => void refresh()}
            >
              {loading ? label('checking') : label('refresh')}
            </Button>
            <Button
              variant="success"
              size="sm"
              className="max-sm:h-10"
              disabled={cantEnter}
              title={hasPosition ? label('hasPosition', { symbol }) : undefined}
              onClick={() => onManual('long')}
            >
              {label('buyLong')}
            </Button>
            <Button
              variant="danger"
              size="sm"
              className="max-sm:h-10"
              disabled={cantEnter}
              title={hasPosition ? label('hasPosition', { symbol }) : undefined}
              onClick={() => onManual('short')}
            >
              {label('sellShort')}
            </Button>
          </>
        }
      >
        <StatStrip
          className="border-t"
          stats={[
            {
              name: label('equity'),
              value: verified ? amount(Number(verified.equity)) : '—',
              note: 'USDT',
            },
            {
              name: label('available'),
              value: verified ? amount(Number(verified.available)) : '—',
              note: 'USDT',
            },
            {
              name: label('unrealized'),
              value: read ? signed(unrealized) : '—',
              note: 'USDT',
              gain: read ? unrealized : null,
            },
            {
              name: label('openPositions'),
              value: read ? positions.length : '—',
              note: label('allMarkets'),
            },
          ]}
        />
        <ScrollArea
          className="min-h-0 flex-1 rounded-b-2xl"
          viewportClassName="max-sm:overflow-x-auto"
        >
          {currentBinding?.running && <Note>{label('traderRunning')}</Note>}
          {hasPosition &&
            !error &&
            !orderError &&
            ordersRead &&
            open.length === 0 && (
              <Note alert>
                {label('noProtection', { symbol })}{' '}
                {exchangeLink(label('checkProtection'))}
              </Note>
            )}
          {notice && <Note status>{notice}</Note>}
          {error && (
            <Note alert>
              {label(snapshot ? 'positionsStale' : 'positionsUnread', {
                error,
              })}
            </Note>
          )}
          <table className={cn(tableClass, narrowTable)}>
            <thead className="sticky top-0 z-10">
              <tr>
                <th className={headClass}>{label('market')}</th>
                <th className={cn(headClass, 'w-20')}>{label('side')}</th>
                <th className={cn(headClass, 'w-24 text-right')}>
                  {label('positionValue')}
                </th>
                <th
                  className={cn(headClass, 'w-28 text-right max-md:hidden')}
                  title={label('marginTitle')}
                >
                  {label('estMargin')}
                </th>
                <th className={cn(headClass, 'w-28 text-right max-md:hidden')}>
                  {label('quantity')}
                </th>
                <th className={cn(headClass, 'w-24 text-right max-sm:hidden')}>
                  {label('avgEntry')}
                </th>
                <th className={cn(headClass, 'w-24 text-right max-sm:hidden')}>
                  {label('markPrice')}
                </th>
                <th className={cn(headClass, 'w-24 text-right')}>
                  {label('unrealized')}
                </th>
                <th className={cn(headClass, 'w-28')}>
                  <span className="sr-only">{label('closePosition')}</span>
                </th>
              </tr>
            </thead>
            <tbody>
              {positions.map((p) => (
                <tr key={p.symbol + p.side}>
                  <td className={cellClass}>
                    <strong className="block truncate font-medium text-vergex-ink">
                      {nativeMarket(p.symbol, venue)}
                    </strong>
                  </td>
                  <td
                    className={cn(
                      cellClass,
                      'text-xs',
                      p.side === 'long'
                        ? 'text-vergex-success'
                        : 'text-vergex-danger'
                    )}
                  >
                    {label(p.side === 'long' ? 'long' : 'short')}{' '}
                    <span className="mono">{p.leverage}x</span>
                  </td>
                  <td className={cn(cellClass, numberClass)}>
                    {amount(p.quantity * p.mark)}
                  </td>
                  <td
                    className={cn(cellClass, numberClass, 'max-md:hidden')}
                    title={label('marginCellTitle')}
                  >
                    ≈ {amount((p.quantity * p.mark) / p.leverage)}
                  </td>
                  <td
                    className={cn(
                      cellClass,
                      numberClass,
                      'truncate max-md:hidden'
                    )}
                    title={format(p.quantity)}
                  >
                    {format(p.quantity)}
                  </td>
                  <td className={cn(cellClass, numberClass, 'max-sm:hidden')}>
                    {format(p.entry)}
                  </td>
                  <td className={cn(cellClass, numberClass, 'max-sm:hidden')}>
                    {format(p.mark)}
                  </td>
                  <td className={cn(cellClass, numberClass, gainTone(p.pnl))}>
                    {signed(p.pnl)}
                  </td>
                  <td className={cn(cellClass, 'text-right')}>
                    {!currentBinding && !apiKeyVenues.includes(venue) ? (
                      exchangeLink(label('closeOnExchange'))
                    ) : (
                      <Button
                        variant="danger"
                        size="sm"
                        className="h-7 px-2.5 text-xs max-sm:h-10"
                        disabled={
                          busy ||
                          !!error ||
                          !!currentBinding?.running ||
                          attempted.has(key(p))
                        }
                        onClick={() => setConfirm(p)}
                      >
                        {attempted.has(key(p))
                          ? label('verifyClose')
                          : label('closePosition')}
                      </Button>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          {positions.length === 0 && (read ? !error : loading) && (
            <EmptyLine>
              {read ? label('noPositions') : label('readingPositions')}
            </EmptyLine>
          )}
          <table className={cn(tableClass, narrowTable, 'mt-4')}>
            <thead>
              <tr>
                <th className={cn(headClass, 'border-t')}>
                  {label('exchangeOrders', { symbol })}
                </th>
                <th className={cn(headClass, 'w-20 border-t')}>
                  {label('side')}
                </th>
                <th className={cn(headClass, 'w-28 border-t text-right')}>
                  {label('priceTrigger')}
                </th>
                <th className={cn(headClass, 'w-24 border-t text-right')}>
                  {label('sizeHead')}
                </th>
                <th className={cn(headClass, 'w-24 border-t max-sm:hidden')}>
                  {label('status')}
                </th>
                <th className={cn(headClass, 'w-40 border-t max-md:hidden')}>
                  {label('orderId')}
                </th>
              </tr>
            </thead>
            <tbody>
              {!orderError &&
                ordersRead &&
                open.map((o) => (
                  <tr key={o.order_id}>
                    <td className={cn(cellClass, 'text-xs text-vergex-ink')}>
                      {o.type.includes('TAKE_PROFIT')
                        ? label('takeProfit')
                        : o.type.includes('STOP')
                          ? label('stopLoss')
                          : o.type}
                    </td>
                    <td className={cn(cellClass, 'text-xs')}>{o.side}</td>
                    <td className={cn(cellClass, numberClass)}>
                      {format(o.stop_price || o.price)}
                    </td>
                    <td className={cn(cellClass, numberClass)}>
                      {format(o.quantity)}
                    </td>
                    <td className={cn(cellClass, 'text-xs max-sm:hidden')}>
                      {o.status}
                    </td>
                    <td
                      className={cn(
                        cellClass,
                        'mono truncate text-xs text-vergex-muted max-md:hidden'
                      )}
                      title={o.order_id}
                    >
                      {o.order_id}
                    </td>
                  </tr>
                ))}
            </tbody>
          </table>
          {orderError ? (
            <EmptyLine>{orderError}</EmptyLine>
          ) : !ordersRead ? (
            <EmptyLine>{label('ordersNotVerified')}</EmptyLine>
          ) : (
            open.length === 0 && (
              <EmptyLine>{label('noOpenOrders', { symbol })}</EmptyLine>
            )
          )}
          <table className={cn(tableClass, narrowTable, 'mt-4')}>
            <thead>
              <tr>
                <th className={cn(headClass, 'border-t')}>
                  {label('executions')}
                </th>
                <th className={cn(headClass, 'border-t max-sm:hidden')}>
                  {label('status')}
                </th>
                <th className={cn(headClass, 'w-24 border-t text-right')}>
                  {label('filledSize')}
                </th>
                <th className={cn(headClass, 'w-72 border-t max-sm:w-28')}>
                  <span className="sr-only">{label('checkTrade')}</span>
                </th>
              </tr>
            </thead>
            {importantRuns.map((run) => (
              <RunRows
                key={run.id}
                run={run}
                accounts={accounts}
                positions={positions}
                accountVerified={
                  snapshot?.accountID === run.account_id && !error
                }
                busy={busy}
                onCheck={onCheck}
                onView={onView}
              />
            ))}
          </table>
          {importantRuns.length === 0 && (
            <EmptyLine>{label('noActive')}</EmptyLine>
          )}
        </ScrollArea>
      </Panel>
      {confirm && (
        <Dialog
          title={label('closeTitle', {
            market: nativeMarket(confirm.symbol, venue),
            side: confirm.side,
          })}
          description={label('closeDescription', {
            account: account ? accountLabel(account, accounts) : '',
            venue,
            quantity: format(confirm.quantity),
          })}
          size="md"
          onClose={() => setConfirm(null)}
          footer={
            <>
              <Button type="button" onClick={() => setConfirm(null)}>
                {label('keepPosition')}
              </Button>
              <Button
                type="button"
                variant="destructive"
                disabled={busy}
                onClick={() => void closePosition()}
              >
                {label('confirmClose')}
              </Button>
            </>
          }
        >
          <p className="text-[13px] leading-relaxed text-vergex-muted">
            {label('closeBody')}
          </p>
        </Dialog>
      )}
    </div>
  )
}

// A line above the tables, for what is going on with the account.
function Note({
  alert,
  status,
  children,
}: {
  alert?: boolean
  status?: boolean
  children: ReactNode
}) {
  return (
    <p
      role={alert ? 'alert' : status ? 'status' : undefined}
      className={cn(
        'flex items-start gap-2 border-b border-vergex-line px-4 py-2 text-xs leading-relaxed',
        alert ? 'text-vergex-warning' : 'text-vergex-muted'
      )}
    >
      {alert && (
        <TriangleAlert
          className="mt-0.5 size-3.5 shrink-0"
          aria-hidden="true"
        />
      )}
      <span className="min-w-0">{children}</span>
    </p>
  )
}

// One plan still at work on the account: how it stands, what can be done about it and,
// opened, the orders recorded for it.
function RunRows({
  run,
  accounts,
  positions,
  accountVerified,
  busy,
  onCheck,
  onView,
}: {
  run: LiveRun
  accounts: Account[]
  positions: AccountPosition[]
  accountVerified: boolean
  busy: boolean
  onCheck?: (run: LiveRun) => Promise<void>
  onView: (run: LiveRun) => void
}) {
  const label = useFuturesLabel('account')
  const [opened, setOpened] = useState(false)
  const entries = run.orders.filter((o) => o.role === 'entry')
  const filled = entries.reduce((sum, o) => sum + Number(o.filled), 0)
  const accepted = entries.some(
    (o) => o.order_id && ['NEW', 'PARTIALLY_FILLED'].includes(o.status)
  )
  const unconfirmed = entries.some((o) =>
    ['UNKNOWN', 'submitting'].includes(o.status)
  )
  const name =
    accounts.find((a) => a.id === run.account_id)?.name ||
    label('accountNotLoaded')
  const nativePosition = accountVerified
    ? positions.find(
        (p) =>
          nativeMarket(p.symbol, run.plan.snapshot.venue) ===
            nativeMarket(run.plan.snapshot.symbol, run.plan.snapshot.venue) &&
          p.side === run.plan.idea.side
      )
    : undefined
  const stateLabel = label(
    unconfirmed
      ? 'stateUnconfirmed'
      : filled > 0
        ? nativePosition
          ? 'statePositionOpen'
          : accountVerified
            ? 'stateFilledNoPosition'
            : 'stateFilledUnverified'
        : accepted
          ? 'stateWorking'
          : run.state === 'approved' && !entries.length
            ? 'stateApprovedOnly'
            : 'stateNoFill'
  )
  const attention = run.state === 'needs_attention' || unconfirmed
  const headline = unconfirmed
    ? label('submissionUnconfirmed')
    : run.state === 'needs_attention'
      ? label('monitorReview')
      : stateLabel
  return (
    <tbody>
      <tr>
        <td className={cellClass}>
          <button
            type="button"
            aria-expanded={opened}
            onClick={() => setOpened(!opened)}
            className="flex w-full min-w-0 items-center gap-1.5 text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-vergex-ink/25"
          >
            <ChevronRight
              className={cn(
                'size-3.5 shrink-0 text-vergex-muted transition-transform',
                opened && 'rotate-90'
              )}
              aria-hidden="true"
            />
            <span className="truncate font-medium text-vergex-ink">
              {run.plan.snapshot.symbol} ·{' '}
              {label(run.plan.idea.side === 'long' ? 'long' : 'short')}
            </span>
            <span className="truncate text-xs text-vergex-muted max-md:hidden">
              {label('executionRecord', { name })}
            </span>
          </button>
        </td>
        <td
          className={cn(
            cellClass,
            'truncate text-xs max-sm:hidden',
            attention && 'text-vergex-warning'
          )}
          title={stateLabel}
        >
          {headline}
        </td>
        <td className={cn(cellClass, numberClass)}>
          {filled > 0 ? format(filled) : '—'}
        </td>
        <td className={cn(cellClass, 'text-right')}>
          <span className="flex justify-end gap-2">
            {onCheck &&
              ['needs_attention', 'position_open', 'stopping'].includes(
                run.state
              ) && (
                <Button
                  size="sm"
                  className="h-7 px-2.5 text-xs max-sm:h-10"
                  disabled={busy}
                  onClick={() => void onCheck(run)}
                >
                  {label('checkTrade')}
                </Button>
              )}
            <Button
              size="sm"
              className="h-7 px-2.5 text-xs max-sm:h-10"
              disabled={busy}
              onClick={() => onView(run)}
            >
              {label('viewOnChart')}
            </Button>
          </span>
        </td>
      </tr>
      {run.state === 'needs_attention' && (
        <tr>
          <td
            colSpan={4}
            className="border-b border-vergex-line px-4 py-2 text-xs text-vergex-warning"
          >
            {run.error?.startsWith('Service restarted')
              ? label('serverRestarted')
              : run.error || label('monitorNeedsReview')}
          </td>
        </tr>
      )}
      {opened && (
        <tr>
          <td colSpan={4} className="border-b border-vergex-line p-0">
            <table className={tableClass}>
              <thead>
                <tr>
                  <th className={headClass}>{label('order')}</th>
                  <th className={cn(headClass, 'text-right')}>
                    {label('price')}
                  </th>
                  <th className={cn(headClass, 'text-right')}>
                    {label('filledSize')}
                  </th>
                  <th className={headClass}>{label('lastState')}</th>
                  <th className={cn(headClass, 'max-sm:hidden')}>
                    {label('accountId')}
                  </th>
                </tr>
              </thead>
              <tbody>
                {run.orders.map((o) => (
                  <tr key={o.client_id}>
                    <td className={cn(cellClass, 'text-xs')}>
                      {o.role === 'entry'
                        ? label('entry')
                        : o.role === 'sl'
                          ? label('stopLoss')
                          : label('takeProfit')}
                    </td>
                    <td className={cn(cellClass, numberClass)}>
                      {format(o.average || o.price)}
                    </td>
                    <td className={cn(cellClass, numberClass)}>
                      {format(o.filled)} / {format(o.quantity)}
                    </td>
                    <td className={cn(cellClass, 'text-xs')}>{o.status}</td>
                    <td
                      className={cn(
                        cellClass,
                        'mono truncate text-xs text-vergex-muted max-sm:hidden'
                      )}
                      title={o.order_id}
                    >
                      {o.order_id || label('notConfirmed')}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
            <p className="px-4 py-2 text-[11px] text-vergex-muted">
              {label('recordsNote')}
            </p>
          </td>
        </tr>
      )}
    </tbody>
  )
}
