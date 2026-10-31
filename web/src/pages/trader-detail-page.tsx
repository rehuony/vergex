import { useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import useSWR from 'swr'
import { ChevronLeft, Pencil, Trash2 } from 'lucide-react'
import { toast } from 'sonner'
import { api } from '@/api'
import { confirmToast } from '@/components/common/notify'
import { Page, type PageTab } from '@/components/common/page'
import { Panel, PanelState } from '@/components/common/panel'
import { DecisionFunnel } from '@/components/trader/decision-funnel'
import { DecisionLog } from '@/components/trader/decision-log'
import { EdgeProfile } from '@/components/trader/edge-profile'
import { EquityCurve } from '@/components/trader/equity-curve'
import { GridRiskPanel } from '@/components/trader/grid-risk-panel'
import { OpenPositions } from '@/components/trader/open-positions'
import { RecentCloses } from '@/components/trader/recent-closes'
import { StartTraderDialog } from '@/components/trader/start-trader-dialog'
import { SymbolBreakdown } from '@/components/trader/symbol-breakdown'
import { TraderBanners } from '@/components/trader/trader-banners'
import { TraderConfigModal } from '@/components/trader/trader-config-modal'
import { parseScanMinutes } from '@/components/trader/trader-format'
import { TraderMetrics } from '@/components/trader/trader-metrics'
import { TraderOrders } from '@/components/trader/trader-orders'
import { useTraderData } from '@/components/trader/use-trader-data'
import { Button } from '@/components/ui/button'
import {
  getExchangeDisplayName,
  getTraderModelLabel,
} from '@/constants/model-providers'
import { buildTraderPath, ROUTES } from '@/constants/route-paths'
import { useLanguage } from '@/contexts/language-context'
import { useExchangeConfigs, useModelConfigs } from '@/hooks/use-connections'
import { t } from '@/i18n/translations'
import type { CreateTraderRequest, TraderConfigData, TraderInfo } from '@/types'
import { cn } from '@/utils/cn'
import { findTraderBySlug, getTraderSlug } from '@/utils/trader-slug'

function TraderSections({
  trader,
  section,
}: {
  trader: TraderInfo
  section: string
}) {
  const { language } = useLanguage()
  const id = trader.trader_id
  const { status, account, positions } = useTraderData(id)
  const { data: stats } = useSWR(
    ['full-stats', id],
    () => api.getFullStats(id, true),
    { refreshInterval: 30000, shouldRetryOnError: false }
  )
  const { data: history } = useSWR(
    ['pos-history', id],
    () => api.getPositionHistory(id, 50, true),
    { refreshInterval: 60000, shouldRetryOnError: false }
  )
  const { data: config } = useSWR(
    ['trader-config', id],
    () => api.getTraderConfig(id, true),
    { refreshInterval: 120000, shouldRetryOnError: false }
  )
  const { data: latest } = useSWR(
    ['decisions-latest', id],
    () => api.getLatestDecisions(id, 1, true),
    { refreshInterval: 30000, shouldRetryOnError: false }
  )
  const latestDecision = latest?.[0]
  const scanMinutes =
    config?.scan_interval_minutes ||
    parseScanMinutes(status.data?.scan_interval)
  const nextCycleMs = latestDecision?.timestamp
    ? new Date(latestDecision.timestamp).getTime() + scanMinutes * 60_000
    : null

  return (
    <div className="flex flex-col gap-4">
      <TraderBanners status={status.data} language={language} />
      {section === '' && (
        <>
          <TraderMetrics
            account={account.data}
            stats={stats}
            positions={positions.data}
            running={status.data?.is_running}
            nextCycleMs={nextCycleMs}
            language={language}
          />
          <EquityCurve traderId={id} language={language} />
          {status.data?.strategy_type === 'grid_trading' && (
            <GridRiskPanel traderId={id} language={language} />
          )}
          <div className="grid gap-4 md:grid-cols-2">
            <SymbolBreakdown
              stats={history?.symbol_stats}
              language={language}
            />
            <EdgeProfile positions={history?.positions} language={language} />
          </div>
        </>
      )}
      {section === 'positions' && (
        <>
          <OpenPositions
            traderId={id}
            positions={positions.data}
            failed={Boolean(positions.error)}
            language={language}
          />
          <RecentCloses trades={history?.positions} language={language} />
        </>
      )}
      {section === 'decisions' && (
        <>
          <DecisionFunnel
            latest={latestDecision}
            positions={positions.data}
            language={language}
          />
          <DecisionLog traderId={id} language={language} />
        </>
      )}
      {section === 'orders' && (
        <TraderOrders
          traderId={id}
          traderName={trader.trader_name}
          language={language}
        />
      )}
    </div>
  )
}

// One trader: its results and history, and the only place to start, stop, edit
// or delete it. Sections switch without remounting the page.
export function TraderDetailPage() {
  const { slug = '', '*': section = '' } = useParams()
  const navigate = useNavigate()
  const { language } = useLanguage()
  const label = (key: string) => t(`traderDetail.${key}`, language)
  const models = useModelConfigs()
  const exchanges = useExchangeConfigs()
  const {
    data: traders,
    error,
    mutate,
  } = useSWR<TraderInfo[]>('traders', () => api.getTraders(true), {
    refreshInterval: 10000,
  })
  const trader = traders ? findTraderBySlug(slug, traders) : undefined
  const { data: config } = useSWR(
    trader ? ['trader-config', trader.trader_id] : null,
    () => api.getTraderConfig(trader!.trader_id, true),
    { refreshInterval: 120000, shouldRetryOnError: false }
  )
  const scanMinutes = config?.scan_interval_minutes
  const [editing, setEditing] = useState<TraderConfigData | null>(null)
  const [starting, setStarting] = useState(false)
  const [stopping, setStopping] = useState(false)

  const back = (
    <Link
      to={ROUTES.autopilot}
      aria-label={t('autopilotNav', language)}
      className="-ml-1 grid size-9 shrink-0 place-items-center rounded-lg text-vergex-text-muted hover:bg-vergex-bg hover:text-vergex-text"
    >
      <ChevronLeft size={20} aria-hidden="true" />
    </Link>
  )

  if (!trader) {
    return (
      <Page
        title={
          <span className="flex items-center gap-1">
            {back}
            {t('autopilotNav', language)}
          </span>
        }
      >
        <Panel>
          {!traders && !error ? (
            <PanelState role="status">{t('loading', language)}</PanelState>
          ) : error ? (
            <PanelState role="alert">{label('traderFailed')}</PanelState>
          ) : (
            <PanelState role="status">{label('notFound')}</PanelState>
          )}
        </Panel>
      </Page>
    )
  }

  const id = trader.trader_id
  const running = Boolean(trader.is_running)
  const base = buildTraderPath(getTraderSlug(trader))
  const tabs: PageTab[] = [
    { to: base, label: label('performance') },
    { to: `${base}/positions`, label: label('positions') },
    { to: `${base}/decisions`, label: label('decisions') },
    { to: `${base}/orders`, label: label('orders') },
  ]

  async function stop() {
    setStopping(true)
    try {
      await api.stopTrader(id)
      toast.success(t('aiTradersToast.stopped', language))
      await mutate()
    } catch {
      toast.error(t('operationFailed', language))
    } finally {
      setStopping(false)
    }
  }

  async function openEdit() {
    try {
      setEditing(await api.getTraderConfig(id))
    } catch {
      toast.error(t('getTraderConfigFailed', language))
    }
  }

  async function saveEdit(data: CreateTraderRequest) {
    const model = models.data?.find(
      (m) => m.id === data.ai_model_id && m.enabled
    )
    const exchange = exchanges.data?.find(
      (e) => e.id === data.exchange_id && e.enabled
    )
    if (!model) {
      toast.error(t('modelConfigNotExist', language))
      return
    }
    if (!exchange) {
      toast.error(t('exchangeConfigNotExist', language))
      return
    }
    try {
      await api.updateTrader(id, {
        name: data.name,
        ai_model_id: data.ai_model_id,
        exchange_id: data.exchange_id,
        strategy_id: data.strategy_id,
        scan_interval_minutes: data.scan_interval_minutes,
        is_cross_margin: data.is_cross_margin,
      })
      toast.success(t('aiTradersToast.saved', language))
      setEditing(null)
      const next = await mutate()
      const renamed = next?.find((item) => item.trader_id === id)
      if (renamed)
        navigate(buildTraderPath(getTraderSlug(renamed)), { replace: true })
    } catch {
      toast.error(t('updateTraderFailed', language))
    }
  }

  async function remove() {
    const ok = await confirmToast(t('confirmDeleteTrader', language), {
      tone: 'danger',
    })
    if (!ok) return
    try {
      await api.deleteTrader(id)
      toast.success(t('aiTradersToast.deleted', language))
      await mutate()
      navigate(ROUTES.autopilot)
    } catch {
      toast.error(t('deleteTraderFailed', language))
    }
  }

  const modelLabel = getTraderModelLabel(trader.ai_model, models.data ?? [])
  const accountLabel = getExchangeDisplayName(
    trader.exchange_id,
    exchanges.data ?? []
  )
  const description = [modelLabel, accountLabel, trader.strategy_name]
    .filter(Boolean)
    .join(' · ')

  return (
    <>
      <Page
        title={
          <span className="flex min-w-0 items-center gap-1">
            {back}
            <span className="truncate" title={trader.trader_name}>
              {trader.trader_name}
            </span>
            <span
              className={cn(
                'ml-2 shrink-0 rounded-md px-2 py-0.5 text-xs font-medium',
                running
                  ? 'bg-vergex-success/10 text-vergex-success'
                  : 'bg-vergex-bg text-vergex-text-muted'
              )}
            >
              {running ? label('running') : label('stopped')}
            </span>
          </span>
        }
        description={description}
        tabs={tabs}
        actions={
          <>
            <Button
              variant="secondary"
              disabled={running}
              title={running ? label('stopToEdit') : undefined}
              onClick={() => void openEdit()}
            >
              <Pencil size={14} aria-hidden="true" />
              {t('edit', language)}
            </Button>
            <Button
              variant="ghost"
              size="icon"
              className="size-10 text-vergex-danger hover:bg-vergex-danger/10 hover:text-vergex-danger"
              aria-label={t('delete', language)}
              title={t('delete', language)}
              onClick={() => void remove()}
            >
              <Trash2 size={16} aria-hidden="true" />
            </Button>
            <Button
              variant={running ? 'danger' : 'primary'}
              disabled={stopping}
              onClick={() => (running ? void stop() : setStarting(true))}
            >
              {running ? t('stop', language) : t('start', language)}
            </Button>
          </>
        }
      >
        <TraderSections trader={trader} section={section} />
      </Page>
      {starting && (
        <StartTraderDialog
          traderId={id}
          traderName={trader.trader_name}
          strategy={trader.strategy_name}
          model={modelLabel}
          account={accountLabel}
          scanMinutes={scanMinutes}
          language={language}
          onClose={() => setStarting(false)}
          onStarted={() => {
            setStarting(false)
            toast.success(t('aiTradersToast.started', language))
            void mutate()
          }}
        />
      )}
      {editing && (
        <TraderConfigModal
          isOpen
          isEditMode
          traderData={editing}
          availableModels={(models.data ?? []).filter((m) => m.enabled)}
          availableExchanges={(exchanges.data ?? []).filter((e) => e.enabled)}
          onSave={saveEdit}
          onClose={() => setEditing(null)}
        />
      )}
    </>
  )
}
