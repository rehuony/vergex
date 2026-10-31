import { useEffect, useState } from 'react'
import useSWR from 'swr'
import { toast } from 'sonner'
import { api } from '@/api'
import { NumberField, SymbolsField } from '@/components/common/form-fields'
import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import { t, type Language } from '@/i18n/translations'
import { SettingActions, SettingRow, SettingState } from './setting-row'

const MAX_WATCHLIST = 100

/** What Forecast collects: the watchlist, high-volume coins and their state. */
export function ForecastSettings({ language }: { language: Language }) {
  const label = (key: string, params?: Record<string, string | number>) =>
    t(`forecastSettings.${key}`, language, params)
  const { data, error, mutate } = useSWR(
    'market-data-settings',
    api.getMarketDataSettings,
    { revalidateOnFocus: false }
  )
  const { data: forecast, mutate: mutateForecast } = useSWR(
    'forecast-settings',
    api.getForecastSettings,
    { revalidateOnFocus: false }
  )
  const { data: status } = useSWR(
    'market-data-status',
    api.getMarketDataStatus,
    { refreshInterval: 5000 }
  )
  const [enabled, setEnabled] = useState(true)
  const [watchlist, setWatchlist] = useState<string[]>([])
  const [topN, setTopN] = useState(0)
  const [threshold, setThreshold] = useState(70)
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    if (!data) return
    setEnabled(data.enabled)
    setWatchlist(data.watchlist)
    setTopN(data.top_n)
  }, [data])
  useEffect(() => {
    if (forecast) setThreshold(forecast.alert_threshold)
  }, [forecast])

  if (error) {
    return <SettingState role="alert">{label('loadFailed')}</SettingState>
  }
  if (!data) return <SettingState>{label('loading')}</SettingState>

  const changed =
    enabled !== data.enabled ||
    topN !== data.top_n ||
    watchlist.join() !== data.watchlist.join() ||
    threshold !== forecast?.alert_threshold

  const save = async () => {
    setSaving(true)
    try {
      await mutate(
        await api.saveMarketDataSettings({
          enabled,
          watchlist,
          top_n: topN,
        }),
        { revalidate: false }
      )
      if (threshold !== forecast?.alert_threshold) {
        await api.saveForecastSettings(threshold)
        await mutateForecast()
      }
      toast.success(label('saved'))
    } catch (err) {
      toast.error(err instanceof Error ? err.message : label('saveFailed'))
    } finally {
      setSaving(false)
    }
  }

  const paused = status?.limiter.reason
  return (
    <div>
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
      <SettingRow label={label('watchlist')} htmlFor="forecast-watchlist">
        <SymbolsField
          id="forecast-watchlist"
          values={watchlist}
          placeholder="BTCUSDT, ETHUSDT"
          max={MAX_WATCHLIST}
          limitMessage={label('watchlistLimit', { count: MAX_WATCHLIST })}
          addLabel={label('add')}
          removeLabel={(symbol) => label('remove', { symbol })}
          onChange={setWatchlist}
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
      <SettingRow label={label('alertThreshold')} htmlFor="forecast-alert">
        <NumberField
          id="forecast-alert"
          value={threshold}
          min={51}
          max={99}
          suffix={label('alertThresholdSuffix', { low: 100 - threshold })}
          onChange={setThreshold}
        />
      </SettingRow>
      <SettingRow label={label('status')}>
        <p className="text-sm tabular-nums text-vergex-text">
          {status
            ? label('statusValue', {
                ready: status.ready,
                series: status.series,
                behind: status.behind,
              })
            : '—'}
        </p>
      </SettingRow>
      <SettingActions
        message={paused || status?.last_error}
        messageTone={paused || status?.last_error ? 'danger' : 'muted'}
      >
        <Button
          type="button"
          size="sm"
          variant="primary"
          disabled={saving || !changed}
          onClick={() => void save()}
        >
          {saving ? label('saving') : label('save')}
        </Button>
      </SettingActions>
    </div>
  )
}
