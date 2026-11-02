import { useEffect, useMemo, useRef, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { Page } from '@/components/common/page'
import { SectionTabs } from '@/components/common/section-tabs'
import {
  type Plan,
  type LiveRun,
  type Constraints,
  expired,
  futuresAPI,
} from '@/api/futures'
import { FuturesOrderBlocker } from '@/components/futures/futures-order-blocker'
import { FuturesAccount } from '@/components/futures/futures-account'
import {
  accountName as nameOfAccount,
  defaultMarket,
  preferredAccount as preferredTradingAccount,
  rememberAccount,
} from '@/utils/futures/futures-accounts'
import { nativeMarket } from '@/utils/futures/futures-account'
import {
  batchPlans,
  readBatch,
  saveBatch,
  saveLimits,
  savedLimits,
  workspaceKey,
  type PlanBatch,
} from '@/utils/futures/futures-workspace'
import {
  findBlockingRun,
  marketRecords,
  runSettled,
} from '@/utils/futures/futures-plans'
import { FuturesHistory } from '@/components/futures/futures-history'
import { PlanReviewDialog } from '@/components/futures/plan-review-dialog'
import { ManualEntryDialog } from '@/components/futures/manual-entry-dialog'
import { FuturesToolbar } from '@/components/futures/futures-toolbar'
import { FuturesNotices } from '@/components/futures/futures-notices'
import { FuturesChartPanel } from '@/components/futures/futures-chart-panel'
import {
  FuturesForecast,
  useForecastSwitch,
} from '@/components/futures/futures-forecast'
import { MarketPicker } from '@/components/futures/market-picker'
import { FuturesPlanDeck } from '@/components/futures/futures-plan-deck'
import { SelectedPlanDetails } from '@/components/futures/selected-plan-details'
import { SelectedPlanActions } from '@/components/futures/selected-plan-actions'
import { FuturesSettings } from '@/components/futures/futures-settings'
import {
  FuturesAssistant,
  type AssistantPane,
} from '@/components/futures/futures-assistant'
import { useOpenSettings } from '@/components/settings/use-open-settings'
import { useAccountCheck } from '@/hooks/futures/use-account-check'
import { useAccountWatch } from '@/hooks/futures/use-account-watch'
import { useFuturesMarket } from '@/hooks/futures/use-futures-market'
import {
  useFuturesData,
  type FuturesConversation,
} from '@/hooks/futures/use-futures-data'
import { useFuturesChart } from '@/components/futures/futures-chart'
import { holdingLines } from '@/components/futures/futures-chart/futures-chart-shapes'
import {
  type ChatMessage,
  useFuturesAssistant,
} from '@/components/futures/use-futures-assistant'
import { useFuturesTrades } from '@/hooks/futures/use-futures-trades'
import { useLanguage } from '@/contexts/language-context'
import { notify } from '@/components/common/notify'
import { t } from '@/i18n'
import type { Account } from '@/types'

type FuturesTab = 'trade' | 'positions' | 'history' | 'settings'

const readTab = (value: string | null): FuturesTab =>
  value === 'positions' || value === 'history' || value === 'settings'
    ? value
    : 'trade'

export function FuturesPage() {
  const { language } = useLanguage()
  const label = (key: string) => t(`futures.${key}`, language)
  const [params, setParams] = useSearchParams()
  const [tab, setTab] = useState(() => readTab(params.get('tab')))
  // Which side of the assistant is open. It opens on the conversation, whatever plans were
  // left from before, and goes to the plans when a turn leaves some or one is chosen.
  const [pane, setPane] = useState<AssistantPane>('chat')
  const [forecastOn, setForecastOn] = useForecastSwitch()
  const [linkedPlan, setLinkedPlan] = useState(() => params.get('plan'))
  const fitPlan = useRef(false)
  const openSettings = useOpenSettings()
  const [limits, setLimits] = useState<Constraints>(savedLimits)
  const [selected, setSelected] = useState(''),
    [compare, setCompare] = useState<string[]>([])
  const [busy, setBusy] = useState(false),
    [error, setError] = useState(''),
    [orderNotice, setOrderNotice] = useState('')
  const [leverage, setLeverage] = useState(5)
  const market = useFuturesMarket({
    setError,
    // Changed connections can move Futures to another account: stop any
    // planning stream first, as changeAccount does, so it cannot write into it.
    onAccountSwitch: () => {
      abort.current?.abort()
      resetAccountWorkspace()
    },
  })
  const {
    accounts,
    models,
    accountId,
    setAccountId,
    modelID,
    modelName,
    chooseModel,
    venue,
    setVenue,
    requestedVenue,
    marketChosen,
    symbol,
    setSymbol,
  } = market
  const accountCheck = useAccountCheck(accountId, venue, tab === 'positions')
  const accountWatch = useAccountWatch(accountId, symbol, tab === 'positions')
  const observation = accountWatch.observation
  const {
    data,
    reload,
    history,
    refreshHistory,
    renameConversation,
    deleteConversation,
    deletePlan,
    preferences,
    setPreferences,
  } = useFuturesData(setError, accountId)
  const account = accounts.find((a) => a.id === accountId)
  const accountName = (row?: Account) => row && nameOfAccount(row, accounts)
  const records = useMemo(
    () => marketRecords(data, accountId, symbol, venue),
    [data, accountId, symbol, venue]
  )
  const batchKey = workspaceKey(accountId, venue, symbol)
  const [batch, setBatch] = useState<{ key: string; value: PlanBatch | null }>(
    () => ({ key: batchKey, value: readBatch(batchKey) })
  )
  const currentBatch =
    batch.key === batchKey ? batch.value : readBatch(batchKey)
  function rememberBatch(value: PlanBatch, key = batchKey) {
    saveBatch(key, value)
    setBatch({ key, value })
  }
  function scrollTo(id: string) {
    requestAnimationFrame(() =>
      document
        .getElementById(id)
        ?.scrollIntoView({ behavior: 'smooth', block: 'start' })
    )
  }
  const selectedRun = data.runs.find(
    (r) => r.plan_id === selected && r.account_id === accountId
  )
  const current = useMemo(
    () =>
      data.records[selected] ||
      (selectedRun
        ? { plan: selectedRun.plan, state: selectedRun.state }
        : undefined),
    [data.records, selected, selectedRun?.plan, selectedRun?.state]
  )
  const run = selectedRun
  const clock = Date.parse(data.now) || Date.now()
  const blockingRun = findBlockingRun(data.runs, accountId, clock)
  const {
    chart,
    price,
    setPrice,
    interval,
    resolution,
    changeResolution,
    scene,
    candlesReady,
  } = useFuturesChart(
    venue,
    symbol,
    current,
    compare,
    data.records,
    run?.state,
    setError
  )
  const assistant = useFuturesAssistant({
    busy,
    setBusy,
    modelID,
    modelName,
    accountId,
    venue,
    symbol,
    chart,
    current,
    selected,
    setSelected,
    setCompare,
    limits,
    setLimits,
    leverage,
    batchKey,
    rememberBatch,
    reload,
    refreshHistory,
    setError,
    setOrderNotice,
    // The idea stays in the composer, so nothing is lost while adding a model.
    onNeedModel: () => openSettings('models'),
  })
  const { setMessages, setTrace, session, abort, resetTurn, send } = assistant
  const trades = useFuturesTrades({
    busy,
    setBusy,
    setError,
    setOrderNotice,
    accountId,
    venue,
    symbol,
    leverage,
    limits,
    setLimits,
    observation,
    clock,
    blockingRun,
    reload,
    setSelected,
    setCompare,
    onShowAccount: () => setTab('positions'),
    // The refreshed plan takes its row, so what is on the chart is what is listed.
    onPlanSized: (old, sized) => {
      fitPlan.current = true
      if (currentBatch?.ids.includes(old))
        rememberBatch({
          ...currentBatch,
          ids: currentBatch.ids.map((id) => (id === old ? sized : id)),
        })
    },
  })
  const {
    review,
    setReview,
    setBuilder,
    reviewWithAccount,
    checkPreviousTrade,
    runAction,
  } = trades
  useEffect(() => {
    setOrderNotice('')
    trades.setTradeCheckError(null)
  }, [accountId])
  // The address says what is open, so it can be linked to and survives a reload.
  useEffect(() => {
    if (!market.configsLoaded) return
    const link = {
      venue,
      symbol,
      account: accountId,
      plan: selected || linkedPlan || '',
      tab: tab === 'trade' ? '' : tab,
    }
    setParams(
      (old) => {
        const next = new URLSearchParams(old)
        for (const [name, value] of Object.entries(link)) {
          if (value) next.set(name, value)
          else next.delete(name)
        }
        return next
      },
      { replace: true }
    )
  }, [
    market.configsLoaded,
    venue,
    symbol,
    accountId,
    selected,
    linkedPlan,
    tab,
  ])
  // A plan named by a link opens its own market and account once they are known.
  useEffect(() => {
    if (!linkedPlan || !market.configsLoaded || !data.now) return
    setLinkedPlan(null)
    const linkedRun = data.runs.find((r) => r.plan_id === linkedPlan)
    const plan = data.records[linkedPlan]?.plan ?? linkedRun?.plan
    if (!plan) return
    const owner = linkedRun?.account_id ?? data.accounts[linkedPlan]
    marketChosen.current = true
    requestedVenue.current = plan.snapshot.venue
    setVenue(plan.snapshot.venue)
    if (accounts.some((a) => a.id === owner)) setAccountId(owner)
    setSymbol(nativeMarket(plan.snapshot.symbol, plan.snapshot.venue))
    setSelected(linkedPlan)
    setCompare([])
    fitPlan.current = true
  }, [linkedPlan, market.configsLoaded, data.now])
  // New plans are what the user asked for, so a turn that leaves some shows them.
  useEffect(() => {
    if (assistant.futuresBusy || assistant.turnPlanIDs.length === 0) return
    setTab('trade')
    setPane('plans')
    fitPlan.current = true
  }, [assistant.futuresBusy, assistant.turnPlanIDs.length])
  // It fits on the chart showing its market, which may still be coming up.
  useEffect(() => {
    if (!fitPlan.current || !current || !chart.current) return
    const shown = chart.current.context()
    if (shown.venue !== current.plan.snapshot.venue) return
    if (
      shown.symbol !==
      nativeMarket(current.plan.snapshot.symbol, current.plan.snapshot.venue)
    )
      return
    fitPlan.current = false
    void chart.current
      .plot(current.plan, current.state, [])
      .then(() => chart.current?.focusPlan())
  }, [current, scene, assistant.futuresBusy, compare.length])
  // What the account holds on this market stays on the chart, whatever else is drawn.
  useEffect(() => {
    const orders = accountWatch.orders
    chart.current?.setHoldings(
      venue,
      symbol,
      holdingLines(
        venue,
        symbol,
        observation?.positions ?? [],
        orders?.accountID === accountId && orders.symbol === symbol
          ? orders.rows
          : []
      )
    )
  }, [
    observation,
    accountWatch.orders,
    accountId,
    venue,
    symbol,
    scene,
    language,
  ])
  function requestReview(plan: Plan) {
    if (accountId) void reviewWithAccount(plan)
    else openSettings('accounts')
  }
  // Plans, checks and dialogs belong to one account; switching starts over.
  function resetAccountWorkspace() {
    setReview(null)
    setBuilder(false)
    setLimits(savedLimits())
    setError('')
    setOrderNotice('')
    setSelected('')
    setCompare([])
    resetTurn()
    // A conversation stays with the account it started on.
    setMessages([])
    session.current = crypto.randomUUID()
  }
  function openMarket(next: string) {
    marketChosen.current = true
    requestedVenue.current = venue
    setPrice('—')
    setSymbol(next)
    setSelected('')
    setPane('chat')
    resetTurn()
    setCompare([])
    setMessages([])
    setTrace([])
    setError('')
    setOrderNotice('')
    session.current = crypto.randomUUID()
  }
  function choose(p: Plan) {
    setSelected(p.id)
    setCompare([])
    setError('')
    setTab('trade')
    setPane('plans')
    fitPlan.current = true
    scrollTo('futures-chart')
  }
  function changeAccount(
    id: string,
    nextVenue = accounts.find((a) => a.id === id)?.venue || venue
  ) {
    marketChosen.current = true
    abort.current?.abort()
    setAccountId(id)
    if (market.configsLoaded) rememberAccount(nextVenue, id)
    setLimits(savedLimits())
    setReview(null)
    setBuilder(false)
    setError('')
    setOrderNotice('')
    requestedVenue.current = nextVenue
    setVenue(nextVenue)
    if (nextVenue !== venue) {
      setSymbol(defaultMarket(nextVenue))
      setPrice('—')
    }
    setSelected('')
    resetTurn()
    setCompare([])
    setMessages([])
    session.current = crypto.randomUUID()
  }
  function viewTrade(trade: LiveRun) {
    marketChosen.current = true
    requestedVenue.current = trade.plan.snapshot.venue
    setVenue(trade.plan.snapshot.venue)
    setAccountId(trade.account_id)
    setSymbol(
      nativeMarket(trade.plan.snapshot.symbol, trade.plan.snapshot.venue)
    )
    setSelected(trade.plan_id)
    setCompare([])
    setPane('plans')
    resetTurn()
    setMessages([])
    // What comes next is another conversation, maybe on another account.
    session.current = crypto.randomUUID()
  }
  function manualEntry(side: 'long' | 'short') {
    trades.setBuilderIdea({
      name: `Manual ${symbol} ${side}`,
      rationale: 'User-created manual limit entry',
      side,
      entry: price === '—' ? '' : price,
      stop: '',
      target: '',
      execution: { kind: 'limit' },
    })
    setBuilder(true)
  }
  function newSession() {
    abort.current?.abort()
    session.current = crypto.randomUUID()
    setMessages([])
    setSelected('')
    setPane('chat')
    resetTurn()
    setCompare([])
    setTrace([])
    setError('')
    setOrderNotice('')
  }
  async function openConversation(h: FuturesConversation) {
    try {
      const { messages } = await futuresAPI<{ messages: ChatMessage[] }>(
        `conversations/${h.id}`
      )
      session.current = h.id
      setSymbol(h.symbol)
      chooseModel(h.agent_model_id)
      setMessages(messages)
      setSelected('')
      setTab('trade')
      setPane('chat')
      resetTurn()
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    }
  }
  async function removeConversation(h: FuturesConversation) {
    await deleteConversation(h.id)
    // What is on the trade tab went with it, and must not be saved under its ID again.
    if (session.current === h.id) newSession()
  }
  // The line that says what went wrong is on the trade tab.
  useEffect(() => {
    if (error && tab !== 'trade') notify.error(error)
  }, [error])
  const deckPlans = batchPlans(records, currentBatch)
  const waitingForEntry = Boolean(
    currentBatch?.waiting && !assistant.futuresBusy
  )
  // A plan that ran is listed with its run.
  const archivedRecords = records.filter(
    (r) =>
      !deckPlans.some((p) => p.id === r.plan.id) &&
      !data.runs.some((run) => run.plan_id === r.plan.id)
  )
  return (
    <Page
      className="vergex-screen animate-page-in"
      title={t('futuresNav', language)}
      leading={
        <SectionTabs
          label={label('sections')}
          value={tab}
          onChange={setTab}
          tabs={[
            { id: 'trade', label: label('tradeTab') },
            { id: 'positions', label: label('positionsTab') },
            { id: 'history', label: label('historyTab') },
            { id: 'settings', label: label('settingsTab') },
          ]}
        />
      }
      actions={
        <FuturesToolbar
          venue={venue}
          loaded={market.configsLoaded}
          accounts={accounts}
          accountID={accountId}
          accountCheck={accountCheck}
          leverage={leverage}
          busy={busy}
          onVenueChange={(next) =>
            changeAccount(preferredTradingAccount(accounts, next), next)
          }
          onAccountChange={(id) => changeAccount(id)}
          onLeverageChange={setLeverage}
        />
      }
    >
      {/* Hidden, not unmounted: the chart keeps its market, a planning turn keeps running
          and the account section remembers its close attempts, all of which must survive
          a look at another tab. */}
      <div
        role="tabpanel"
        hidden={tab !== 'trade'}
        className="grid min-w-0 gap-4 lg:h-[max(520px,calc(100dvh-181px))] lg:grid-cols-[minmax(0,1fr)_380px]"
      >
        <FuturesChartPanel
          market={
            <MarketPicker
              venue={venue}
              selected={symbol}
              disabled={busy}
              onSelect={openMarket}
            />
          }
          price={price}
          resolution={resolution}
          onResolution={changeResolution}
          forecastOn={forecastOn}
          onForecastChange={setForecastOn}
          overlay={
            <FuturesForecast
              on={forecastOn}
              chart={chart}
              venue={venue}
              symbol={symbol}
              interval={interval}
              scene={scene}
              candlesReady={candlesReady}
              comparing={compare.length >= 2}
            />
          }
        />
        <FuturesAssistant
          pane={pane}
          onPaneChange={setPane}
          planCount={deckPlans.length}
          busy={busy}
          futuresBusy={assistant.futuresBusy}
          models={models}
          modelID={modelID}
          modelName={modelName}
          onModelChange={chooseModel}
          instruction={assistant.instruction}
          onInstructionChange={assistant.setInstruction}
          onSend={send}
          onStop={() => abort.current?.abort()}
          onNewIdea={newSession}
          messages={assistant.messages}
          trace={assistant.trace}
          streaming={assistant.streaming}
          hasPlan={Boolean(current)}
          followRef={assistant.chatFollow}
          notices={
            <FuturesNotices
              busy={busy && !assistant.futuresBusy}
              progress={label('validatingOrder')}
              notice={orderNotice}
              error={error}
              blocker={
                blockingRun && (
                  <FuturesOrderBlocker
                    run={blockingRun}
                    now={clock}
                    observation={observation}
                    checkError={
                      trades.tradeCheckError?.runID === blockingRun.id
                        ? trades.tradeCheckError.message
                        : undefined
                    }
                    onAccount={() => setTab('positions')}
                    busy={busy}
                    onCheck={() => void checkPreviousTrade(blockingRun)}
                    onView={() => {
                      viewTrade(blockingRun)
                      setTab('positions')
                    }}
                    onRevoke={() => void runAction('stop', blockingRun)}
                  />
                )
              }
              trace={assistant.trace}
              onDismissError={() => setError('')}
            />
          }
          plans={
            <>
              <FuturesPlanDeck
                batch={currentBatch}
                plans={deckPlans}
                checks={assistant.checks}
                futuresBusy={assistant.futuresBusy}
                waitingForEntry={waitingForEntry}
                selected={selected}
                clock={clock}
                connected={Boolean(accountId)}
                comparing={compare.length >= 2}
                onChoose={(p) => {
                  // Pressing the plan that is open closes it, unless it is being compared.
                  if (p.id === selected && compare.length < 2) setSelected('')
                  else choose(p)
                }}
                onCompare={() => setCompare(deckPlans.map((p) => p.id))}
                onExitCompare={() => setCompare([])}
              />
              {current && (
                <SelectedPlanDetails
                  record={current}
                  onClose={() => setSelected('')}
                />
              )}
            </>
          }
          actions={
            <SelectedPlanActions
              record={current}
              run={run}
              accountName={accountName(
                accounts.find((a) => a.id === data.accounts[selected])
              )}
              connected={Boolean(accountId)}
              reviewDisabled={
                busy || (!!accountId && data.accounts[selected] !== accountId)
              }
              busy={busy}
              clock={clock}
              onReview={() => {
                if (current) requestReview(current.plan)
              }}
              onRunAction={(action, r) => void runAction(action, r)}
            />
          }
        />
      </div>
      <div role="tabpanel" hidden={tab !== 'positions'} id="futures-account">
        <FuturesAccount
          account={account}
          accounts={accounts}
          accountCheck={accountCheck}
          venue={venue}
          symbol={symbol}
          runs={data.runs}
          busy={busy}
          active={tab === 'positions'}
          onView={(trade) => {
            viewTrade(trade)
            setTab('trade')
          }}
          onManual={manualEntry}
          onBusy={setBusy}
          onRefresh={reload}
          watch={accountWatch}
          onCheck={checkPreviousTrade}
        />
      </div>
      <div role="tabpanel" hidden={tab !== 'history'}>
        <FuturesHistory
          active={tab === 'history'}
          accountID={accountId}
          runs={data.runs.filter(
            (r) => r.account_id === accountId && runSettled(r.state)
          )}
          archived={archivedRecords}
          clock={clock}
          conversations={history}
          busy={busy}
          onChoose={choose}
          onOpenConversation={openConversation}
          onRenameConversation={renameConversation}
          onDeleteConversation={removeConversation}
          onDeleteDraft={(plan) => deletePlan(plan.id)}
        />
      </div>
      <div role="tabpanel" hidden={tab !== 'settings'}>
        <FuturesSettings
          active={tab === 'settings'}
          limits={limits}
          onLimitsChange={(next) => {
            setLimits(next)
            saveLimits(next)
          }}
          accountSized={Boolean(accountId)}
          preferences={preferences}
          onPreferencesChange={setPreferences}
          sessionID={session.current}
        />
      </div>
      {review && (
        <PlanReviewDialog
          plan={review}
          accountName={accountName(account)}
          replacement={trades.reviewReplacement}
          busy={busy}
          expired={expired(review, clock)}
          onBack={() => setReview(null)}
          onConfirm={trades.approveAndExecute}
        />
      )}
      {trades.builder && (
        <ManualEntryDialog
          symbol={symbol}
          venue={venue}
          accountName={accountName(account)}
          leverage={leverage}
          onLeverageChange={setLeverage}
          idea={trades.builderIdea}
          onIdeaChange={trades.setBuilderIdea}
          levels={trades.levels}
          onLevelsChange={trades.setLevels}
          busy={busy}
          onCancel={() => setBuilder(false)}
          onSave={trades.saveBuilder}
        />
      )}
    </Page>
  )
}
