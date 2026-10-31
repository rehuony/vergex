import { useEffect, useMemo, useRef, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { Page } from '@/components/common/page'
import { SectionTabs } from '@/components/common/section-tabs'
import {
  type Plan,
  type LiveRun,
  type Constraints,
  defaultLimits,
  expired,
} from '@/api/planning'
import { FrontierOrderBlocker } from '@/components/planning/frontier-order-blocker'
import { FrontierAccount } from '@/components/planning/frontier-account'
import {
  defaultMarket,
  preferredAccount as preferredTradingAccount,
  rememberAccount,
} from '@/utils/planning/frontier-accounts'
import { nativeMarket } from '@/utils/planning/frontier-account'
import {
  batchPlans,
  comparablePlans,
  readBatch,
  saveBatch,
  workspaceKey,
  type PlanBatch,
} from '@/utils/planning/frontier-workspace'
import {
  findBlockingRun,
  marketRecords,
  runSettled,
} from '@/utils/planning/frontier-plans'
import { FrontierHistory } from '@/components/planning/frontier-history'
import { PlanReviewDialog } from '@/components/planning/plan-review-dialog'
import { ManualEntryDialog } from '@/components/planning/manual-entry-dialog'
import { FrontierToolbar } from '@/components/planning/frontier-toolbar'
import { FrontierNotices } from '@/components/planning/frontier-notices'
import { FrontierChartPanel } from '@/components/planning/frontier-chart-panel'
import { FrontierForecast } from '@/components/planning/frontier-forecast'
import { FrontierPlanDeck } from '@/components/planning/frontier-plan-deck'
import { SelectedPlanDetails } from '@/components/planning/selected-plan-details'
import { SelectedPlanActions } from '@/components/planning/selected-plan-actions'
import { FrontierAdvanced } from '@/components/planning/frontier-advanced'
import { FrontierAssistant } from '@/components/planning/frontier-assistant'
import { useOpenSettings } from '@/components/settings/use-open-settings'
import { useAccountCheck } from '@/hooks/planning/use-account-check'
import { useAccountWatch } from '@/hooks/planning/use-account-watch'
import { useFrontierMarket } from '@/hooks/planning/use-frontier-market'
import {
  usePlanningData,
  type PlanningConversation,
} from '@/hooks/planning/use-planning-data'
import { useFrontierChart } from '@/components/planning/planning-chart'
import { usePlanningAssistant } from '@/components/planning/use-planning-assistant'
import { useFrontierTrades } from '@/hooks/planning/use-frontier-trades'
import { useLanguage } from '@/contexts/language-context'
import { t } from '@/i18n/translations'
import './trading-planning-page.css'

type FrontierTab = 'plans' | 'positions' | 'history'

const readTab = (value: string | null): FrontierTab =>
  value === 'positions' || value === 'history' ? value : 'plans'

export function TradingPlanningPage() {
  const { language } = useLanguage()
  const label = (key: string) => t(`frontier.${key}`, language)
  const [params, setParams] = useSearchParams()
  const [tab, setTab] = useState(() => readTab(params.get('tab')))
  const [linkedPlan, setLinkedPlan] = useState(() => params.get('plan'))
  const focusLinkedPlan = useRef(false)
  const openSettings = useOpenSettings()
  const [limits, setLimits] = useState<Constraints>(defaultLimits)
  const [selected, setSelected] = useState(''),
    [compare, setCompare] = useState<string[]>([])
  const [busy, setBusy] = useState(false),
    [error, setError] = useState(''),
    [orderNotice, setOrderNotice] = useState('')
  const [leverage, setLeverage] = useState(5)
  const market = useFrontierMarket({
    setError,
    // Changed connections can move Frontier to another account: stop any
    // planning stream first, as changeAccount does, so it cannot write into it.
    onAccountSwitch: () => {
      abort.current?.abort()
      resetAccountWorkspace()
    },
  })
  const {
    accounts,
    models,
    exchangeID,
    setExchangeID,
    modelID,
    setModelID,
    venue,
    setVenue,
    requestedVenue,
    marketChosen,
    symbol,
    setSymbol,
  } = market
  const accountCheck = useAccountCheck(exchangeID, venue)
  const accountWatch = useAccountWatch(exchangeID, symbol)
  const observation = accountWatch.observation
  const { data, reload, history, refreshHistory, preferences, setPreferences } =
    usePlanningData(setError)
  const account = accounts.find((a) => a.id === exchangeID)
  const records = useMemo(
    () => marketRecords(data, exchangeID, symbol, venue),
    [data, exchangeID, symbol, venue]
  )
  const batchKey = workspaceKey(exchangeID, venue, symbol)
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
  function showTab(next: FrontierTab) {
    setTab(next)
    scrollTo('frontier-sections')
  }
  const selectedRun = data.runs.find(
    (r) => r.plan_id === selected && r.exchange_id === exchangeID
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
  const blockingRun = findBlockingRun(data.runs, exchangeID, clock)
  const { chart, price, setPrice, interval, scene, onForecast } =
    useFrontierChart(
      venue,
      symbol,
      current,
      compare,
      data.records,
      run?.state,
      setError
    )
  const assistant = usePlanningAssistant({
    busy,
    setBusy,
    modelID,
    exchangeID,
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
  const trades = useFrontierTrades({
    busy,
    setBusy,
    setError,
    setOrderNotice,
    exchangeID,
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
    onShowAccount: () => showTab('positions'),
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
  }, [exchangeID])
  // The address says what is open, so it can be linked to and survives a reload.
  useEffect(() => {
    if (!market.configsLoaded) return
    const link = {
      venue,
      symbol,
      account: exchangeID,
      plan: selected || linkedPlan || '',
      tab: tab === 'plans' ? '' : tab,
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
    exchangeID,
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
    const owner = linkedRun?.exchange_id ?? data.accounts[linkedPlan]
    marketChosen.current = true
    requestedVenue.current = plan.snapshot.venue
    setVenue(plan.snapshot.venue)
    if (accounts.some((a) => a.id === owner)) setExchangeID(owner)
    setSymbol(nativeMarket(plan.snapshot.symbol, plan.snapshot.venue))
    setSelected(linkedPlan)
    setCompare([])
    focusLinkedPlan.current = true
  }, [linkedPlan, market.configsLoaded, data.now])
  // It fits on the chart showing its market, which may still be coming up.
  useEffect(() => {
    if (!focusLinkedPlan.current || !current || !chart.current) return
    const shown = chart.current.context()
    if (shown.venue !== current.plan.snapshot.venue) return
    if (
      shown.symbol !==
      nativeMarket(current.plan.snapshot.symbol, current.plan.snapshot.venue)
    )
      return
    focusLinkedPlan.current = false
    void chart.current
      .plot(current.plan, current.state, [])
      .then(() => chart.current?.focusPlan())
  }, [current, scene])
  function requestReview(plan: Plan) {
    if (exchangeID) void reviewWithAccount(plan)
    else openSettings('exchanges')
  }
  // Plans, checks and dialogs belong to one account; switching starts over.
  function resetAccountWorkspace() {
    setReview(null)
    setBuilder(false)
    setLimits(defaultLimits)
    setError('')
    setOrderNotice('')
    setSelected('')
    setCompare([])
    resetTurn()
  }
  function openMarket(next: string) {
    marketChosen.current = true
    requestedVenue.current = venue
    setPrice('—')
    setSymbol(next)
    setSelected('')
    resetTurn()
    setCompare([])
    setMessages([])
    setTrace([])
    setError('')
    setOrderNotice('')
    session.current = crypto.randomUUID()
  }
  async function choose(p: Plan) {
    setSelected(p.id)
    setCompare([])
    setError('')
    scrollTo('frontier-chart')
    await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()))
    await chart.current?.plot(p, data.records[p.id]?.state || 'draft', [])
    await chart.current?.focusPlan()
  }
  function changeAccount(
    id: string,
    nextVenue = accounts.find((a) => a.id === id)?.exchange_type || venue
  ) {
    marketChosen.current = true
    abort.current?.abort()
    setExchangeID(id)
    if (market.configsLoaded) rememberAccount(nextVenue, id)
    setLimits(defaultLimits)
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
    setExchangeID(trade.exchange_id)
    setSymbol(
      nativeMarket(trade.plan.snapshot.symbol, trade.plan.snapshot.venue)
    )
    setSelected(trade.plan_id)
    setCompare([])
    resetTurn()
    setMessages([])
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
    resetTurn()
    setCompare([])
    setTrace([])
    setError('')
    setOrderNotice('')
  }
  function openConversation(h: PlanningConversation) {
    session.current = h.id
    changeAccount(h.exchange_id)
    session.current = h.id
    setSymbol(h.symbol)
    setModelID(h.model_id)
    setMessages(JSON.parse(h.messages))
    setSelected('')
    resetTurn()
  }
  // New plans are what the user asked for, so bring them into view.
  useEffect(() => {
    if (assistant.turnPlanIDs.length > 0) setTab('plans')
  }, [assistant.turnPlanIDs.length])
  const deckPlans = batchPlans(records, currentBatch)
  const waitingForEntry = Boolean(
    currentBatch?.waiting && !assistant.planningBusy
  )
  const comparisonPlans = comparablePlans(deckPlans, clock)
  const archivedRecords = records.filter(
    (r) => !deckPlans.some((p) => p.id === r.plan.id)
  )
  return (
    <Page className="planning-page" title="Frontier">
      <FrontierToolbar
        venue={venue}
        accounts={accounts}
        accountID={exchangeID}
        accountCheck={accountCheck}
        symbol={symbol}
        price={price}
        leverage={leverage}
        busy={busy}
        onVenueChange={(next) =>
          changeAccount(preferredTradingAccount(accounts, next), next)
        }
        onAccountChange={(id) => changeAccount(id)}
        onOpenMarket={openMarket}
        onLeverageChange={setLeverage}
      />
      {blockingRun && (
        <FrontierOrderBlocker
          run={blockingRun}
          now={clock}
          observation={observation}
          checkError={
            trades.tradeCheckError?.runID === blockingRun.id
              ? trades.tradeCheckError.message
              : undefined
          }
          onAccount={() => showTab('positions')}
          busy={busy}
          onCheck={() => void checkPreviousTrade(blockingRun)}
          onView={() => {
            viewTrade(blockingRun)
            showTab('positions')
          }}
          onRevoke={() => void runAction('stop', blockingRun)}
        />
      )}
      <FrontierNotices
        notice={orderNotice}
        error={error}
        blocked={Boolean(blockingRun)}
        trace={assistant.trace}
        onDismissError={() => setError('')}
      />
      <div className="grid gap-3 lg:h-[max(520px,calc(100dvh-160px))] lg:grid-cols-[minmax(0,1fr)_380px]">
        <FrontierChartPanel
          plan={current?.plan}
          comparing={compare.length}
          compareDisabled={busy || comparisonPlans.length < 2}
          onFit={() => chart.current?.focusPlan()}
          onCompare={() => {
            setCompare(comparisonPlans.slice(0, 4).map((p) => p.id))
          }}
          onExitCompare={() => setCompare([])}
          overlay={
            <FrontierForecast
              chart={chart}
              venue={venue}
              symbol={symbol}
              interval={interval}
              scene={scene}
              comparing={compare.length >= 2}
              hovering={onForecast}
            />
          }
        />
        <FrontierAssistant
          busy={busy}
          planningBusy={assistant.planningBusy}
          status={assistant.status}
          models={models}
          modelID={modelID}
          onModelChange={setModelID}
          instruction={assistant.instruction}
          onInstructionChange={assistant.setInstruction}
          onSend={send}
          onStop={() => abort.current?.abort()}
          onNewIdea={newSession}
          messages={assistant.messages}
          trace={assistant.trace}
          hasPlan={Boolean(current)}
          followRef={assistant.chatFollow}
          review={
            <SelectedPlanActions
              record={current}
              run={run}
              accountName={
                accounts.find((a) => a.id === data.accounts[selected])?.name
              }
              connected={Boolean(exchangeID)}
              reviewDisabled={
                busy || (!!exchangeID && data.accounts[selected] !== exchangeID)
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
      <div
        id="frontier-sections"
        className="frontier-sections mt-4 scroll-mt-20"
      >
        <SectionTabs
          label={label('sections')}
          value={tab}
          onChange={setTab}
          tabs={[
            { id: 'plans', label: label('plansTab') },
            { id: 'positions', label: label('positionsTab') },
            { id: 'history', label: label('historyTab') },
          ]}
          className="!mt-0 !mb-4"
        />
        {/* Hidden, not unmounted: the account section remembers its close
            attempts, which must survive a look at another section. */}
        <div role="tabpanel" hidden={tab !== 'plans'}>
          <FrontierPlanDeck
            batch={currentBatch}
            plans={deckPlans}
            checks={assistant.checks}
            planningBusy={assistant.planningBusy}
            waitingForEntry={waitingForEntry}
            selected={selected}
            clock={clock}
            connected={Boolean(exchangeID)}
            reviewDisabled={busy}
            onChoose={choose}
            onReview={requestReview}
          />
          {current && <SelectedPlanDetails record={current} />}
          <FrontierAdvanced
            limits={limits}
            onLimitsChange={setLimits}
            accountSized={Boolean(exchangeID)}
            busy={busy}
            onCustomEntry={() => setBuilder(true)}
            preferences={preferences}
            onPreferencesChange={setPreferences}
            sessionRef={session}
            onStatus={assistant.setStatus}
            onError={setError}
          />
        </div>
        <div role="tabpanel" hidden={tab !== 'positions'}>
          <div className="frontier-portfolio" id="frontier-account">
            <FrontierAccount
              account={account}
              accounts={accounts}
              venue={venue}
              symbol={symbol}
              runs={data.runs}
              busy={busy}
              onView={viewTrade}
              onManual={manualEntry}
              onBusy={setBusy}
              onRefresh={reload}
              watch={accountWatch}
              onCheck={checkPreviousTrade}
            />
          </div>
        </div>
        <div role="tabpanel" hidden={tab !== 'history'}>
          <FrontierHistory
            active={tab === 'history'}
            accountID={exchangeID}
            runs={data.runs.filter(
              (r) => r.exchange_id === exchangeID && runSettled(r.state)
            )}
            archived={archivedRecords}
            selected={selected}
            compare={compare}
            clock={clock}
            conversations={history}
            busy={busy}
            onChoose={choose}
            onOpenConversation={openConversation}
          />
        </div>
      </div>
      {review && (
        <PlanReviewDialog
          plan={review}
          accountName={account?.name}
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
          accountName={account?.name}
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
