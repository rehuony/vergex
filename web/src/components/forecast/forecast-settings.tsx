import { useEffect, useRef, useState, type ReactNode } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { api } from '@/api'
import { NumberField, SymbolsField } from '@/components/common/form-fields'
import { Panel } from '@/components/common/panel'
import {
  SettingActions,
  SettingRow,
  SettingState,
} from '@/components/common/setting-row'
import { Button } from '@/components/ui/button'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Switch } from '@/components/ui/switch'
import { t, type Language } from '@/i18n'
import { cn } from '@/utils/cn'
import {
  forecastSettingsQuery,
  marketSettingsQuery,
  marketStatusQuery,
} from './forecast-queries'
import { MAX_WATCHLIST } from './use-watchlist'

const labelsIn =
  (language: Language) =>
  (key: string, params?: Record<string, string | number>) =>
    t(`forecastSettings.${key}`, language, params)

// What is collected: whether anything is, the high-volume coins and the watchlist, whose
// coins take whatever height is left, with the save bar pinned under them. The button
// stays pressable: pressing it with nothing changed does nothing.
function CollectionPanel({ language }: { language: Language }) {
  const label = labelsIn(language)
  const client = useQueryClient()
  const { data, error } = useQuery(marketSettingsQuery)
  const [enabled, setEnabled] = useState(true)
  const [watchlist, setWatchlist] = useState<string[]>([])
  const [topN, setTopN] = useState(0)
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    if (!data) return
    setEnabled(data.enabled)
    setWatchlist(data.watchlist)
    setTopN(data.top_n)
  }, [data])

  const changed =
    data &&
    (enabled !== data.enabled ||
      topN !== data.top_n ||
      watchlist.join() !== data.watchlist.join())

  const save = async () => {
    if (saving || !changed) return
    setSaving(true)
    try {
      client.setQueryData(
        marketSettingsQuery.queryKey,
        await api.saveMarketDataSettings({
          enabled,
          watchlist,
          top_n: topN,
        })
      )
      toast.success(label('saved'))
    } catch (err) {
      toast.error(err instanceof Error ? err.message : label('saveFailed'))
    } finally {
      setSaving(false)
    }
  }

  return (
    <Panel
      title={label('collection')}
      className="flex min-h-0 flex-col overflow-hidden"
    >
      <div className="flex min-h-0 flex-1 flex-col border-t border-vergex-line px-4">
        {error ? (
          <SettingState role="alert">{label('loadFailed')}</SettingState>
        ) : !data ? (
          <SettingState>{label('loading')}</SettingState>
        ) : (
          <>
            <SettingRow
              label={label('collect')}
              className="max-sm:grid-cols-[minmax(0,1fr)_auto] max-sm:items-center"
            >
              <Switch
                aria-label={label('collect')}
                checked={enabled}
                onChange={setEnabled}
              />
            </SettingRow>
            <SettingRow label={label('topCoins')} htmlFor="forecast-top">
              <NumberField
                id="forecast-top"
                value={topN}
                min={0}
                max={400}
                step={10}
                suffix={label('byVolume')}
                onChange={setTopN}
              />
            </SettingRow>
            <SettingRow
              label={`${label('watchlist')} ${watchlist.length} / ${MAX_WATCHLIST}`}
              htmlFor="forecast-watchlist"
              top
              className="lg:min-h-0 lg:flex-1 lg:grid-rows-[minmax(0,1fr)] lg:[&>div]:h-full lg:[&>div]:flex-nowrap lg:[&>div]:items-stretch"
            >
              <SymbolsField
                id="forecast-watchlist"
                values={watchlist}
                placeholder="BTCUSDT, ETHUSDT"
                max={MAX_WATCHLIST}
                limitMessage={label('watchlistLimit', { count: MAX_WATCHLIST })}
                addLabel={label('add')}
                removeLabel={(symbol) => label('remove', { symbol })}
                scrollClassName="max-h-40 lg:max-h-none lg:min-h-0 lg:flex-1"
                onChange={setWatchlist}
              />
            </SettingRow>
            <SettingActions>
              <Button
                type="button"
                size="sm"
                variant="primary"
                className="max-sm:h-10"
                onClick={() => void save()}
              >
                {saving ? label('saving') : label('save')}
              </Button>
            </SettingActions>
          </>
        )}
      </div>
    </Panel>
  )
}

// When a coin's call is strong enough to send an alert: one row, saved on leaving the
// field or pressing Enter. The ref is what to save, because the field pulls an
// out-of-range value back as it loses focus, in the same event this panel hears it in,
// before the new state has reached a render.
function AlertPanel({ language }: { language: Language }) {
  const label = labelsIn(language)
  const { data, error, refetch } = useQuery(forecastSettingsQuery)
  const [threshold, setThreshold] = useState(70)
  const latest = useRef(threshold)

  const show = (value: number) => {
    latest.current = value
    setThreshold(value)
  }

  useEffect(() => {
    if (!data) return
    latest.current = data.alert_threshold
    setThreshold(data.alert_threshold)
  }, [data])

  const save = async () => {
    if (!data || latest.current === data.alert_threshold) return
    try {
      await api.saveForecastSettings(latest.current)
      await refetch()
      toast.success(label('saved'))
    } catch (err) {
      show(data.alert_threshold)
      toast.error(err instanceof Error ? err.message : label('saveFailed'))
    }
  }

  return (
    <Panel title={label('alerts')}>
      <div
        className="border-t border-vergex-line px-4"
        onBlur={() => void save()}
        onKeyDown={(event) => {
          if (event.key === 'Enter' && event.target instanceof HTMLElement)
            event.target.blur()
        }}
      >
        {error ? (
          <SettingState role="alert">{label('loadFailed')}</SettingState>
        ) : !data ? (
          <SettingState>{label('loading')}</SettingState>
        ) : (
          <SettingRow
            label={label('alertThreshold')}
            htmlFor="forecast-alert"
            className="grid-cols-[auto_minmax(0,1fr)] items-center gap-3 lg:grid-cols-[auto_minmax(0,1fr)] lg:gap-3"
          >
            <NumberField
              id="forecast-alert"
              value={threshold}
              min={51}
              max={99}
              suffix={label('alertThresholdSuffix', { low: 100 - threshold })}
              onChange={show}
            />
          </SettingRow>
        )}
      </div>
    </Panel>
  )
}

function StatusRow({
  name,
  tone,
  title,
  children,
}: {
  name: string
  tone?: string
  title?: string
  children: ReactNode
}) {
  return (
    <div className="flex items-baseline justify-between gap-4 border-t border-vergex-line px-4 py-2.5">
      <dt className="shrink-0 text-[13px] text-vergex-muted">{name}</dt>
      <dd
        title={title}
        className={cn(
          'min-w-0 truncate text-right text-[13px] tabular-nums text-vergex-ink',
          tone
        )}
      >
        {children}
      </dd>
    </div>
  )
}

// How collection is doing, read only and kept in view. Its rows are always there, so a
// problem appearing changes no region's size.
function StatusPanel({ language }: { language: Language }) {
  const label = labelsIn(language)
  const { data: status } = useQuery({
    ...marketStatusQuery,
    refetchInterval: 5000,
  })
  const backfill = status?.backfill
  const problem = status?.limiter.reason || status?.last_error
  return (
    <Panel
      title={label('health')}
      className="flex min-h-0 flex-1 flex-col overflow-hidden"
    >
      <ScrollArea className="min-h-0 flex-1 rounded-b-2xl">
        <dl>
          <StatusRow name={label('seriesReady')}>
            {status ? `${status.ready} / ${status.series}` : '—'}
          </StatusRow>
          <StatusRow name={label('behind')}>{status?.behind ?? '—'}</StatusRow>
          <StatusRow name={label('failing')}>
            {status?.failing ?? '—'}
          </StatusRow>
          <StatusRow name={label('watchCoins')}>
            {status?.watch_symbols ?? '—'}
          </StatusRow>
          <StatusRow name={label('topCoins')}>
            {status?.top_symbols ?? '—'}
          </StatusRow>
          <StatusRow
            name={label('stream')}
            tone={
              status &&
              (status.stream.connected
                ? 'text-vergex-success'
                : 'text-vergex-danger')
            }
          >
            {status
              ? status.stream.connected
                ? label('streamUp', { topics: status.stream.topics })
                : label('streamDown')
              : '—'}
          </StatusRow>
          <StatusRow name={label('requestWeight')}>
            {status?.limiter.used_weight ?? '—'}
          </StatusRow>
          <StatusRow name={label('backfill')}>
            {backfill?.running ? `${backfill.done} / ${backfill.total}` : '—'}
          </StatusRow>
          <StatusRow
            name={label('problem')}
            title={problem}
            tone={problem ? 'text-vergex-danger' : undefined}
          >
            {problem || '—'}
          </StatusRow>
        </dl>
      </ScrollArea>
    </Panel>
  )
}

// Forecast's settings: what is collected fills the left, with the alert threshold and how
// collection is doing beside it, all within the first screen.
export function ForecastSettings({ language }: { language: Language }) {
  return (
    <div className="grid min-w-0 gap-4 lg:h-[max(520px,calc(100dvh-181px))] lg:grid-cols-[minmax(0,1fr)_320px]">
      <CollectionPanel language={language} />
      <div className="flex min-h-0 min-w-0 flex-col gap-4">
        <AlertPanel language={language} />
        <StatusPanel language={language} />
      </div>
    </div>
  )
}
