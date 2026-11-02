import { useEffect, useMemo, useState } from 'react'
import { CopyPlus, Pencil, Plus, Power, Trash2 } from 'lucide-react'
import { useQueryClient } from '@tanstack/react-query'
import { Link, useSearchParams } from 'react-router-dom'
import { api } from '@/api'
import {
  FORECAST_INTERVALS,
  type ForecastConfig,
  type ForecastStrategy,
} from '@/api/forecast'
import {
  RailItem,
  SplitRail,
  SplitView,
  type RailAction,
} from '@/components/common/split-view'
import { fieldClass } from '@/components/common/form-fields'
import { confirmToast, notify } from '@/components/common/notify'
import { Panel, PanelLoading, PanelState } from '@/components/common/panel'
import { SectionTabs } from '@/components/common/section-tabs'
import {
  SettingActions,
  SettingBadge,
  SettingRow,
} from '@/components/common/setting-row'
import { fmtDate } from '@/components/forecast/forecast-format'
import { ForecastFrame } from '@/components/forecast/forecast-frame'
import { strategiesQuery } from '@/components/forecast/forecast-queries'
import { EditableTitle, NameInput } from '@/components/common/name-editing'
import { StrategyPerformance } from '@/components/forecast/strategy-performance'
import {
  StrategyRules,
  StrategyRulesView,
} from '@/components/forecast/strategy-rules'
import { useStrategies } from '@/components/forecast/strategy-select'
import { Button, buttonVariants } from '@/components/ui/button'
import { ScrollArea } from '@/components/ui/scroll-area'
import { AppSelect } from '@/components/ui/select'
import { ROUTES } from '@/constants/route-paths'
import { useLanguage } from '@/contexts/language-context'
import { t } from '@/i18n'
import { cn } from '@/utils/cn'

type DetailTab = 'performance' | 'rules'

// What a new strategy starts from: a saved one's rules or the defaults.
const FROM_DEFAULTS = 'defaults'
const DRAFT_KEY = 'vergex.forecast.draft'

interface Draft {
  from: string
  name: string
  config: ForecastConfig
  // What it started as, to tell whether leaving it would lose anything.
  pristine: string
  // A try at saving shows what is missing.
  tried: boolean
}

// A draft outlives leaving the page, so a look at another tab loses nothing.
const savedDraft = (): Draft | null => {
  try {
    const saved = sessionStorage.getItem(DRAFT_KEY)
    return saved ? JSON.parse(saved) : null
  } catch {
    return null
  }
}

// A strategy's rules with every factor the code knows, so one saved before a factor
// existed can still turn it on.
const withEveryFactor = (config: ForecastConfig, defaults: ForecastConfig) => {
  const factors = { ...config.factors }
  for (const [name, factor] of Object.entries(defaults.factors)) {
    factors[name] ??= { ...factor, enabled: false }
  }
  return structuredClone({ ...config, factors })
}

// Forecast strategies: the list beside one strategy's record and rules. A saved one's
// rules cannot change, so a different idea starts as a new strategy from any of them.
export function ForecastStrategiesPage() {
  const { language } = useLanguage()
  const label = (key: string, params?: Record<string, string | number>) =>
    t(`forecast.${key}`, language, params)
  const client = useQueryClient()
  const { data, error, reload, toggle } = useStrategies(language)
  const [params, setParams] = useSearchParams()
  const selectedId = params.get('id')
  const tab: DetailTab = params.get('tab') === 'rules' ? 'rules' : 'performance'
  const [draft, setDraft] = useState<Draft | null>(savedDraft)
  const [renaming, setRenaming] = useState<string | null>(null)
  const [renamingHere, setRenamingHere] = useState(false)
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    try {
      if (draft) sessionStorage.setItem(DRAFT_KEY, JSON.stringify(draft))
      else sessionStorage.removeItem(DRAFT_KEY)
    } catch {
      /* Only a convenience: the draft just won't outlive the page. */
    }
  }, [draft])

  const select = (id: string | null, nextTab: DetailTab = tab) => {
    const next = new URLSearchParams()
    if (id) next.set('id', id)
    if (nextTab === 'rules') next.set('tab', nextTab)
    setParams(next, { replace: true })
  }

  const selected: ForecastStrategy | undefined = useMemo(
    () =>
      data?.strategies.find((strategy) => strategy.id === selectedId) ??
      data?.strategies.find(
        (strategy) => strategy.id === data.active_strategy_id
      ) ??
      data?.strategies[0],
    [data, selectedId]
  )

  const rulesFrom = (from: string): ForecastConfig | null => {
    if (!data) return null
    const source = data.strategies.find((strategy) => strategy.id === from)
    const config = withEveryFactor(
      source?.config ?? data.default_config,
      data.default_config
    )
    // Rules saved before the limits still have to meet them to be saved again.
    const horizons = { ...config.horizons }
    for (const interval of FORECAST_INTERVALS) {
      horizons[interval] = Math.min(
        horizons[interval],
        data.horizon_limits[interval]
      )
    }
    return { ...config, horizons }
  }

  // Leaving a draft with changes asks first. It reports whether the draft is gone.
  const leaveDraft = async () => {
    if (
      draft &&
      (JSON.stringify(draft.config) !== draft.pristine || draft.name.trim()) &&
      !(await confirmToast(label('discardMessage'), {
        title: label('discardTitle'),
        okText: label('discard'),
        cancelText: label('keepEditing'),
        tone: 'danger',
      }))
    ) {
      return false
    }
    setDraft(null)
    return true
  }

  const startDraft = async (from = selected?.id ?? FROM_DEFAULTS) => {
    if (!(await leaveDraft())) return
    const config = rulesFrom(from)
    if (!config) return
    setDraft({
      from,
      name: '',
      config,
      pristine: JSON.stringify(config),
      tried: false,
    })
  }

  const chooseFrom = (from: string) => {
    const config = rulesFrom(from)
    if (!draft || !config) return
    setDraft({ ...draft, from, config, pristine: JSON.stringify(config) })
  }

  const choose = async (id: string) => {
    if (await leaveDraft()) select(id)
  }

  const noneOn =
    draft &&
    !Object.values(draft.config.factors).some((factor) => factor.enabled)
  // A market with no weight has no call to make, and would wait forever for one.
  const oneSided =
    draft &&
    !(['trending', 'ranging'] as const).every((market) =>
      Object.values(draft.config.factors).some(
        (factor) => factor.enabled && factor[market] !== 0
      )
    )
  const draftMessage = !draft?.tried
    ? undefined
    : noneOn
      ? label('needFactor')
      : oneSided
        ? label('needBothMarkets')
        : !draft.name.trim()
          ? label('needName')
          : undefined

  const save = async () => {
    if (!draft) return
    if (noneOn || oneSided || !draft.name.trim()) {
      setDraft({ ...draft, tried: true })
      return
    }
    setBusy(true)
    try {
      const strategy = await api.createForecastStrategy(
        draft.config,
        draft.name.trim()
      )
      await reload()
      setDraft(null)
      select(strategy.id, 'rules')
      notify.success(label('strategySaved', { name: strategy.name }))
    } catch (err) {
      notify.error(err instanceof Error ? err.message : label('saveFailed'))
    } finally {
      setBusy(false)
    }
  }

  const rename = async (strategy: ForecastStrategy, name: string) => {
    setRenaming(null)
    setRenamingHere(false)
    if (!data) return
    const renamed = {
      ...data,
      strategies: data.strategies.map((other) =>
        other.id === strategy.id ? { ...other, name } : other
      ),
    }
    client.setQueryData(strategiesQuery.queryKey, renamed)
    try {
      await api.renameForecastStrategy(strategy.id, name)
    } catch {
      client.setQueryData(strategiesQuery.queryKey, data)
      notify.error(label('renameFailed'))
    }
  }

  const remove = async (strategy: ForecastStrategy) => {
    const ok = await confirmToast(
      label('deleteMessage', { name: strategy.name }),
      {
        title: label('deleteStrategy'),
        okText: label('deleteStrategy'),
        cancelText: label('deleteCancel'),
        tone: 'danger',
      }
    )
    if (!ok) return
    setBusy(true)
    try {
      await api.deleteForecastStrategy(strategy.id)
      if (strategy.id === selected?.id) select(null)
      await reload()
      notify.success(label('strategyDeleted', { name: strategy.name }))
    } catch (err) {
      notify.error(err instanceof Error ? err.message : label('deleteFailed'))
    } finally {
      setBusy(false)
    }
  }

  const nameTaken = (strategy: ForecastStrategy) => (name: string) =>
    data?.strategies.some(
      (other) => other.id !== strategy.id && other.name === name
    ) ?? false

  const isActive = selected?.id === data?.active_strategy_id
  const lastOne = data?.strategies.length === 1
  const badges = (strategy: ForecastStrategy) => (
    <>
      {strategy.id === data?.active_strategy_id && (
        <SettingBadge tone="success">{label('inUse')}</SettingBadge>
      )}
      {strategy.retired && (
        <span title={label('retiredTitle')} className="inline-flex">
          <SettingBadge>{label('retired')}</SettingBadge>
        </span>
      )}
    </>
  )
  const actionsFor = (strategy: ForecastStrategy): RailAction[] => [
    {
      label:
        strategy.id === data?.active_strategy_id
          ? label('stopStrategy')
          : label('useStrategy'),
      icon: Power,
      onSelect: () => void toggle(strategy),
    },
    {
      label: label('rename'),
      icon: Pencil,
      movesFocus: true,
      onSelect: () => setRenaming(strategy.id),
    },
    {
      label: label('newFromThis'),
      icon: CopyPlus,
      movesFocus: true,
      onSelect: () => void startDraft(strategy.id),
    },
    {
      label: label('deleteStrategy'),
      icon: Trash2,
      danger: true,
      disabled: strategy.id === data?.active_strategy_id || lastOne,
      onSelect: () => void remove(strategy),
    },
  ]

  return (
    <ForecastFrame language={language}>
      {error && !data ? (
        <Panel>
          <PanelState role="alert">{label('loadFailed')}</PanelState>
        </Panel>
      ) : !data || !selected ? (
        <Panel>
          <PanelLoading />
        </Panel>
      ) : (
        <SplitView
          rail={
            <SplitRail
              label={label('tabStrategies')}
              addLabel={label('newStrategy')}
              onAdd={() => {
                if (!draft) void startDraft()
              }}
            >
              {draft && (
                <RailItem
                  current
                  name={draft.name.trim() || label('newStrategy')}
                  details={
                    <span className="text-xs text-vergex-muted">
                      {label('notSaved')}
                    </span>
                  }
                  onOpen={() => undefined}
                />
              )}
              {data.strategies.map((strategy) => (
                <RailItem
                  key={strategy.id}
                  current={!draft && selected.id === strategy.id}
                  title={strategy.name}
                  name={strategy.name}
                  details={
                    <span className="flex min-w-0 items-center gap-2 text-xs text-vergex-muted">
                      <span className="truncate">
                        {fmtDate(strategy.created_at)}
                      </span>
                      {badges(strategy)}
                    </span>
                  }
                  onOpen={() => void choose(strategy.id)}
                  onStartRename={() => setRenaming(strategy.id)}
                  rename={
                    renaming === strategy.id && (
                      <NameInput
                        row
                        value={strategy.name}
                        label={label('strategyName')}
                        isTaken={nameTaken(strategy)}
                        takenLabel={label('nameTaken')}
                        language={language}
                        onSave={(name) => void rename(strategy, name)}
                        onCancel={() => setRenaming(null)}
                      />
                    )
                  }
                  actions={actionsFor(strategy)}
                  actionsLabel={label('moreActions')}
                />
              ))}
            </SplitRail>
          }
        >
          <div className="flex min-h-0 min-w-0 flex-col gap-4">
            <div className="flex items-center gap-2 lg:hidden">
              <AppSelect
                aria-label={label('strategy')}
                className="min-w-0 flex-1 max-sm:h-10"
                value={selected.id}
                options={data.strategies.map((strategy) => ({
                  value: strategy.id,
                  label:
                    strategy.name +
                    (strategy.id === data.active_strategy_id
                      ? ` · ${label('inUse')}`
                      : ''),
                }))}
                onChange={(id) => void choose(id)}
              />
              <Button
                type="button"
                variant="secondary"
                size="icon"
                aria-label={label('newStrategy')}
                className="shrink-0 max-sm:size-10"
                onClick={() => {
                  if (!draft) void startDraft()
                }}
              >
                <Plus className="size-4" aria-hidden="true" />
              </Button>
            </div>

            {draft ? (
              <Panel
                title={label('newStrategy')}
                className="flex min-h-0 flex-1 flex-col overflow-hidden"
              >
                <ScrollArea className="min-h-0 flex-1 border-t border-vergex-line">
                  <div className="px-4">
                    <SettingRow label={label('strategyName')} htmlFor="fc-name">
                      <input
                        id="fc-name"
                        autoFocus
                        className={fieldClass}
                        value={draft.name}
                        maxLength={80}
                        onChange={(event) =>
                          setDraft({ ...draft, name: event.target.value })
                        }
                        onKeyDown={(event) => {
                          if (event.key === 'Enter') void save()
                        }}
                      />
                    </SettingRow>
                    <SettingRow label={label('startFrom')}>
                      <AppSelect
                        aria-label={label('startFrom')}
                        className="min-w-0 flex-1 max-sm:h-10"
                        value={draft.from}
                        options={[
                          {
                            value: FROM_DEFAULTS,
                            label: label('defaultRules'),
                          },
                          ...data.strategies.map((strategy) => ({
                            value: strategy.id,
                            label: strategy.name,
                          })),
                        ]}
                        onChange={chooseFrom}
                      />
                    </SettingRow>
                  </div>
                  <div className="border-t border-vergex-line">
                    <StrategyRules
                      config={draft.config}
                      horizonLimits={data?.horizon_limits ?? {}}
                      onChange={(config) => setDraft({ ...draft, config })}
                      language={language}
                    />
                  </div>
                </ScrollArea>
                <div className="shrink-0 bg-vergex-surface px-4">
                  <SettingActions message={draftMessage}>
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      className="max-sm:h-10"
                      onClick={() => void leaveDraft()}
                    >
                      {label('cancelDraft')}
                    </Button>
                    <Button
                      type="button"
                      variant="primary"
                      size="sm"
                      className="max-sm:h-10"
                      disabled={busy}
                      onClick={() => void save()}
                    >
                      {label('saveStrategy')}
                    </Button>
                  </SettingActions>
                </div>
              </Panel>
            ) : (
              <Panel
                title={
                  <EditableTitle
                    name={selected.name}
                    editing={renamingHere}
                    renameLabel={label('rename')}
                    onEdit={() => setRenamingHere(true)}
                    field={
                      <NameInput
                        value={selected.name}
                        label={label('strategyName')}
                        isTaken={nameTaken(selected)}
                        takenLabel={label('nameTaken')}
                        language={language}
                        onSave={(name) => void rename(selected, name)}
                        onCancel={() => setRenamingHere(false)}
                        className="max-w-64 md:max-w-96"
                      />
                    }
                  >
                    {badges(selected)}
                  </EditableTitle>
                }
                meta={
                  <span
                    className={cn(
                      'block truncate',
                      renamingHere && 'invisible'
                    )}
                  >
                    {fmtDate(selected.created_at)}
                  </span>
                }
                actions={
                  <>
                    <Button
                      type="button"
                      variant={isActive ? 'secondary' : 'primary'}
                      size="sm"
                      className="max-sm:h-10"
                      onClick={() => void toggle(selected)}
                    >
                      {isActive ? label('stopStrategy') : label('useStrategy')}
                    </Button>
                    {!selected.retired && (
                      <Link
                        to={`${ROUTES.forecastBacktests}?strategy=${selected.id}`}
                        className={buttonVariants({
                          variant: 'secondary',
                          size: 'sm',
                          className: 'max-sm:h-10',
                        })}
                      >
                        {label('backtestThis')}
                      </Link>
                    )}
                    <Button
                      type="button"
                      variant="ghost"
                      size="icon"
                      className="hover:text-vergex-danger max-sm:size-10"
                      aria-label={label('deleteStrategy')}
                      title={
                        isActive
                          ? label('inUseNoDelete')
                          : lastOne
                            ? label('lastNoDelete')
                            : label('deleteStrategy')
                      }
                      disabled={isActive || lastOne || busy}
                      onClick={() => void remove(selected)}
                    >
                      <Trash2 className="size-4" />
                    </Button>
                  </>
                }
                className="flex min-h-0 flex-1 flex-col overflow-hidden"
              >
                <SectionTabs
                  tabs={[
                    { id: 'performance', label: label('performance') },
                    { id: 'rules', label: label('rules') },
                  ]}
                  value={tab}
                  onChange={(next) => select(selected.id, next)}
                  label={label('strategyTabs')}
                  className="mb-0! shrink-0 px-2"
                />
                {/* A new strategy opens at the top of its own content. */}
                <ScrollArea
                  key={selected.id}
                  className="min-h-0 flex-1 rounded-b-2xl"
                >
                  {tab === 'performance' ? (
                    <StrategyPerformance
                      strategyId={selected.id}
                      retired={selected.retired}
                      inUse={isActive}
                      language={language}
                    />
                  ) : (
                    <StrategyRulesView
                      config={selected.config}
                      language={language}
                    />
                  )}
                </ScrollArea>
              </Panel>
            )}
          </div>
        </SplitView>
      )}
    </ForecastFrame>
  )
}
