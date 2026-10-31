import { apiKeyVenues, venueName } from '@/utils/planning/frontier-accounts'
import { useEffect, useRef, useState } from 'react'
import { api } from '@/api'
import {
  planningAPI,
  CloseNotSubmittedError,
  type LiveRun,
  type PlanningState,
} from '@/api/planning'
import {
  accountPositions,
  closeAccountPosition,
  positionDecimal,
  nativeMarket,
  type AccountPosition,
} from '@/utils/planning/frontier-account'
import { accountLabel } from '@/utils/planning/frontier-accounts'
import { Dialog } from '@/components/common/dialog'
import { PanelState } from '@/components/common/panel'
import { SettingsLink } from '@/components/settings/settings-link'
import { Button, buttonVariants } from '@/components/ui/button'
import type { AccountWatch } from '@/hooks/planning/use-account-watch'
import type { Exchange } from '@/types'
import { useFrontierLabel } from './use-frontier-label'

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
const exchangeURL = (venue: string) =>
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

export function FrontierAccount({
  account,
  accounts,
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
  account?: Exchange
  accounts: Exchange[]
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
  const label = useFrontierLabel('account')
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
    .filter((run) => run.exchange_id === accountID && liveStates.has(run.state))
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
          (t) => t.trader_id === traderID && t.exchange_id === accountID
        )
      )
        throw new Error(label('bindingChanged'))
      if (traders.some((t) => t.exchange_id === accountID && t.is_running))
        throw new Error(label('stopTraderFirst'))
      await closeAccountPosition(accountID, venue, position, runs, {
        stop: (run) =>
          planningAPI('stop', { id: run.id, hash: run.approval_hash }),
        read: async () => (await planningAPI<PlanningState>('state')).runs,
        wait: () => new Promise((resolve) => setTimeout(resolve, 500)),
        close: async () => {
          const fresh = accountPositions(
            await api.getPositions(undefined, true, accountID)
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
            const result = await planningAPI<{ message: string }>(
              'close-position',
              {
                exchange_id: accountID,
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
      <section className="frontier-account" aria-label={label('heading')}>
        <PanelState role="status">
          {label('noAccount', { venue: venueName(venue) })}
          <SettingsLink
            section="exchanges"
            className={buttonVariants({ variant: 'secondary', size: 'sm' })}
          >
            {label('addAccount')}
          </SettingsLink>
        </PanelState>
      </section>
    )
  }

  return (
    <section className="frontier-account" aria-label={label('heading')}>
      <header>
        <div>
          <h2>
            {label('title')}{' '}
            <span className="frontier-count">
              {snapshot?.accountID === accountID ? positions.length : '—'}
            </span>
          </h2>
          <p>{`${accountLabel(account, accounts)} · ${venueName(venue)}`}</p>
        </div>
        <div className="frontier-account-actions">
          <a href={exchangeURL(venue)} target="_blank" rel="noreferrer">
            {label('openExchange')}
          </a>
          <Button
            size="sm"
            className="h-8 text-xs"
            disabled={busy || loading}
            onClick={() => void refresh()}
          >
            {loading ? label('checking') : label('refresh')}
          </Button>
          <Button
            variant="success"
            size="sm"
            className="h-8 bg-vergex-success/10 text-xs font-semibold"
            disabled={busy || !!currentBinding?.running || hasPosition}
            onClick={() => onManual('long')}
          >
            {label('buyLong')}
          </Button>
          <Button
            variant="danger"
            size="sm"
            className="h-8 bg-vergex-danger/10 text-xs font-semibold"
            disabled={busy || !!currentBinding?.running || hasPosition}
            onClick={() => onManual('short')}
          >
            {label('sellShort')}
          </Button>
        </div>
      </header>
      {currentBinding?.running && (
        <p className="frontier-notice">{label('traderRunning')}</p>
      )}
      {hasPosition && <small>{label('hasPosition', { symbol })}</small>}
      {hasPosition &&
        !error &&
        !orderError &&
        orders?.accountID === accountID &&
        orders.symbol === symbol &&
        open.length === 0 && (
          <p role="alert" className="frontier-notice">
            {label('noProtection', { symbol })}{' '}
            <a href={exchangeURL(venue)} target="_blank" rel="noreferrer">
              {label('checkProtection')}
            </a>
          </p>
        )}
      {notice && (
        <p role="status" className="frontier-notice">
          {notice}
        </p>
      )}
      {error && (
        <p role="alert" className="frontier-notice">
          {label(snapshot ? 'positionsStale' : 'positionsUnread', { error })}
        </p>
      )}
      <div className="frontier-account-subhead">
        <span>{label('allMarkets')}</span>
        <span>
          {snapshot?.accountID === accountID
            ? label(error ? 'lastReceived' : 'received', {
                time: new Date(snapshot.at).toLocaleTimeString(),
              })
            : label('waiting')}
        </span>
      </div>
      {positions.length > 0 ? (
        <div className="frontier-table-scroll">
          <table>
            <thead>
              <tr>
                <th>{label('market')}</th>
                <th>{label('positionValue')}</th>
                <th title={label('marginTitle')}>{label('estMargin')}</th>
                <th>{label('quantity')}</th>
                <th>{label('avgEntry')}</th>
                <th>{label('markPrice')}</th>
                <th>{label('unrealized')}</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {positions.map((p) => (
                <tr key={p.symbol + p.side}>
                  <td>
                    <strong>{nativeMarket(p.symbol, venue)}</strong>
                    <span className={'frontier-position-badge ' + p.side}>
                      {label(p.side === 'long' ? 'long' : 'short')}{' '}
                      <b>{p.leverage}x</b>
                    </span>
                  </td>
                  <td className="frontier-position-value">
                    <strong>{amount(p.quantity * p.mark)}</strong>
                    <small>{label('notional')}</small>
                  </td>
                  <td
                    className="frontier-position-margin"
                    title={label('marginCellTitle')}
                  >
                    <strong>
                      ≈ {amount((p.quantity * p.mark) / p.leverage)}
                    </strong>
                    <small>{label('estimated')}</small>
                  </td>
                  <td>
                    {format(p.quantity)}{' '}
                    <small>
                      {nativeMarket(p.symbol, venue).replace(/USDT$/, '')}
                    </small>
                  </td>
                  <td>{format(p.entry)}</td>
                  <td>{format(p.mark)}</td>
                  <td className={p.pnl >= 0 ? 'target-text' : 'stop-text'}>
                    {p.pnl >= 0 ? '+' : ''}
                    {amount(p.pnl)} USDT
                  </td>
                  <td>
                    {!currentBinding && !apiKeyVenues.includes(venue) ? (
                      <a
                        href={exchangeURL(venue)}
                        target="_blank"
                        rel="noreferrer"
                      >
                        {label('closeOnExchange')}
                      </a>
                    ) : (
                      <Button
                        variant="danger"
                        size="sm"
                        className="h-7 px-2.5 text-xs"
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
        </div>
      ) : snapshot?.accountID === accountID && !error ? (
        <p className="frontier-flat">{label('noPositions')}</p>
      ) : loading ? (
        <p>{label('readingPositions')}</p>
      ) : null}
      {positions.length > 0 && (
        <p className="frontier-margin-note">{label('marginNote')}</p>
      )}
      {open.filter((o) => /STOP|TAKE_PROFIT/.test(o.type)).length > 0 && (
        <div className="frontier-protection">
          <strong>{label('exitOrders', { symbol })}</strong>
          {open
            .filter((o) => /STOP|TAKE_PROFIT/.test(o.type))
            .map((o) => (
              <span key={o.order_id}>
                {o.type.includes('TAKE_PROFIT')
                  ? label('takeProfit')
                  : label('stopLoss')}{' '}
                <b>{format(o.stop_price || o.price)}</b> · {label('size')}{' '}
                {format(o.quantity)}
              </span>
            ))}
        </div>
      )}
      <details className="frontier-native-orders" open>
        <summary>
          {label('exchangeOrders', { symbol })} ·{' '}
          {orders?.accountID === accountID && orders.symbol === symbol
            ? open.length
            : label('notVerified')}
        </summary>
        {orderError ? (
          <p>{orderError}</p>
        ) : orders?.accountID === accountID && orders.symbol === symbol ? (
          open.length ? (
            <div className="frontier-table-scroll">
              <table>
                <thead>
                  <tr>
                    <th>{label('type')}</th>
                    <th>{label('side')}</th>
                    <th>{label('priceTrigger')}</th>
                    <th>{label('sizeHead')}</th>
                    <th>{label('status')}</th>
                    <th>{label('orderId')}</th>
                  </tr>
                </thead>
                <tbody>
                  {open.map((o) => (
                    <tr key={o.order_id}>
                      <td>{o.type}</td>
                      <td>{o.side}</td>
                      <td>{format(o.stop_price || o.price)}</td>
                      <td>{format(o.quantity)}</td>
                      <td>{o.status}</td>
                      <td>{o.order_id}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          ) : (
            <p>{label('noOpenOrders', { symbol })}</p>
          )
        ) : (
          <p>{label('ordersNotVerified')}</p>
        )}
      </details>
      {importantRuns.map((run) => {
        const entries = run.orders.filter((o) => o.role === 'entry')
        const filled = entries.reduce((sum, o) => sum + Number(o.filled), 0)
        const accepted = entries.some(
          (o) => o.order_id && ['NEW', 'PARTIALLY_FILLED'].includes(o.status)
        )
        const unconfirmed = entries.some((o) =>
          ['UNKNOWN', 'submitting'].includes(o.status)
        )
        const name =
          accounts.find((a) => a.id === run.exchange_id)?.name ||
          label('accountNotLoaded')
        const accountVerified =
          snapshot?.accountID === run.exchange_id && !error
        const nativePosition = accountVerified
          ? positions.find(
              (p) =>
                nativeMarket(p.symbol, run.plan.snapshot.venue) ===
                  nativeMarket(
                    run.plan.snapshot.symbol,
                    run.plan.snapshot.venue
                  ) && p.side === run.plan.idea.side
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
        return (
          <details
            className={
              'frontier-trade-summary' +
              (run.state === 'needs_attention' || unconfirmed
                ? ' attention'
                : '')
            }
            key={run.id}
          >
            <summary className="frontier-record-toggle">
              <span>
                {run.plan.snapshot.symbol} ·{' '}
                {label(run.plan.idea.side === 'long' ? 'long' : 'short')}{' '}
                <span className="frontier-record-label">
                  {label('executionRecord', { name })}
                </span>
              </span>
              <span
                className={
                  run.state === 'needs_attention' || unconfirmed
                    ? 'frontier-review-badge'
                    : ''
                }
              >
                {unconfirmed
                  ? label('submissionUnconfirmed')
                  : run.state === 'needs_attention'
                    ? label('monitorReview')
                    : stateLabel}
              </span>
            </summary>
            <div>
              <strong>
                {run.plan.snapshot.symbol} ·{' '}
                {label(run.plan.idea.side === 'long' ? 'long' : 'short')}
              </strong>
              <p>
                {name} · {run.plan.snapshot.venue} · {stateLabel}
                {filled > 0
                  ? ` · ${format(filled)} ${run.plan.snapshot.base_asset}`
                  : ''}
              </p>
              {run.state === 'needs_attention' && (
                <p className="frontier-monitor-alert">
                  {run.error?.startsWith('Service restarted')
                    ? label('serverRestarted')
                    : run.error || label('monitorNeedsReview')}
                </p>
              )}
            </div>
            <div>
              {onCheck &&
                ['needs_attention', 'position_open', 'stopping'].includes(
                  run.state
                ) && (
                  <Button
                    size="sm"
                    className="h-8 text-xs"
                    disabled={busy}
                    onClick={() => void onCheck(run)}
                  >
                    {label('checkTrade')}
                  </Button>
                )}
              <Button
                size="sm"
                className="h-8 text-xs"
                disabled={busy}
                onClick={() => onView(run)}
              >
                {label('viewOnChart')}
              </Button>
              <a
                href={exchangeURL(run.plan.snapshot.venue)}
                target="_blank"
                rel="noreferrer"
              >
                {label('openExchange')}
              </a>
            </div>
            <details>
              <summary>
                {label('recordedOrders')}
                {run.updated_at
                  ? ' · ' + new Date(run.updated_at).toLocaleTimeString()
                  : ''}
              </summary>
              <table>
                <thead>
                  <tr>
                    <th>{label('order')}</th>
                    <th>{label('price')}</th>
                    <th>{label('filledSize')}</th>
                    <th>{label('lastState')}</th>
                    <th>{label('exchangeId')}</th>
                  </tr>
                </thead>
                <tbody>
                  {run.orders.map((o) => (
                    <tr key={o.client_id}>
                      <td>
                        {o.role === 'entry'
                          ? label('entry')
                          : o.role === 'sl'
                            ? label('stopLoss')
                            : label('takeProfit')}
                      </td>
                      <td>{format(o.average || o.price)}</td>
                      <td>
                        {format(o.filled)} / {format(o.quantity)}
                      </td>
                      <td>{o.status}</td>
                      <td>{o.order_id || label('notConfirmed')}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
              <small>{label('recordsNote')}</small>
            </details>
          </details>
        )
      })}
      {!importantRuns.length && <small>{label('noActive')}</small>}
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
          <p className="text-[13px] leading-relaxed text-vergex-text-muted">
            {label('closeBody')}
          </p>
        </Dialog>
      )}
    </section>
  )
}
