import { useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { Copy, Pencil, Plus, Power, Trash2 } from 'lucide-react'
import { toast } from 'sonner'
import { api } from '@/api'
import { confirmToast, notify } from '@/components/common/notify'
import { Skeleton } from '@/components/ui/skeleton'
import { NameInput } from '@/components/common/name-editing'
import { Panel, PanelLoading, PanelState } from '@/components/common/panel'
import { SearchField } from '@/components/common/search-field'
import {
  RailItem,
  SplitRail,
  SplitView,
  type RailAction,
} from '@/components/common/split-view'
import { useOpenSettings } from '@/components/settings/use-open-settings'
import { StartTraderDialog } from '@/components/trader/start-trader-dialog'
import { NewTraderDialog } from '@/components/trader/trader-dialog'
import {
  readTraderTab,
  TraderPanel,
  type TraderTab,
} from '@/components/trader/trader-panel'
import {
  traderMetricsQuery,
  tradersQuery,
} from '@/components/trader/trader-queries'
import { NoTraders, sortTraders } from '@/components/trader/traders-list'
import { TradersFrame } from '@/components/trader/traders-frame'
import { Button } from '@/components/ui/button'
import { AppSelect } from '@/components/ui/select'
import {
  getAccountDisplayName,
  getTraderModelLabel,
} from '@/constants/model-providers'
import { buildTraderPath } from '@/constants/route-paths'
import { useAuth } from '@/contexts/auth-context'
import { useLanguage } from '@/contexts/language-context'
import {
  configuredAccounts as savedAccounts,
  configuredModels as savedModels,
  useAccounts,
  useModelConfigs,
} from '@/hooks/use-connections'
import { t } from '@/i18n'
import type { CreateTraderRequest, TraderInfo } from '@/types'
import { cn } from '@/utils/cn'
import { fmtPct } from '@/utils/trader-format'

// Past this many traders the list gets a search box.
const SEARCH_FROM = 8
const LAST_TRADER_KEY = 'vergex.traders.last'

const lastTrader = () => {
  try {
    return localStorage.getItem(LAST_TRADER_KEY)
  } catch {
    return null
  }
}

const rememberTrader = (id: string) => {
  try {
    localStorage.setItem(LAST_TRADER_KEY, id)
  } catch {
    /* Only a convenience: the next visit opens the first trader instead. */
  }
}

// The traders, each opened beside the list in place. Models and exchange accounts are set
// up in settings. The address names the trader and its tab, and without them the page
// opens the last one looked at.
export function TradersPage() {
  const { language } = useLanguage()
  const { user, token } = useAuth()
  const navigate = useNavigate()
  const openSettings = useOpenSettings()
  const [params, setParams] = useSearchParams()
  const label = (key: string, params?: Record<string, string | number>) =>
    t(`traderList.${key}`, language, params)
  const detail = (key: string) => t(`traderDetail.${key}`, language)

  const [creating, setCreating] = useState(false)
  const [starting, setStarting] = useState<TraderInfo | null>(null)
  const [stopping, setStopping] = useState(false)
  const [search, setSearch] = useState('')
  // Where the name is being edited: in the list, or in the panel's title.
  const [renaming, setRenaming] = useState<{
    id: string
    inRail: boolean
  } | null>(null)
  // A changed draft of the open trader's settings is worth asking about before it goes.
  const dirty = useRef(false)

  const models = useModelConfigs()
  const accounts = useAccounts()
  const allModels = models.data ?? []
  const allAccounts = accounts.data ?? []
  const configError = Boolean(models.error || accounts.error)
  useEffect(() => {
    if (configError) notify.error(label('connectionsFailed'))
  }, [configError])

  const {
    data: traders,
    error: tradersError,
    refetch,
  } = useQuery({ ...tradersQuery, enabled: Boolean(user && token) })
  const { data: metrics } = useQuery({
    ...traderMetricsQuery,
    enabled: Boolean(user && token),
  })

  const configuredModels = savedModels(allModels)
  const configuredAccounts = savedAccounts(allAccounts)

  const describe = (trader: TraderInfo) =>
    [
      label(trader.kind === 'grid' ? 'kindGrid' : 'kindAgent'),
      getTraderModelLabel(trader.agent_model, allModels),
    ].join(' · ')

  const rows = useMemo(() => {
    const term = search.trim().toLowerCase()
    return sortTraders(
      (traders ?? []).filter(
        (trader) =>
          !term ||
          `${trader.trader_name} ${describe(trader)} ${getAccountDisplayName(trader.account_id, allAccounts)}`
            .toLowerCase()
            .includes(term)
      ),
      metrics,
      null
    )
  }, [traders, metrics, allModels, allAccounts, search])

  const asked = params.get('id')
  const remembered = lastTrader()
  const selected =
    traders?.find((trader) => trader.trader_id === asked) ??
    traders?.find((trader) => trader.trader_id === remembered) ??
    rows[0]
  const tab = readTraderTab(params.get('tab'))

  useEffect(() => {
    if (selected) rememberTrader(selected.trader_id)
  }, [selected?.trader_id])

  const select = async (id: string, next: TraderTab = tab) => {
    if (id !== selected?.trader_id && dirty.current) {
      const ok = await confirmToast(t('traderSettings.leaveDraft', language), {
        okText: t('traderSettings.discardDraft', language),
        cancelText: t('traderSettings.keepEditing', language),
        tone: 'danger',
      })
      if (!ok) return
      dirty.current = false
    }
    setRenaming(null)
    setParams(
      (current) => {
        const kept = new URLSearchParams(current)
        kept.set('id', id)
        if (next === 'performance') kept.delete('tab')
        else kept.set('tab', next)
        return kept
      },
      { replace: true }
    )
  }

  // Without what a trader needs, adding one leads to where it is set up.
  const add = () => {
    if (models.isLoading || accounts.isLoading) return
    if (!configuredModels.length) openSettings('models')
    else if (!configuredAccounts.length) openSettings('accounts')
    else setCreating(true)
  }

  const create = async (data: CreateTraderRequest) => {
    try {
      const model = allModels.find((m) => m.id === data.agent_model_id)
      const account = allAccounts.find((e) => e.id === data.account_id)
      if (!model?.enabled) {
        toast.error(t('modelNotConfigured', language))
        return
      }
      if (!account) {
        toast.error(t('accountNotConfigured', language))
        return
      }
      const created = await api.createTrader(data)
      toast.success(t('agentTradersToast.created', language))
      setCreating(false)
      await refetch()
      navigate(buildTraderPath(created.trader_id))
    } catch {
      toast.error(t('createTraderFailed', language))
    }
  }

  const startStop = async (trader: TraderInfo) => {
    if (!trader.is_running) {
      setStarting(trader)
      return
    }
    setStopping(true)
    try {
      const { ordersLeft } = await api.stopTrader(trader.trader_id)
      if (ordersLeft) toast.error(t('agentTradersToast.ordersLeft', language))
      else toast.success(t('agentTradersToast.stopped', language))
      await refetch()
    } catch {
      toast.error(t('operationFailed', language))
    } finally {
      setStopping(false)
    }
  }

  const rename = async (trader: TraderInfo, name: string) => {
    setRenaming(null)
    try {
      await api.updateTrader(trader.trader_id, { name })
      await refetch()
    } catch {
      toast.error(t('updateTraderFailed', language))
    }
  }

  const duplicate = async (trader: TraderInfo) => {
    try {
      const copy = await api.duplicateTrader(
        trader.trader_id,
        detail('copyName').replace('{name}', trader.trader_name)
      )
      toast.success(detail('duplicated'))
      await refetch()
      await select(copy.trader_id, 'performance')
    } catch {
      toast.error(detail('duplicateFailed'))
    }
  }

  const remove = async (trader: TraderInfo) => {
    const ok = await confirmToast(t('confirmDeleteTrader', language), {
      tone: 'danger',
    })
    if (!ok) return
    try {
      await api.deleteTrader(trader.trader_id)
      toast.success(t('agentTradersToast.deleted', language))
      if (trader.trader_id === selected?.trader_id) dirty.current = false
      await refetch()
      if (trader.trader_id === asked) {
        setParams(
          (current) => {
            current.delete('id')
            current.delete('tab')
            return current
          },
          { replace: true }
        )
      }
    } catch {
      toast.error(t('deleteTraderFailed', language))
    }
  }

  // A rename restarts a running trader, so as with the rest of what it runs with, a
  // trader is stopped first.
  const actionsFor = (trader: TraderInfo, inRail: boolean): RailAction[] => [
    {
      label: trader.is_running ? t('stop', language) : t('start', language),
      icon: Power,
      onSelect: () => void startStop(trader),
    },
    {
      label: label('rename'),
      icon: Pencil,
      movesFocus: true,
      disabled: Boolean(trader.is_running),
      onSelect: () => setRenaming({ id: trader.trader_id, inRail }),
    },
    {
      label: detail('duplicate'),
      icon: Copy,
      onSelect: () => void duplicate(trader),
    },
    {
      label: t('delete', language),
      icon: Trash2,
      danger: true,
      onSelect: () => void remove(trader),
    },
  ]

  return (
    <TradersFrame language={language}>
      {!traders ? (
        <Panel>
          {tradersError ? (
            <PanelState role="alert">{label('loadFailed')}</PanelState>
          ) : (
            <PanelLoading />
          )}
        </Panel>
      ) : traders.length === 0 ? (
        <Panel>
          <NoTraders
            modelCount={configuredModels.length}
            accountCount={configuredAccounts.length}
            language={language}
            create={
              <Button
                type="button"
                variant="primary"
                className="max-sm:h-10"
                disabled={configError}
                onClick={add}
              >
                <Plus className="size-4" aria-hidden="true" />
                {t('createTrader', language)}
              </Button>
            }
          />
        </Panel>
      ) : (
        <SplitView
          rail={
            <SplitRail
              label={label('tabs')}
              addLabel={t('createTrader', language)}
              onAdd={add}
              top={
                traders.length >= SEARCH_FROM && (
                  <SearchField
                    label={label('search')}
                    value={search}
                    onChange={setSearch}
                    className="w-full"
                  />
                )
              }
            >
              {rows.length === 0 && (
                <li className="px-3 py-2 text-xs text-vergex-muted">
                  {label('noMatch')}
                </li>
              )}
              {rows.map((trader) => {
                const change = metrics?.[trader.trader_id]?.return_pct
                const running = Boolean(trader.is_running)
                return (
                  <RailItem
                    key={trader.trader_id}
                    current={trader.trader_id === selected?.trader_id}
                    title={trader.trader_name}
                    name={trader.trader_name}
                    details={
                      <span className="flex min-w-0 items-center gap-2 text-xs font-normal text-vergex-muted">
                        <span
                          role="img"
                          aria-label={
                            running ? label('running') : label('stopped')
                          }
                          className={cn(
                            'size-1.5 shrink-0 rounded-full',
                            running
                              ? 'bg-vergex-success'
                              : 'bg-vergex-line-strong'
                          )}
                        />
                        <span className="truncate">{describe(trader)}</span>
                        <span
                          className={cn(
                            'mono ml-auto shrink-0',
                            change
                              ? change > 0
                                ? 'text-vergex-success'
                                : 'text-vergex-danger'
                              : undefined
                          )}
                        >
                          {metrics ? fmtPct(change) : <Skeleton />}
                        </span>
                      </span>
                    }
                    onOpen={() => void select(trader.trader_id)}
                    onStartRename={
                      running
                        ? undefined
                        : () =>
                            setRenaming({
                              id: trader.trader_id,
                              inRail: true,
                            })
                    }
                    rename={
                      renaming?.id === trader.trader_id &&
                      renaming.inRail && (
                        <NameInput
                          row
                          value={trader.trader_name}
                          label={label('traderName')}
                          language={language}
                          onSave={(name) => void rename(trader, name)}
                          onCancel={() => setRenaming(null)}
                        />
                      )
                    }
                    actions={actionsFor(trader, true)}
                    actionsLabel={label('moreActions')}
                  />
                )
              })}
            </SplitRail>
          }
        >
          <div className="flex min-h-0 min-w-0 flex-col gap-4">
            <div className="flex items-center gap-2 lg:hidden">
              <AppSelect
                aria-label={label('tabs')}
                className="min-w-0 flex-1 max-sm:h-10"
                value={selected?.trader_id ?? ''}
                options={rows.map((trader) => ({
                  value: trader.trader_id,
                  label: trader.trader_name,
                }))}
                onChange={(id) => void select(id)}
              />
              <Button
                type="button"
                variant="secondary"
                size="icon"
                aria-label={t('createTrader', language)}
                className="shrink-0 max-sm:size-10"
                onClick={add}
              >
                <Plus className="size-4" aria-hidden="true" />
              </Button>
            </div>
            {selected && (
              <TraderPanel
                key={selected.trader_id}
                trader={selected}
                tab={tab}
                models={allModels}
                accounts={allAccounts}
                actions={actionsFor(selected, false)}
                renaming={
                  renaming?.id === selected.trader_id && !renaming.inRail
                }
                stopping={stopping}
                language={language}
                onTab={(next) => void select(selected.trader_id, next)}
                onStartStop={() => void startStop(selected)}
                onRenaming={(next) =>
                  setRenaming(
                    next ? { id: selected.trader_id, inRail: false } : null
                  )
                }
                onRename={(name) => void rename(selected, name)}
                onDirty={(next) => {
                  dirty.current = next
                }}
              />
            )}
          </div>
        </SplitView>
      )}
      {creating && (
        <NewTraderDialog
          models={allModels.filter((m) => m.enabled)}
          accounts={allAccounts}
          onCreate={create}
          onClose={() => setCreating(false)}
        />
      )}
      {starting && (
        <StartTraderDialog
          traderId={starting.trader_id}
          traderName={starting.trader_name}
          kind={starting.kind}
          model={getTraderModelLabel(starting.agent_model, allModels)}
          account={getAccountDisplayName(starting.account_id, allAccounts)}
          scanMinutes={starting.scan_interval_minutes}
          language={language}
          onClose={() => setStarting(null)}
          onStarted={() => {
            setStarting(null)
            toast.success(t('agentTradersToast.started', language))
            void refetch()
          }}
        />
      )}
    </TradersFrame>
  )
}
