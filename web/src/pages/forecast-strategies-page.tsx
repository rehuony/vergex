import { useEffect, useMemo, useState } from 'react'
import { Trash2 } from 'lucide-react'
import { Link } from 'react-router-dom'
import useSWR from 'swr'
import { api } from '@/api'
import {
  FORECAST_INTERVALS,
  type FactorConfig,
  type ForecastConfig,
  type ForecastStrategy,
} from '@/api/forecast'
import {
  FieldGrid,
  NumberField,
  fieldClass,
} from '@/components/common/form-fields'
import { confirmToast, notify } from '@/components/common/notify'
import { Panel, PanelState } from '@/components/common/panel'
import { fmtDate } from '@/components/forecast/forecast-format'
import { ForecastFrame } from '@/components/forecast/forecast-frame'
import {
  SettingActions,
  SettingBadge,
  SettingRow,
} from '@/components/common/setting-row'
import { Button, buttonVariants } from '@/components/ui/button'
import { ScrollArea } from '@/components/ui/scroll-area'
import { AppSelect } from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { ROUTES } from '@/constants/route-paths'
import { useLanguage } from '@/contexts/language-context'
import { t } from '@/i18n/translations'

const FACTOR_ORDER = [
  'trend',
  'momentum',
  'rsi',
  'bollinger',
  'returns',
  'volume',
  'candle',
  'structure',
  'stochastic',
  'vwap',
  'divergence',
  'funding',
  'open_interest',
  'long_short',
]

const sameConfig = (a: ForecastConfig, b: ForecastConfig) =>
  JSON.stringify(sortKeys(a)) === JSON.stringify(sortKeys(b))

function sortKeys(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(sortKeys)
  if (value && typeof value === 'object') {
    return Object.fromEntries(
      Object.entries(value)
        .sort(([a], [b]) => a.localeCompare(b))
        .map(([key, item]) => [key, sortKeys(item)])
    )
  }
  return value
}

// Forecast strategies: a saved one cannot change, so editing one and saving always
// makes a new one to backtest and then put to use.
export function ForecastStrategiesPage() {
  const { language } = useLanguage()
  const label = (key: string, params?: Record<string, string | number>) =>
    t(`forecast.${key}`, language, params)
  const { data, error, mutate } = useSWR(
    'forecast-strategies',
    api.getForecastStrategies,
    { revalidateOnFocus: false }
  )
  const [selectedId, setSelectedId] = useState<number | null>(null)
  const [draft, setDraft] = useState<ForecastConfig | null>(null)
  const [name, setName] = useState('')
  const [busy, setBusy] = useState(false)

  const selected: ForecastStrategy | undefined = useMemo(
    () =>
      data?.strategies.find((strategy) => strategy.id === selectedId) ??
      data?.strategies.find(
        (strategy) => strategy.id === data.active_strategy_id
      ),
    [data, selectedId]
  )

  useEffect(() => {
    if (selected) {
      setDraft(structuredClone(selected.config))
      setName('')
    }
  }, [selected])

  const changed = Boolean(
    draft && selected && !sameConfig(draft, selected.config)
  )

  const choose = async (id: number) => {
    if (id === selected?.id) return
    if (
      changed &&
      !(await confirmToast(label('discardMessage'), {
        title: label('discardTitle'),
        okText: label('discard'),
        cancelText: label('keepEditing'),
        tone: 'danger',
      }))
    ) {
      return
    }
    setSelectedId(id)
  }

  const save = async () => {
    if (!draft) return
    setBusy(true)
    try {
      const strategy = await api.createForecastStrategy(draft, name.trim())
      await mutate()
      setSelectedId(strategy.id)
      notify.success(label('strategySaved', { name: strategy.name }))
    } catch (err) {
      notify.error(err instanceof Error ? err.message : label('saveFailed'))
    } finally {
      setBusy(false)
    }
  }

  const remove = async () => {
    if (!selected) return
    const ok = await confirmToast(
      label('deleteMessage', { name: selected.name }),
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
      await api.deleteForecastStrategy(selected.id)
      setSelectedId(null)
      await mutate()
      notify.success(label('strategyDeleted', { name: selected.name }))
    } catch (err) {
      notify.error(err instanceof Error ? err.message : label('deleteFailed'))
    } finally {
      setBusy(false)
    }
  }

  const patch = (next: Partial<ForecastConfig>) =>
    setDraft((current) => (current ? { ...current, ...next } : current))
  const patchFactor = (name: string, next: Partial<FactorConfig>) =>
    setDraft((current) =>
      current
        ? {
            ...current,
            factors: {
              ...current.factors,
              [name]: { ...current.factors[name], ...next },
            },
          }
        : current
    )

  const isActive = selected?.id === data?.active_strategy_id
  const noneOn =
    draft && !Object.values(draft.factors).some((factor) => factor.enabled)
  return (
    <ForecastFrame language={language}>
      {error ? (
        <Panel>
          <PanelState role="alert">{label('loadFailed')}</PanelState>
        </Panel>
      ) : !data || !draft || !selected ? (
        <Panel>
          <PanelState role="status">{label('loading')}</PanelState>
        </Panel>
      ) : (
        <div className="grid grid-cols-1 items-start gap-4 md:grid-cols-[260px_minmax(0,1fr)]">
          <Panel className="hidden md:block">
            <ScrollArea className="max-h-[70vh] rounded-xl">
              <ul className="p-1.5">
                {data.strategies.map((strategy) => (
                  <li key={strategy.id}>
                    <Button
                      type="button"
                      variant="ghost"
                      aria-current={
                        selected.id === strategy.id ? 'true' : undefined
                      }
                      title={strategy.name}
                      onClick={() => void choose(strategy.id)}
                      className="h-auto w-full flex-col items-stretch gap-0.5 whitespace-normal px-2.5 py-2 text-left hover:text-vergex-text-muted aria-[current=true]:bg-[var(--brand-soft)]"
                    >
                      <span className="flex min-w-0 items-center gap-2">
                        <span className="min-w-0 flex-1 truncate text-sm font-medium text-vergex-text">
                          {strategy.name}
                        </span>
                        {strategy.id === data.active_strategy_id && (
                          <SettingBadge tone="success">
                            {label('inUse')}
                          </SettingBadge>
                        )}
                      </span>
                      <span className="text-xs font-normal">
                        {fmtDate(strategy.created_at)}
                      </span>
                    </Button>
                  </li>
                ))}
              </ul>
            </ScrollArea>
          </Panel>

          <div className="flex min-w-0 flex-col gap-4">
            <AppSelect
              aria-label={label('strategy')}
              className="md:hidden"
              value={selected.id}
              options={data.strategies.map((strategy) => ({
                value: strategy.id,
                label:
                  strategy.name +
                  (strategy.id === data.active_strategy_id
                    ? ` · ${label('inUse')}`
                    : ''),
              }))}
              onChange={(id) => void choose(Number(id))}
            />
            <Panel
              title={
                <span className="block truncate" title={selected.name}>
                  {selected.name}
                </span>
              }
              meta={fmtDate(selected.created_at)}
              actions={
                <>
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
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon"
                    className="hover:text-vergex-danger max-sm:size-10"
                    aria-label={label('deleteStrategy')}
                    title={
                      isActive
                        ? label('inUseNoDelete')
                        : label('deleteStrategy')
                    }
                    disabled={isActive || busy}
                    onClick={() => void remove()}
                  >
                    <Trash2 className="size-4" />
                  </Button>
                </>
              }
            >
              <div className="border-t border-[var(--line)] px-4">
                <FieldGrid>
                  <SettingRow label={label('adxThreshold')} htmlFor="fc-adx">
                    <NumberField
                      id="fc-adx"
                      value={draft.adx_threshold}
                      min={5}
                      max={60}
                      onChange={(adx_threshold) => patch({ adx_threshold })}
                    />
                  </SettingRow>
                  <SettingRow label={label('slope')} htmlFor="fc-slope">
                    <NumberField
                      id="fc-slope"
                      value={draft.slope}
                      min={0.1}
                      max={20}
                      step={0.1}
                      onChange={(slope) => patch({ slope })}
                    />
                  </SettingRow>
                  <SettingRow
                    label={label('directionThreshold')}
                    htmlFor="fc-direction"
                  >
                    <NumberField
                      id="fc-direction"
                      value={Math.round(draft.direction_threshold * 100)}
                      min={0}
                      max={49}
                      suffix={label('pointsFrom50')}
                      onChange={(points) =>
                        patch({ direction_threshold: points / 100 })
                      }
                    />
                  </SettingRow>
                  {FORECAST_INTERVALS.map((interval) => (
                    <SettingRow
                      key={interval}
                      label={label('horizonFor', { interval })}
                      htmlFor={`fc-horizon-${interval}`}
                    >
                      <NumberField
                        id={`fc-horizon-${interval}`}
                        value={draft.horizons[interval] ?? 4}
                        min={1}
                        max={96}
                        suffix={label('bars')}
                        onChange={(bars) =>
                          patch({
                            horizons: { ...draft.horizons, [interval]: bars },
                          })
                        }
                      />
                    </SettingRow>
                  ))}
                </FieldGrid>
              </div>

              <div className="border-t border-[var(--line)] px-4 py-2">
                <div className="grid grid-cols-[minmax(0,1fr)_40px_72px_72px] items-center gap-x-3 py-2 text-xs font-medium text-vergex-text-muted sm:grid-cols-[minmax(0,1fr)_40px_112px_112px]">
                  <span>{label('factors')}</span>
                  <span>{label('on')}</span>
                  <span>{label('weightTrending')}</span>
                  <span>{label('weightRanging')}</span>
                </div>
                {FACTOR_ORDER.filter((name) => draft.factors[name]).map(
                  (name) => {
                    const factor = draft.factors[name]
                    const title = label(`factor.${name}`)
                    return (
                      <div
                        key={name}
                        className="grid grid-cols-[minmax(0,1fr)_40px_72px_72px] items-center gap-x-3 border-t border-[var(--line)] py-2 sm:grid-cols-[minmax(0,1fr)_40px_112px_112px]"
                      >
                        <span className="min-w-0 text-sm leading-tight text-vergex-text">
                          {title}
                        </span>
                        <Switch
                          aria-label={title}
                          checked={factor.enabled}
                          onChange={(enabled) => patchFactor(name, { enabled })}
                        />
                        <div className="flex">
                          <NumberField
                            value={factor.trending}
                            min={-5}
                            max={5}
                            step={0.1}
                            disabled={!factor.enabled}
                            ariaLabel={label('weightOf', {
                              name: title,
                              regime: label('weightTrending'),
                            })}
                            onChange={(trending) =>
                              patchFactor(name, { trending })
                            }
                          />
                        </div>
                        <div className="flex">
                          <NumberField
                            value={factor.ranging}
                            min={-5}
                            max={5}
                            step={0.1}
                            disabled={!factor.enabled}
                            ariaLabel={label('weightOf', {
                              name: title,
                              regime: label('weightRanging'),
                            })}
                            onChange={(ranging) =>
                              patchFactor(name, { ranging })
                            }
                          />
                        </div>
                      </div>
                    )
                  }
                )}
              </div>

              <div className="border-t border-[var(--line)] px-4">
                <SettingRow label={label('strategyName')} htmlFor="fc-name">
                  <input
                    id="fc-name"
                    className={fieldClass}
                    value={name}
                    maxLength={80}
                    onChange={(event) => setName(event.target.value)}
                  />
                </SettingRow>
              </div>

              <div className="sticky bottom-0 z-10 rounded-b-xl bg-[var(--surface)] px-4">
                <SettingActions
                  message={
                    noneOn
                      ? label('needFactor')
                      : changed && !name.trim()
                        ? label('needName')
                        : undefined
                  }
                >
                  <Button
                    type="button"
                    variant="primary"
                    size="sm"
                    className="max-sm:h-10"
                    disabled={
                      busy || !changed || !name.trim() || Boolean(noneOn)
                    }
                    onClick={() => void save()}
                  >
                    {label('saveStrategy')}
                  </Button>
                </SettingActions>
              </div>
            </Panel>
          </div>
        </div>
      )}
    </ForecastFrame>
  )
}
