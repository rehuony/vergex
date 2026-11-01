import useSWR, { useSWRConfig } from 'swr'
import { api } from '@/api'
import { notify } from '@/components/common/notify'
import { AppSelect } from '@/components/ui/select'
import { t, type Language } from '@/i18n'

// The strategy in use. Choosing one puts it to use: signals and alerts follow it, and
// nothing trades on them, so there is no separate step to confirm.
export function StrategySelect({ language }: { language: Language }) {
  const label = (key: string, params?: Record<string, string | number>) =>
    t(`forecast.${key}`, language, params)
  const { data, mutate } = useSWR(
    'forecast-strategies',
    api.getForecastStrategies,
    { revalidateOnFocus: false }
  )
  const { mutate: refresh } = useSWRConfig()
  // Holds its place while loading, so nothing beside it moves.
  if (!data) return <div className="h-9 w-48 max-sm:h-10" />

  const choose = async (id: string) => {
    const chosen = data.strategies.find((strategy) => strategy.id === id)
    try {
      await api.activateForecastStrategy(id)
      await mutate()
      void refresh(
        (key) => Array.isArray(key) && String(key[0]).startsWith('forecast-')
      )
      if (chosen) {
        notify.success(label('strategyActivated', { name: chosen.name }))
      }
    } catch (err) {
      notify.error(err instanceof Error ? err.message : label('useFailed'))
    }
  }
  return (
    <AppSelect
      aria-label={label('strategy')}
      className="h-9 w-48 max-sm:h-10"
      value={data.active_strategy_id}
      options={data.strategies.map((strategy) => ({
        value: strategy.id,
        label: strategy.name,
      }))}
      onChange={(id) => void choose(id)}
    />
  )
}
